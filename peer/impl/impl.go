package impl

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/dns"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// NewPeer creates a new peer node with the given configuration.
// It initializes the routing table, sequence tracking, and rumor storage.
func NewPeer(conf peer.Configuration) peer.Peer {
	node := &node{conf: conf, stopCh: make(chan struct{})}
	addr := ""
	node.mu.Lock()
	node.routingTable = make(map[string]string)
	node.lastSeq = make(map[string]uint)
	node.rumors = make(map[string]map[uint]types.Rumor)
	node.pendingAcks = make(map[string]*pendingAck)
	if conf.Socket != nil {
		addr = conf.Socket.GetAddress()
		if addr != "" {
			node.routingTable[addr] = addr
		}
	}
	consensus, err := NewNamecoinConsensus(conf.PoWConfig, NewTxBuffer())
	// happens only if configuration is bad or "on a startup".
	if err != nil {
		panic(err)
	}

	namecoinChain, err := LoadNamecoinChain(conf.Storage.GetNamecoinStore())
	// happens only if configuration is bad or "on a startup".
	if err != nil {
		panic(err)
	}
	if addr != "" {
		logger := zlog.With().Str("node", addr).Logger()
		namecoinChain.state.SetLogger(logger)
		node.logger = logger
	}
	namecoinChain.SetPowTarget(conf.PoWConfig.Target)

	transactionService := NewTransactionService(namecoinChain.state)

	node.NamecoinChainService = NewChainService(namecoinChain)
	node.namecoinConsensus = consensus
	node.transactionService = transactionService
	node.namecoinDNS = NewNamecoinDNS()
	node.namecoinDNS.Start(node)
	node.pendingTxConfirm = make(map[string]chan error)
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
	// Learner/TLC aggregation
	acceptCount    map[uint]map[uint]map[string]struct{}
	tlcCount       map[uint]map[string]struct{}
	tlcBlock       map[uint]types.BlockchainBlock
	tlcBroadcasted map[uint]bool

	// Namecoin
	namecoinConsensus    *NamecoinConsensus
	NamecoinChainService *ChainService
	transactionService   *TransactionService
	namecoinDNS          *NamecoinDNS
	dnsServer            *dns.Server

	// MinerPubKey
	minerMu       sync.Mutex
	minerStopCh   chan struct{}
	minerWG       sync.WaitGroup
	minerDisabled bool

	// Step completion waiters
	stepWaitMu  sync.Mutex
	stepWaiters map[uint][]chan struct{}

	// Transaction confirmation tracking
	txConfirmMu      sync.Mutex
	pendingTxConfirm map[string]chan error

	logger zerolog.Logger
}

// Start implements peer.Service.
// It initializes message handlers, starts the listen loop, and begins anti-entropy/heartbeat if configured.
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
	// Register consensus messages
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosPrepareMessage{}, n.handlePaxosPrepare)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosPromiseMessage{}, n.handlePaxosPromise)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosProposeMessage{}, n.handlePaxosPropose)
	n.conf.MessageRegistry.RegisterMessageCallback(types.PaxosAcceptMessage{}, n.handlePaxosAccept)
	n.conf.MessageRegistry.RegisterMessageCallback(types.TLCMessage{}, n.handleTLC)

	//Namecoin handlers
	n.conf.MessageRegistry.RegisterMessageCallback(types.NamecoinTransactionMessage{}, n.handleNamecoinTransactionMessage)
	n.conf.MessageRegistry.RegisterMessageCallback(types.NamecoinBlockMessage{}, n.handleNamecoinBlockMessage)

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

	if n.conf.EnableMiner {
		// Allow mining when the node starts.
		n.minerMu.Lock()
		n.minerDisabled = false
		n.minerMu.Unlock()
		n.StartMiner()
	}

	// Start DNS server if configured.
	if addr := strings.TrimSpace(n.conf.DNSAddr); addr != "" && n.namecoinDNS != nil {
		srv, err := dns.ListenAndServe(addr, n.namecoinDNS)
		if err != nil {
			return err
		}
		n.dnsServer = srv
	}
	return nil
}

// ResolveDomain resolves a Namecoin domain using the live Namecoin chain state.
func (n *node) ResolveDomain(domain string) types.DNSResponse {
	if n.namecoinDNS == nil {
		return types.DNSResponse{Domain: domain, Status: types.DNSStatusInvalid}
	}
	return n.namecoinDNS.Resolve(strings.ToLower(strings.TrimSpace(domain)))
}

