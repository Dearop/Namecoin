package impl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

func NewPeer(conf peer.Configuration) peer.Peer {
	node := &node{conf: conf, stopCh: make(chan struct{})}
	node.mu.Lock()
	node.routingTable = make(map[string]string)
	node.lastSeq = make(map[string]uint)
	node.rumors = make(map[string]map[uint]types.Rumor)
	node.pendingAcks = make(map[string]*pendingAck)
	if conf.Socket != nil {
		nodeAddr := conf.Socket.GetAddress()
		if nodeAddr != "" {
			node.routingTable[nodeAddr] = nodeAddr
		}
	}
	node.mu.Unlock()
	return node
}

// node implements a peer to build a Peerster system
// - implements peer.Peer
type node struct {
	peer.Peer
	conf          peer.Configuration
	stopCh        chan struct{}
	wg            sync.WaitGroup
	routingTable  map[string]string
	mu            sync.RWMutex
	lastSeq       map[string]uint
	rumors        map[string]map[uint]types.Rumor
	pmu           sync.Mutex
	pendingAcks   map[string]*pendingAck
	lastStatusMu  sync.Mutex
	lastStatusAt  time.Time
	statusRateMu  sync.Mutex
	lastStatusTo  map[string]time.Time
	catalog       map[string]map[string]struct{}
	dataReqMu     sync.Mutex
	pendingData   map[string]chan types.DataReplyMessage
	searchMu      sync.Mutex
	pendingSearch map[string]chan types.SearchReplyMessage
}

type pendingAck struct {
	packet transport.Packet
	dest   string
	timer  *time.Timer
}

// Start implements peer.Service
func (n *node) Start() error {
	if err := n.validateNode(true); err != nil {
		return err
	}
	if n.stopCh == nil {
		n.stopCh = make(chan struct{})
	}
	if n.routingTable == nil {
		n.mu.Lock()
		if n.routingTable == nil {
			n.routingTable = make(map[string]string)
		}
		n.mu.Unlock()
	}
	n.conf.MessageRegistry.RegisterMessageCallback(types.ChatMessage{}, n.handleChatMessage)
	n.conf.MessageRegistry.RegisterMessageCallback(types.RumorsMessage{}, n.handleRumorsMessage)
	n.conf.MessageRegistry.RegisterMessageCallback(types.StatusMessage{}, n.handleStatusMessage)
	n.conf.MessageRegistry.RegisterMessageCallback(types.AckMessage{}, n.handleAckMessage)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PrivateMessage{}, n.handlePrivateMessage)
	// Register data sharing messages
	n.conf.MessageRegistry.RegisterMessageCallback(types.DataRequestMessage{}, n.handleDataRequest)
	n.conf.MessageRegistry.RegisterMessageCallback(types.DataReplyMessage{}, n.handleDataReply)
	n.conf.MessageRegistry.RegisterMessageCallback(types.SearchReplyMessage{}, n.handleSearchReply)
	n.conf.MessageRegistry.RegisterMessageCallback(types.SearchRequestMessage{}, n.handleSearchRequest)
	n.wg.Add(1)
	go n.listenLoop()

	// Anti-entropy
	if n.conf.AntiEntropyInterval > 0 {
		n.wg.Add(1)
		go n.antiEntropyLoop(n.conf.AntiEntropyInterval)
	}

	// Heartbeat: send one immediately and then periodically if enabled
	if n.conf.HeartbeatInterval > 0 {
		// fire-and-forget initial heartbeat
		go n.sendHeartbeat()
		n.wg.Add(1)
		go n.heartbeatLoop(n.conf.HeartbeatInterval)
	}
	return nil
}

// acceptExpectedRumors processes expected rumors and returns those that were newly accepted.
func (n *node) acceptExpectedRumors(rumors []types.Rumor, header *transport.Header) []types.Rumor {
	if err := n.validateNode(false); err != nil {
		return nil
	}
	if header == nil {
		return nil
	}
	accepted := make([]types.Rumor, 0, len(rumors))
	for _, r := range rumors {
		if n.processRumorIfExpected(r, header) {
			accepted = append(accepted, r)
		}
	}
	return accepted
}

