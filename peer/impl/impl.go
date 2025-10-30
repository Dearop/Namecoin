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
	conf             peer.Configuration
	stopCh           chan struct{}
	wg               sync.WaitGroup
	routingTable     map[string]string
	mu               sync.RWMutex
	lastSeq          map[string]uint
	rumors           map[string]map[uint]types.Rumor
	pmu              sync.Mutex
	pendingAcks      map[string]*pendingAck
	lastStatusMu     sync.Mutex
	lastStatusAt     time.Time
	statusRateMu     sync.Mutex
	lastStatusTo     map[string]time.Time
	catalog          map[string]map[string]struct{}
	dataReqMu        sync.Mutex
	pendingData      map[string]chan types.DataReplyMessage
	processedDataReq sync.Map // map[string]bool - tracks processed request IDs
	searchMu         sync.Mutex
	pendingSearch    map[string]chan types.SearchReplyMessage
	currentStep      uint
	// Acceptor state per step
	promisedID    map[uint]uint
	acceptedID    map[uint]uint
	acceptedValue map[uint]*types.PaxosValue
	// Proposer state per step
	proposerPromises          map[uint]map[uint]map[string]struct{}
	proposerValue             map[uint]types.PaxosValue
	proposerHighestAcceptedID map[uint]uint
	proposerPhase             map[uint]int // 0 none, 1 phase1, 2 phase2, 3 done
	proposerID                map[uint]uint
	proposerAccepts           map[uint]map[uint]map[string]struct{}
	proposerRetryOn           map[uint]bool
	// Learner/TLC aggregation
	acceptCount    map[uint]map[uint]map[string]struct{}
	tlcCount       map[uint]map[string]struct{}
	tlcBlock       map[uint]types.BlockchainBlock
	tlcBroadcasted map[uint]bool
	// Step completion waiters
	stepWaitMu  sync.Mutex
	stepWaiters map[uint][]chan struct{}
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
	// Register consensus messages (HW3)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosPrepareMessage{}, n.handlePaxosPrepare)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosPromiseMessage{}, n.handlePaxosPromise)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosProposeMessage{}, n.handlePaxosPropose)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosAcceptMessage{}, n.handlePaxosAccept)
	n.conf.MessageRegistry.RegisterMessageCallback(types.TLCMessage{}, n.handleTLC)
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
	// send to all neighbors for reliable broadcast
	for _, neighbor := range neighbors {
		header := transport.NewHeader(nodeAddr, nodeAddr, neighbor)
		pkt := transport.Packet{Header: &header, Msg: &wireMsg}

		// set up ack waiting if configured
		if n.conf.AckTimeout > 0 {
			n.trackAck(pkt, neighbor, rumorsMsg)
		}
		_ = n.conf.Socket.Send(neighbor, pkt, time.Second)
	}
	// prevent immediate status before first anti-entropy tick
	if n.conf.AntiEntropyInterval > 0 {
		n.lastStatusMu.Lock()
		if n.lastStatusAt.IsZero() {
			n.lastStatusAt = time.Now()
		}
		n.lastStatusMu.Unlock()
	}
	return nil
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