// NamecoinChainState returns the underlying Namecoin chain (testing helper).
func (n *node) NamecoinChainState() *NamecoinChain {
	return n.NamecoinChainService.GetLongestChain()
}

// DNSServerAddr returns the bound DNS server address (testing helper).
func (n *node) GetDNSAddr() string {
	if n.dnsServer == nil {
		return ""
	}
	return n.dnsServer.Addr()
}

func (n *node) GetMinerID() string {
	return n.conf.PoWConfig.PubKey
}

func (n *node) GetDomains() []types.NameRecord {
	chain := n.NamecoinChainService.GetLongestChain()
	domainsMap, _ := chain.SnapshotDomains()
	domains := make([]types.NameRecord, 0, len(domainsMap))
	for _, record := range domainsMap {
		domains = append(domains, record)
	}
	return domains
}

func (n *node) HandleNamecoinCommand(buf []byte) error {
	log.Printf("[DEBUG] Received raw bytes: %s", string(buf))

	var transaction SignedTransaction
	err := json.Unmarshal(buf, &transaction)
	if err != nil {
		log.Printf("[ERROR] Failed to unmarshal transaction: %v", err)
		return err
	}

	//log that we received a transaction
	log.Printf("Received Namecoin transaction: %+v", transaction)
	log.Printf("[DEBUG] Transaction fields - Type: %s, From: %s, Amount: %d, Payload: %s, TxID: %s",
		transaction.Type, transaction.From, transaction.Amount, string(transaction.Payload), transaction.TxID)

	err = n.transactionService.ValidateTransaction(&transaction)
	if err != nil {
		return err
	}

	// Use the miner's public key for balance verification instead of transaction.From
	// The miner receives mining rewards, so UTXOs are stored under the miner's key
	minerPubKey := n.conf.PoWConfig.PubKey
	log.Printf("[DEBUG] Using miner's public key for balance: %s", minerPubKey)

	inputs, outputs, err := n.transactionService.VerifyBalance(transaction.TxID, minerPubKey, transaction.Amount)
	if err != nil {
		return err
	}

	tx := types.Tx{
		From:    transaction.From,
		Type:    transaction.Type,
		Inputs:  inputs,
		Outputs: outputs,
		Amount:  transaction.Amount,
		Payload: transaction.Payload,
	}

	if err := n.transactionService.ValidateTxCommand(&tx); err != nil {
		return err
	}

	//build the txID
	txID, err := BuildTransactionID(&tx)
	if err != nil {
		return err
	}

	// Register for confirmation before broadcasting
	log.Printf("[DEBUG] Registering transaction %s for confirmation tracking", txID)
	confirmCh := n.registerTxConfirmation(txID)

	msg := types.NamecoinTransactionMessage{
		TxID: transaction.TxID,
		Tx:   tx,
	}

	marshaled, err := n.conf.MessageRegistry.MarshalMessage(msg)
	if err != nil {
		n.unregisterTxConfirmation(txID)
		return err
	}

	log.Printf("[DEBUG] Broadcasting transaction %s to network", txID)
	err = n.Broadcast(marshaled)
	if err != nil {
		log.Printf("[ERROR] Failed to broadcast transaction %s: %v", txID, err)
		n.unregisterTxConfirmation(txID)
		return err
	}

	// Wait for transaction to be included in a block (no timeout)
	log.Printf("[DEBUG] Waiting for transaction %s to be included in block (no timeout)", txID)
	err = <-confirmCh
	if err != nil {
		log.Printf("[ERROR] Transaction %s failed block inclusion: %v", txID, err)
		return xerrors.Errorf("transaction failed to be included in block: %w", err)
	}
	log.Printf("[SUCCESS] Transaction %s confirmed in block", txID)
	return nil
}

type pendingAck struct {
	packet transport.Packet
	dest   string
	timer  *time.Timer
}

