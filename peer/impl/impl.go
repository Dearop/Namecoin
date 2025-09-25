package impl

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// NewPeer creates a new peer. You can change the content and location of this
// function but you MUST NOT change its signature and package location.
func NewPeer(conf peer.Configuration) peer.Peer {
	// here you must return a struct that implements the peer.Peer functions.
	// Therefore, you are free to rename and change it as you want.
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
	conf         peer.Configuration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	routingTable map[string]string
	mu           sync.RWMutex
	lastSeq      map[string]uint
	rumors       map[string]map[uint]types.Rumor
	pmu          sync.Mutex
	pendingAcks  map[string]*pendingAck
	lastStatusMu sync.Mutex
	lastStatusAt time.Time
	statusRateMu sync.Mutex
	lastStatusTo map[string]time.Time
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

func (n *node) handleChatMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	chat, ok := m.(*types.ChatMessage)
	if !ok || chat == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil || pkt.Msg == nil {
		return xerrors.Errorf("invalid packet")
	}
	src := "unknown"
	if pkt.Header != nil && pkt.Header.Source != "" {
		src = pkt.Header.Source
	}
	if os.Getenv("GLOG") != "no" {
		log.Printf("[chat] from: %s, msg: %s", src, chat.Message)
	}
	return nil
}

func (n *node) handlePrivateMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	pm, ok := m.(*types.PrivateMessage)
	if !ok || pm == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil || pm.Msg == nil {
		return xerrors.Errorf("invalid private message")
	}
	dest := strings.TrimSpace(pkt.Header.Destination)
	// process only if destination is in recipients set
	if _, ok := pm.Recipients[dest]; !ok {
		return nil
	}
	// Process embedded message with same header
	_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: pkt.Header, Msg: pm.Msg})
	return nil
}

func (n *node) handleRumorsMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	rumorsMsg, ok := m.(*types.RumorsMessage)
	if !ok || rumorsMsg == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}

	// Process expected rumors and collect newly accepted ones
	forwardable := n.acceptExpectedRumors(rumorsMsg.Rumors, pkt.Header)

	// Send ack back to source (directly) with the receiver's current status
	source := strings.TrimSpace(pkt.Header.Source)
	if source != "" {
		ack := types.AckMessage{
			AckedPacketID: pkt.Header.PacketID,
			Status:        n.buildStatus(),
		}
		wireAck, err := n.conf.MessageRegistry.MarshalMessage(ack)
		if err == nil {
			header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), source)
			_ = n.conf.Socket.Send(source, transport.Packet{Header: &header, Msg: &wireAck}, time.Second)
		}
	}
	// Forward only the newly accepted rumors once to a random neighbor (not the source)
	n.forwardAcceptedRumorsOnce(forwardable, pkt.Header)
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

func (n *node) handleStatusMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	remote, ok := m.(types.StatusMessage)
	if !ok {
		p, ok2 := m.(*types.StatusMessage)
		if !ok2 || p == nil {
			return xerrors.Errorf("unexpected message type")
		}
		remote = *p
	}
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}
	self := n.conf.Socket.GetAddress()
	source := strings.TrimSpace(pkt.Header.Source)

	n.respondToStatus(source, self, remote)
	return nil
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
			n.maybeContinueMongering(source)
		}
	}
}

func (n *node) handleAckMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	ack, ok := m.(*types.AckMessage)
	if !ok || ack == nil {
		return xerrors.Errorf("unexpected message type")
	}
	// Stop waiting for this ack if it's pending
	n.pmu.Lock()
	p, ok := n.pendingAcks[pkt.Header.PacketID]
	if ok && p != nil && p.timer != nil {
		p.timer.Stop()
		delete(n.pendingAcks, pkt.Header.PacketID)
	}
	n.pmu.Unlock()

	// Process the embedded status using the registry
	wire, err := n.conf.MessageRegistry.MarshalMessage(ack.Status)
	if err == nil {
		// reuse header to process locally
		_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: pkt.Header, Msg: &wire})
	}
	return nil
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
		// re-arm waiting for new packet id is not required by spec here
	}(pkt.Header.PacketID)
}

func (n *node) listenLoop() {
	if err := n.validateNode(false); err != nil {
		return
	}
	var errorCount int
	defer n.wg.Done()
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}
		pkt, err := n.conf.Socket.Recv(time.Second * 1)
		if err != nil {
			if xerrors.Is(err, transport.TimeoutError(0)) {
				continue
			}
			errorCount++
			if errorCount > 10 {
				// stop the node in a goroutine to avoid blocking the receive loop
				go func() {
					_ = n.Stop()
				}()
				if os.Getenv("GLOG") != "no" {
					log.Printf("High error count, stopping node")
				}
				return
			}
			continue
		}
		if errorCount > 0 {
			errorCount = 0
		}
		n.processPacket(pkt)
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
	n.wg.Wait()
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

// sendStatusToNeighbor moved to utils.go (throttled)