// forwardAcceptedRumorsOnce forwards accepted rumors to one random neighbor (not the source), if any.
func (n *node) forwardAcceptedRumorsOnce(accepted []types.Rumor, header *transport.Header) {
	if len(accepted) == 0 || header == nil {
		return
	}
	if err := n.validateNode(false); err != nil {
		return
	}
	wire, err := n.conf.MessageRegistry.MarshalMessage(types.RumorsMessage{Rumors: accepted})
	if err != nil {
		return
	}
	src := strings.TrimSpace(header.Source)
	nodeAddr := n.conf.Socket.GetAddress()
	n.mu.RLock()
	neighbors := make([]string, 0, len(n.routingTable))
	for origin, relay := range n.routingTable {
		if origin == relay && origin != nodeAddr && origin != src {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()
	if len(neighbors) == 0 {
		return
	}
	dest := neighbors[int(time.Now().UnixNano())%len(neighbors)]
	fwdHeader := transport.NewHeader(nodeAddr, nodeAddr, dest)
	_ = n.conf.Socket.Send(dest, transport.Packet{Header: &fwdHeader, Msg: &wire}, time.Second)
}

// respondToStatus handles status comparison, sending missing rumors and optionally continuing mongering.
func (n *node) respondToStatus(source, self string, remote types.StatusMessage) {
	if err := n.validateNode(false); err != nil {
		return
	}
	source = strings.TrimSpace(source)
	self = strings.TrimSpace(self)
	haveForThem, needFromThem, local := n.computeStatusDeltas(remote)
	if haveForThem {
		n.sendMissingRumorsTo(source, self, remote, local)
	}
	// Important: do not respond to empty anti-entropy statuses.
	if needFromThem && source != "" && self != "" && len(remote) > 0 {
		wire, err := n.conf.MessageRegistry.MarshalMessage(n.buildStatus())
		if err == nil {
			header := transport.NewHeader(self, self, source)
			_ = n.conf.Socket.Send(source, transport.Packet{Header: &header, Msg: &wire}, time.Second)
		}
	}
	if !haveForThem && !needFromThem {
		if n.conf.ContinueMongering == 0 {
			return
		}
		// only continue mongering if there is at least one neighbor different from source
		nodeAddr := n.conf.Socket.GetAddress()
		n.mu.RLock()
		candidates := 0
		for origin, relay := range n.routingTable {
			if origin == relay && origin != nodeAddr && origin != source {
				candidates++
			}
		}
		n.mu.RUnlock()
		if candidates > 0 {
			n.probabilisticallyMonger(source)
		}
	}
}

func (n *node) trackAck(pkt transport.Packet, dest string, rumors types.RumorsMessage) {
	timeout := n.conf.AckTimeout
	if timeout <= 0 {
		return
	}
	t := time.NewTimer(timeout)
	n.pmu.Lock()
	n.pendingAcks[pkt.Header.PacketID] = &pendingAck{packet: pkt, dest: dest, timer: t}
	n.pmu.Unlock()
	go func(packetID string) {
		<-t.C
		// on timeout, forward to another neighbor different from previous
		n.pmu.Lock()
		p := n.pendingAcks[packetID]
		if p == nil {
			n.pmu.Unlock()
			return
		}
		delete(n.pendingAcks, packetID)
		n.pmu.Unlock()

		// choose a different neighbor
		nodeAddr := n.conf.Socket.GetAddress()
		n.mu.RLock()
		neighbors := make([]string, 0, len(n.routingTable))
		for origin, relay := range n.routingTable {
			if origin == relay && origin != nodeAddr && origin != dest {
				neighbors = append(neighbors, origin)
			}
		}
		n.mu.RUnlock()
		if len(neighbors) == 0 {
			return
		}
		newDest := neighbors[int(time.Now().UnixNano())%len(neighbors)]
		wire, err := n.conf.MessageRegistry.MarshalMessage(rumors)
		if err != nil {
			return
		}
		header := transport.NewHeader(nodeAddr, nodeAddr, newDest)
		_ = n.conf.Socket.Send(newDest, transport.Packet{Header: &header, Msg: &wire}, time.Second)
	}(pkt.Header.PacketID)
}

func (n *node) listenLoop() {
	if err := n.validateNode(false); err != nil {
		return
	}
	var errorCount int
	defer n.wg.Done()

	for {
		if n.shouldStop() {
			return
		}

		pkt, err := n.conf.Socket.Recv(time.Second * 1)
		if err != nil {
			if n.handleSocketError(err, &errorCount) {
				return
			}
			continue
		}

		n.resetErrorCount(&errorCount)
		n.processPacket(pkt)
	}
}

// shouldStop checks if the node should stop and returns true if it should
func (n *node) shouldStop() bool {
	select {
	case <-n.stopCh:
		return true
	default:
		return false
	}
}

// handleSocketError handles socket errors and returns true if the node should stop
func (n *node) handleSocketError(err error, errorCount *int) bool {
	if xerrors.Is(err, transport.TimeoutError(0)) {
		return false
	}

	*errorCount++
	if *errorCount > 10 {
		n.stopNodeOnHighErrorCount()
		return true
	}

	n.logSocketError(*errorCount, err)
	return false
}

// stopNodeOnHighErrorCount stops the node when error count is too high
func (n *node) stopNodeOnHighErrorCount() {
	go func() {
		_ = n.Stop()
	}()
	if os.Getenv("GLOG") != "no" {
		log.Printf("High error count, stopping node")
	}
}

// logSocketError logs socket errors if appropriate
func (n *node) logSocketError(errorCount int, err error) {
	if os.Getenv("GLOG") != "no" && errorCount <= 5 {
		log.Printf("Socket error (attempt %d): %v", errorCount, err)
	}
}

// resetErrorCount resets the error count when a successful packet is received
func (n *node) resetErrorCount(errorCount *int) {
	if *errorCount > 0 {
		*errorCount = 0
	}
}

// Stop implements peer.Service
func (n *node) Stop() error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	select {
	case <-n.stopCh:
	default:
		close(n.stopCh)
	}
	// Wait for goroutines to finish with a timeout to avoid hanging
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// All goroutines finished normally
	case <-time.After(5 * time.Second):
		// Timeout - some goroutines might still be running
		if os.Getenv("GLOG") != "no" {
			log.Printf("Warning: some goroutines did not finish within timeout")
		}
	}
	return nil
}