func (n *node) StartMiner() {
	n.minerMu.Lock()
	if n.minerDisabled {
		n.minerMu.Unlock()
		return
	}
	if n.minerStopCh != nil {
		// MinerPubKey already running
		n.minerMu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	n.minerStopCh = stopCh
	n.minerWG.Add(1)
	n.minerMu.Unlock()

	go func(stop <-chan struct{}) {
		defer n.minerWG.Done()

		for {
			if err := n.MinerDoWork(stop); errors.Is(err, ErrMiningAborted) {
				return
			}

			select {
			case <-stop:
				return
			default:
			}
		}
	}(stopCh)
}

func (n *node) StopMiner() {
	n.minerMu.Lock()
	if n.minerStopCh == nil {
		n.minerMu.Unlock()
		return
	}

	n.logger.Debug().Msg("StopMiner has been called")
	close(n.minerStopCh)
	n.minerStopCh = nil
	n.minerMu.Unlock()

	n.minerWG.Wait()
}

func (n *node) DisableMiner() {
	n.minerMu.Lock()
	if n.minerDisabled {
		n.minerMu.Unlock()
		return
	}
	n.logger.Debug().Msg("DisableMiner has been called")
	n.minerDisabled = true
	stopCh := n.minerStopCh
	n.minerStopCh = nil
	n.minerMu.Unlock()

	if stopCh != nil {
		close(stopCh)
		n.minerWG.Wait()
	}
}

func (n *node) EnableMiner() {
	n.minerMu.Lock()
	n.minerDisabled = false
	n.minerMu.Unlock()
}

func (n *node) MinerDoWork(stop <-chan struct{}) error {
	// Bail out quickly if stop was already signaled.
	if stopped(stop) {
		return ErrMiningAborted
	}

	// Build base header from current chain head
	headHash, headHeight := n.NamecoinChainService.HeadSnapshot()

	nextHeight := headHeight + 1
	if headHash == nil {
		nextHeight = 0
	}
	base := types.BlockHeader{
		Height:    nextHeight,
		PrevHash:  headHash,
		Miner:     n.conf.PoWConfig.PubKey,
		Timestamp: time.Now().Unix(),
	}

	// Mine block
	block, err := n.namecoinConsensus.MineAndApply(stop, &base)
	if err != nil {
		return err
	}

	// If we were asked to stop while mining, drop the freshly mined block
	// instead of applying/broadcasting it.
	if stopped(stop) {
		return ErrMiningAborted
	}

	// Apply block locally before gossiping it so height advances even if
	// networking/logging is slow.
	changed, err := n.NamecoinChainService.AppendBlockToLongestChain(&block)
	if err != nil {
		// should not happen; log but continue
		log.Printf("Error applying mined block: %v", err)
		// Notify all transactions in this block that they failed
		log.Printf("[DEBUG] Notifying %d transactions in FAILED mined block", len(block.Transactions))
		for _, val := range block.Transactions {
			txID, txErr := BuildTransactionID(&val)
			if txErr == nil {
				n.notifyTxConfirmation(txID, err)
			}
		}
		return err
	}

	if !changed {
		// Our block lost the race → drop it silently
		return nil
	}

	// Only notify transactions if the block actually extended the longest chain
	log.Printf("[DEBUG] Successfully applied mined block at height %d, notifying %d transactions",
		block.Header.Height, len(block.Transactions))
	for _, val := range block.Transactions {
		if val.Type != "Reward" {
			txID, txErr := BuildTransactionID(&val)
			if txErr == nil {
				n.notifyTxConfirmation(txID, nil)
			}
		}
	}

	// Broadcast mined block
	msg := types.NamecoinBlockMessage{
		Block: block,
	}

	wire, err := n.conf.MessageRegistry.MarshalMessage(msg)
	if err != nil {
		return err
	}

	err = n.broadcastWithOptions(wire, true)
	if err != nil {
		return err
	}

	return nil
}

func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// acceptExpectedRumors processes expected rumors and returns those that were newly accepted.
// It validates each rumor and processes only those that are in sequence.
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

// forwardAcceptedRumorsOnce forwards accepted rumors to one random neighbor (not the source).
// It selects a random neighbor from the routing table and sends the rumors message.
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
// It determines what rumors to send and whether to continue gossip propagation.
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

// trackAck tracks acknowledgment for a packet with timeout-based retry.
// On timeout, it forwards the rumor to a different neighbor and re-arms tracking.
func (n *node) trackAck(pkt transport.Packet, dest string, rumors types.RumorsMessage) {
	if err := n.validateNode(false); err != nil {
		return
	}
	if pkt.Header == nil || strings.TrimSpace(pkt.Header.PacketID) == "" {
		return
	}
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
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
		newPkt := transport.Packet{Header: &header, Msg: &wire}
		_ = n.conf.Socket.Send(newDest, newPkt, time.Second)
		// Re-arm tracking for the newly selected destination so retries continue
		// until some peer acknowledges the rumor.
		n.trackAck(newPkt, newDest, rumors)
	}(pkt.Header.PacketID)
}

// listenLoop is the main packet receiving loop for the node.
// It continuously receives packets and processes them until the node stops.
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

// Stop implements peer.Service.
// It signals all goroutines to stop and waits for them to finish with a timeout.
func (n *node) Stop() error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	n.StopMiner()
	// Stop DNS server first to avoid new work during shutdown.
	if n.dnsServer != nil {
		_ = n.dnsServer.Close()
		n.dnsServer = nil
	}
	if n.namecoinDNS != nil {
		n.namecoinDNS.Stop()
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

// Unicast implements peer.Messaging.
// It routes a message to a destination using the routing table and sends it via the next hop.
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

// getNeighborsAndRelays extracts neighbors and relay set from routing table.
// Neighbors are direct peers (origin==relay), while relaySet contains all known relays.
func (n *node) getNeighborsAndRelays(nodeAddr string) (neighbors []string, relaySet map[string]struct{}) {
	if err := n.validateNode(false); err != nil {
		return nil, nil
	}
	nodeAddr = strings.TrimSpace(nodeAddr)
	if nodeAddr == "" {
		return nil, nil
	}
	n.mu.RLock()
	neighbors = make([]string, 0, len(n.routingTable))
	relaySet = make(map[string]struct{}, len(n.routingTable))
	for origin, relay := range n.routingTable {
		relay = strings.TrimSpace(relay)
		if relay == "" || relay == nodeAddr {
			continue
		}
		relaySet[relay] = struct{}{}
		if origin == relay {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()
	return neighbors, relaySet
}

// buildRumorMessage creates and stores a rumor message.
// It increments the sequence number for the origin and stores the rumor for anti-entropy.
func (n *node) buildRumorMessage(nodeAddr string, msg transport.Message) (
	transport.Message, types.RumorsMessage, error) {
	if err := n.validateNode(true); err != nil {
		return transport.Message{}, types.RumorsMessage{}, err
	}
	nodeAddr = strings.TrimSpace(nodeAddr)
	if nodeAddr == "" {
		return transport.Message{}, types.RumorsMessage{}, xerrors.Errorf("invalid origin")
	}
	if strings.TrimSpace(msg.Type) == "" {
		return transport.Message{}, types.RumorsMessage{}, xerrors.Errorf("invalid message: empty type")
	}
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
		return transport.Message{}, types.RumorsMessage{}, err
	}

	// store the rumor so anti-entropy/catch-up can resend it later
	n.mu.Lock()
	n.rumors[nodeAddr][seq] = rumor
	n.mu.Unlock()

	return wireMsg, rumorsMsg, nil
}

// isPaxosMessage checks if a message type is a Paxos/TLC message.
// Returns true for paxosprepare, paxospromise, paxospropose, paxosaccept, tlc, and private messages.
func isPaxosMessage(msgType string) bool {
	switch msgType {
	case "paxosprepare", "paxospromise", "paxospropose", "paxosaccept", "tlc", "private":
		return true
	}
	return false
}

// determineBroadcastTargets determines which neighbors to send to.
// For Paxos messages, it sends to all relays; for others, it picks a random neighbor.
func (n *node) determineBroadcastTargets(isPaxos bool, neighbors []string, relaySet map[string]struct{}) []string {
	var targets []string
	if isPaxos {
		// Deliver Paxos/TLC messages to every known next hop (reliable flood)
		if len(relaySet) == 0 && len(neighbors) > 0 {
			// Fallback: if we somehow lost relay info, at least push to direct peers
			for _, neighbor := range neighbors {
				relaySet[neighbor] = struct{}{}
			}
		}
		if len(relaySet) > 0 {
			targets = make([]string, 0, len(relaySet))
			for relay := range relaySet {
				targets = append(targets, relay)
			}
		}
	} else if len(neighbors) > 0 {
		// Traditional rumor mongering: pick a random direct neighbor
		targets = []string{neighbors[int(time.Now().UnixNano())%len(neighbors)]}
	} else if len(relaySet) > 0 {
		// If we have no direct neighbor entries, fall back to any known relay
		targets = make([]string, 0, len(relaySet))
		for relay := range relaySet {
			targets = append(targets, relay)
			break
		}
	}
	return targets
}

// sendToTargets sends the wire message to all targets.
// Optionally tracks acknowledgments for rumor messages based on the trackAck flag.
func (n *node) sendToTargets(nodeAddr string, wireMsg transport.Message,
	targets []string, trackAck bool, rumorsMsg types.RumorsMessage) {
	if err := n.validateNode(false); err != nil {
		return
	}
	nodeAddr = strings.TrimSpace(nodeAddr)
	if nodeAddr == "" || len(targets) == 0 {
		return
	}
	if strings.TrimSpace(wireMsg.Type) == "" {
		return
	}
	for _, neighbor := range targets {
		header := transport.NewHeader(nodeAddr, nodeAddr, neighbor)
		pkt := transport.Packet{Header: &header, Msg: &wireMsg}

		// set up ack waiting if configured
		if trackAck && n.conf.AckTimeout > 0 {
			n.trackAck(pkt, neighbor, rumorsMsg)
		}
		_ = n.conf.Socket.Send(neighbor, pkt, time.Second)
	}
}

// Broadcast implements peer.Messaging.
// It broadcasts a message using rumor mongering to propagate it through the network.
func (n *node) Broadcast(msg transport.Message) error {
	return n.broadcastWithOptions(msg, false)
}

// broadcastWithOptions centralizes the broadcast logic and optionally skips local processing.
// It builds rumor messages, determines targets, and sends to neighbors.
func (n *node) broadcastWithOptions(msg transport.Message, skipLocalProcessing bool) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	if strings.TrimSpace(msg.Type) == "" {
		return xerrors.Errorf("invalid message: empty type")
	}
	nodeAddr := n.conf.Socket.GetAddress()

	neighbors, relaySet := n.getNeighborsAndRelays(nodeAddr)
	wireMsg, rumorsMsg, err := n.buildRumorMessage(nodeAddr, msg)
	if err != nil {
		return err
	}

	isPaxos := isPaxosMessage(msg.Type)
	targets := n.determineBroadcastTargets(isPaxos, neighbors, relaySet)
	n.sendToTargets(nodeAddr, wireMsg, targets, !isPaxos, rumorsMsg)

	// Process the embedded message locally after sending to neighbors. The rumor
	// is already stored above for anti-entropy purposes.
	if !skipLocalProcessing {
		localHeader := transport.NewHeader(nodeAddr, nodeAddr, nodeAddr)
		_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: &localHeader, Msg: &msg})
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

// AddPeer implements peer.Messaging.
// It adds peer addresses to the routing table as direct neighbors.
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

// GetRoutingTable implements peer.Messaging.
// Returns a copy of the routing table mapping destinations to relay addresses.
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

// SetRoutingEntry implements peer.Messaging.
// Sets or removes a routing table entry mapping an origin to a relay address.
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

// registerTxConfirmation registers a transaction to wait for confirmation
func (n *node) registerTxConfirmation(txID string) chan error {
	n.txConfirmMu.Lock()
	defer n.txConfirmMu.Unlock()

	ch := make(chan error, 1)
	n.pendingTxConfirm[txID] = ch
	log.Printf("[DEBUG] Registered txID=%s for confirmation (total pending: %d)", txID, len(n.pendingTxConfirm))
	return ch
}

// unregisterTxConfirmation removes a transaction from the confirmation wait list
func (n *node) unregisterTxConfirmation(txID string) {
	n.txConfirmMu.Lock()
	defer n.txConfirmMu.Unlock()

	if ch, ok := n.pendingTxConfirm[txID]; ok {
		log.Printf("[DEBUG] Unregistering txID=%s (cleaning up)", txID)
		close(ch)
		delete(n.pendingTxConfirm, txID)
	} else {
		log.Printf("[WARN] Attempted to unregister txID=%s but not found in pending map", txID)
	}
}

// notifyTxConfirmation notifies waiting handlers that a transaction was included in a block
func (n *node) notifyTxConfirmation(txID string, err error) {
	n.txConfirmMu.Lock()
	defer n.txConfirmMu.Unlock()

	if ch, ok := n.pendingTxConfirm[txID]; ok {
		if err != nil {
			log.Printf("[DEBUG] Notifying txID=%s with ERROR: %v", txID, err)
		} else {
			log.Printf("[DEBUG] Notifying txID=%s with SUCCESS (block applied)", txID)
		}
		ch <- err
		close(ch)
		delete(n.pendingTxConfirm, txID)
	} else {
		log.Printf("[DEBUG] Attempted to notify txID=%s but not in pending map "+
			"(may have timed out or already processed)", txID)
	}
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