// Unicast implements peer.Messaging
func (n *node) Unicast(dest string, msg transport.Message) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	nodeAddr := n.conf.Socket.GetAddress()
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return xerrors.Errorf("empty destination")
	}
	// Basic fuzz-hardening on message: ensure type is non-empty
	if strings.TrimSpace(msg.Type) == "" {
		return xerrors.Errorf("invalid message: empty type")
	}
	nextHop, ok := n.lookupNextHop(dest)
	if !ok {
		return xerrors.Errorf("destination %s unknown", dest)
	}
	header := transport.NewHeader(nodeAddr, nodeAddr, dest)
	return n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &msg}, time.Second)
}

// Broadcast implements peer.Messaging
func (n *node) Broadcast(msg transport.Message) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	if strings.TrimSpace(msg.Type) == "" {
		return xerrors.Errorf("invalid message: empty type")
	}
	nodeAddr := n.conf.Socket.GetAddress()

	// Pick a random neighbor (entries where origin==relay and not self)
	n.mu.RLock()
	neighbors := make([]string, 0, len(n.routingTable))
	for origin, relay := range n.routingTable {
		if origin == relay && origin != nodeAddr {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()

	// Process locally first
	localHeader := transport.NewHeader(nodeAddr, nodeAddr, nodeAddr)
	_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: &localHeader, Msg: &msg})

	// Update our own sequence and build rumor
	n.mu.Lock()
	seq := n.lastSeq[nodeAddr] + 1
	n.lastSeq[nodeAddr] = seq
	if n.rumors[nodeAddr] == nil {
		n.rumors[nodeAddr] = make(map[uint]types.Rumor)
	}
	n.mu.Unlock()

	rumor := types.Rumor{Origin: nodeAddr, Sequence: seq, Msg: &msg}
	rumorsMsg := types.RumorsMessage{Rumors: []types.Rumor{rumor}}
	wireMsg, err := n.conf.MessageRegistry.MarshalMessage(rumorsMsg)
	if err != nil {
		return err
	}

	// store the rumor so anti-entropy/catch-up can resend it later
	n.mu.Lock()
	if n.rumors[nodeAddr] == nil {
		n.rumors[nodeAddr] = make(map[uint]types.Rumor)
	}
	n.rumors[nodeAddr][seq] = rumor
	n.mu.Unlock()

	if len(neighbors) == 0 {
		return nil
	}
	// choose neighbor deterministically
	neighbor := neighbors[int(time.Now().UnixNano())%len(neighbors)]

	header := transport.NewHeader(nodeAddr, nodeAddr, neighbor)
	pkt := transport.Packet{Header: &header, Msg: &wireMsg}

	// set up ack waiting if configured
	if n.conf.AckTimeout > 0 {
		n.trackAck(pkt, neighbor, rumorsMsg)
	}
	// prevent immediate status before first anti-entropy tick
	if n.conf.AntiEntropyInterval > 0 {
		n.lastStatusMu.Lock()
		if n.lastStatusAt.IsZero() {
			n.lastStatusAt = time.Now()
		}
		n.lastStatusMu.Unlock()
	}
	return n.conf.Socket.Send(neighbor, pkt, time.Second)
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	if err := n.validateNode(false); err != nil {
		return
	}
	nodeAddr := n.conf.Socket.GetAddress()
	if nodeAddr == "" {
		return
	}
	n.mu.Lock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[nodeAddr] = nodeAddr
	for _, peerAddr := range addr {
		peerAddr = strings.TrimSpace(peerAddr)
		if peerAddr == "" || peerAddr == nodeAddr {
			continue
		}
		n.routingTable[peerAddr] = peerAddr
	}
	n.mu.Unlock()
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	if err := n.validateNode(false); err != nil {
		return peer.RoutingTable{}
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	copyTable := make(peer.RoutingTable, len(n.routingTable))
	for dest, relay := range n.routingTable {
		copyTable[dest] = relay
	}
	return copyTable
}

// SetRoutingEntry implements peer.Messaging
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	if err := n.validateNode(false); err != nil {
		return
	}
	nodeAddr := n.conf.Socket.GetAddress()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[nodeAddr] = nodeAddr

	origin = strings.TrimSpace(origin)
	relayAddr = strings.TrimSpace(relayAddr)
	if origin == "" {
		return
	}
	if relayAddr == "" {
		delete(n.routingTable, origin)
		return
	}
	n.routingTable[origin] = relayAddr
}

// validateNode checks the receiver and its critical configuration.
// If requireRegistry is true, it also enforces a non-nil MessageRegistry.
func (n *node) validateNode(requireRegistry bool) error {
	if n == nil {
		return xerrors.Errorf("nil node")
	}
	if n.conf.Socket == nil {
		return xerrors.Errorf("socket not configured")
	}
	if n.conf.Socket.GetAddress() == "" {
		return xerrors.Errorf("node address not set")
	}
	if requireRegistry && n.conf.MessageRegistry == nil {
		return xerrors.Errorf("registry not configured")
	}
	return nil
}

func (n *node) antiEntropyLoop(interval time.Duration) {
	defer n.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			// exactly one status per tick
			n.sendStatusToNeighbor()
		}
	}
}

// Upload implements peer.DataSharing
func (n *node) Upload(data io.Reader) (string, error) {
	if err := n.validateNode(false); err != nil {
		return "", err
	}
	if data == nil {
		return "", xerrors.Errorf("nil reader")
	}
	blobStore := n.conf.Storage.GetDataBlobStore()
	chunkSize := n.conf.ChunkSize
	if chunkSize == 0 {
		chunkSize = 8192
	}

	// Limit: files up to 2 MiB
	const maxSize = 2 * 1024 * 1024
	var total int

	buf := make([]byte, chunkSize)
	chunkHashes := make([]string, 0, 16)

	for {
		nr, err := io.ReadFull(data, buf)
		if err == io.EOF && nr == 0 {
			break
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", xerrors.Errorf("read error: %v", err)
		}
		chunk := append([]byte(nil), buf[:nr]...)
		total += nr
		if total > maxSize {
			return "", xerrors.Errorf("data too large: exceeds 2 MiB")
		}
		sum := sha256.Sum256(chunk)
		hexKey := hex.EncodeToString(sum[:])
		blobStore.Set(hexKey, chunk)
		chunkHashes = append(chunkHashes, hexKey)
		if err == io.ErrUnexpectedEOF {
			// processed last partial chunk
			break
		}
	}

	if len(chunkHashes) == 0 {
		return "", xerrors.Errorf("empty data")
	}

	// Build metafile: hex hashes separated by peer.MetafileSep
	var mf bytes.Buffer
	for i, h := range chunkHashes {
		if i > 0 {
			mf.WriteString(peer.MetafileSep)
		}
		mf.WriteString(h)
	}
	metafile := mf.Bytes()

	// Ensure metafile fits in one chunk
	if len(metafile) > int(chunkSize) {
		return "", xerrors.Errorf("metafile too large for one chunk")
	}

	ms := sha256.Sum256(metafile)
	metahash := hex.EncodeToString(ms[:])

	// Store metafile under its metahash
	blobStore.Set(metahash, metafile)

	return metahash, nil
}

// Download implements peer.DataSharing
func (n *node) Download(metahash string) ([]byte, error) {
	if err := n.validateNode(false); err != nil {
		return nil, err
	}
	mh := strings.TrimSpace(metahash)
	if mh == "" {
		return nil, xerrors.Errorf("empty metahash")
	}
	blob := n.conf.Storage.GetDataBlobStore()

	// Step 1: get metafile value (list of chunk keys)
	metafile := blob.Get(mh)
	if metafile == nil {
		// fetch remotely using catalog
		val, err := n.fetchRemote(mh)
		if err != nil {
			return nil, err
		}
		metafile = val
		blob.Set(mh, metafile)
	}

	// Parse metafile into chunk keys
	lines := strings.Split(string(metafile), peer.MetafileSep)
	if len(lines) == 0 {
		return nil, xerrors.Errorf("invalid metafile")
	}

	// Step 2: sequentially fetch chunks
	var out bytes.Buffer
	for _, key := range lines {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, xerrors.Errorf("invalid chunk key in metafile")
		}
		data := blob.Get(key)
		if data == nil {
			val, err := n.fetchRemote(key)
			if err != nil {
				return nil, err
			}
			// verify integrity
			sum := sha256.Sum256(val)
			if hex.EncodeToString(sum[:]) != key {
				// wrong data: drop and update catalog
				n.removeFromCatalog(key)
				return nil, xerrors.Errorf("tampered chunk")
			}
			blob.Set(key, val)
			data = val
		}
		out.Write(data)
	}
	return out.Bytes(), nil
}

func (n *node) fetchRemote(key string) ([]byte, error) {
	// pick random peer from catalog for key, ensure routable
	dest, ok := n.pickPeerForKey(key)
	if !ok {
		return nil, xerrors.Errorf("no catalog entry for %s", key)
	}
	nextHop, ok := n.lookupNextHop(dest)
	if !ok {
		return nil, xerrors.Errorf("no route to %s", dest)
	}

	// create pending channel
	reqID := xid.New().String()
	ch := make(chan types.DataReplyMessage, 1)
	n.dataReqMu.Lock()
	if n.pendingData == nil {
		n.pendingData = make(map[string]chan types.DataReplyMessage)
	}
	n.pendingData[reqID] = ch
	n.dataReqMu.Unlock()
	defer func() {
		n.dataReqMu.Lock()
		delete(n.pendingData, reqID)
		n.dataReqMu.Unlock()
	}()

	// send request with backoff
	back := n.conf.BackoffDataRequest
	if back.Initial <= 0 {
		back.Initial = time.Second * 2
	}
	if back.Factor == 0 {
		back.Factor = 2
	}
	if back.Retry == 0 {
		back.Retry = 5
	}

	req := types.DataRequestMessage{RequestID: reqID, Key: key}
	wire, err := n.conf.MessageRegistry.MarshalMessage(req)
	if err != nil {
		return nil, err
	}
	header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), dest)

	wait := back.Initial
	for i := uint(0); i < back.Retry; i++ {
		_ = n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &wire}, time.Second)
		select {
		case rep := <-ch:
			if rep.Key != key || rep.RequestID != reqID {
				continue
			}
			if rep.Value == nil {
				// remove bogus catalog entry
				n.removeFromCatalog(key)
				return nil, xerrors.Errorf("empty reply")
			}
			return rep.Value, nil
		case <-time.After(wait):
			wait *= time.Duration(back.Factor)
		}
	}
	return nil, xerrors.Errorf("timeout fetching %s", key)
}

func (n *node) pickPeerForKey(key string) (string, bool) {
	n.mu.RLock()
	peers := n.catalog[key]
	n.mu.RUnlock()
	if len(peers) == 0 {
		return "", false
	}
	// deterministic pick - get first peer
	for p := range peers {
		return p, true
	}
	return "", false
}

func (n *node) removeFromCatalog(key string) {
	n.mu.Lock()
	delete(n.catalog, key)
	n.mu.Unlock()
}
