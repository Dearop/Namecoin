//go:build performance
// +build performance

package perf

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

type networkShape string

const (
	networkShapeMesh networkShape = "mesh"
	networkShapeLine networkShape = "line"
	networkShapeStar networkShape = "star"
)

type scenarioConfig struct {
	nodeCount   int
	minerCount  int
	target      *big.Int
	shape       networkShape
	antiEntropy time.Duration
	heartbeat   time.Duration
	counting    bool
}

type namecoinHarness struct {
	t         *testing.T
	nodes     []z.TestNode
	transport transport.Transport
	shape     networkShape
	counting  *countingTransport
}

func newNamecoinHarness(t *testing.T, cfg scenarioConfig) *namecoinHarness {
	if cfg.nodeCount == 0 {
		cfg.nodeCount = 1
	}
	if cfg.minerCount == 0 {
		cfg.minerCount = cfg.nodeCount
	}
	if cfg.target == nil {
		cfg.target = new(big.Int).Lsh(big.NewInt(1), 252)
	}
	if cfg.shape == "" {
		cfg.shape = networkShapeMesh
	}
	if cfg.antiEntropy == 0 {
		cfg.antiEntropy = 300 * time.Millisecond
	}
	if cfg.heartbeat == 0 {
		cfg.heartbeat = 250 * time.Millisecond
	}

	var trans transport.Transport
	var ct *countingTransport
	if cfg.counting {
		ct = newCountingTransport()
		trans = ct
	} else {
		trans = channelFac()
	}

	nodes := make([]z.TestNode, cfg.nodeCount)
	for i := 0; i < cfg.nodeCount; i++ {
		minerEnabled := i < cfg.minerCount
		node := z.NewTestNode(t, peerFac, trans, "127.0.0.1:0",
			z.WithPoWConfig(peer.PoWConfig{
				Target: cfg.target,
				PubKey: fmt.Sprintf("miner-%d", i),
			}),
			z.WithEnableMiner(minerEnabled),
			z.WithAntiEntropy(cfg.antiEntropy),
			z.WithHeartbeat(cfg.heartbeat),
		)
		nodes[i] = node
	}

	connectNodes(nodes, cfg.shape)

	return &namecoinHarness{
		t:         t,
		nodes:     nodes,
		transport: trans,
		shape:     cfg.shape,
		counting:  ct,
	}
}

func (h *namecoinHarness) stop() {
	for _, n := range h.nodes {
		func(node z.TestNode) {
			defer func() {
				if r := recover(); r != nil {
					// Defensive: avoid test panics from miner WaitGroup reuse during teardown.
				}
			}()
			node.Stop()
		}(n)
	}
}

func connectNodes(nodes []z.TestNode, shape networkShape) {
	switch shape {
	case networkShapeLine:
		for i := 0; i < len(nodes)-1; i++ {
			nodes[i].AddPeer(nodes[i+1].GetAddr())
			nodes[i+1].AddPeer(nodes[i].GetAddr())
		}
	case networkShapeStar:
		if len(nodes) == 0 {
			return
		}
		hub := nodes[0].GetAddr()
		for i := 1; i < len(nodes); i++ {
			nodes[0].AddPeer(nodes[i].GetAddr())
			nodes[i].AddPeer(hub)
		}
	default:
		for _, n1 := range nodes {
			for _, n2 := range nodes {
				n1.AddPeer(n2.GetAddr())
			}
		}
	}
}

func buildNameNewTx(t *testing.T, minerID, domain, salt string, ttl uint64) (types.Tx, string) {
	commitment := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))
	tx := types.Tx{
		From:    minerID,
		Type:    impl.NameNewCommandName,
		Amount:  1,
		Payload: json.RawMessage(mustJSONMarshal(t, impl.NameNew{Commitment: commitment, TTL: ttl})),
		Outputs: []types.TxOutput{{To: minerID, Amount: 1}},
	}

	txID, err := impl.BuildTransactionID(&tx)
	require.NoError(t, err)
	return tx, txID
}

func buildNameFirstUpdateTx(t *testing.T, minerID, domain, salt, nameNewTxID, ip string, ttl uint64) (types.Tx, string) {
	firstUpdate := impl.NameFirstUpdate{
		Domain: domain,
		Salt:   salt,
		IP:     ip,
		TTL:    ttl,
		TxID:   nameNewTxID,
	}
	tx := types.Tx{
		From:    minerID,
		Type:    impl.NameFirstUpdateCommandName,
		Amount:  1,
		Inputs:  []types.TxInput{{TxID: nameNewTxID, Index: 0}},
		Outputs: []types.TxOutput{{To: minerID, Amount: 1}},
		Payload: json.RawMessage(mustJSONMarshal(t, firstUpdate)),
	}

	txID, err := impl.BuildTransactionID(&tx)
	require.NoError(t, err)
	return tx, txID
}

func buildNameUpdateTx(t *testing.T, minerID, domain, ip, prevTxID string, ttl uint64) (types.Tx, string) {
	update := impl.NameUpdate{
		Domain: domain,
		IP:     ip,
		TTL:    ttl,
	}
	tx := types.Tx{
		From:    minerID,
		Type:    impl.NameUpdateCommandName,
		Amount:  1,
		Inputs:  []types.TxInput{{TxID: prevTxID, Index: 0}},
		Outputs: []types.TxOutput{{To: minerID, Amount: 1}},
		Payload: json.RawMessage(mustJSONMarshal(t, update)),
	}

	txID, err := impl.BuildTransactionID(&tx)
	require.NoError(t, err)
	return tx, txID
}

func broadcastTx(t *testing.T, node z.TestNode, tx types.Tx, txID string) time.Time {
	msg := types.NamecoinTransactionMessage{
		Type:      tx.Type,
		From:      tx.From,
		Amount:    tx.Amount,
		Payload:   tx.Payload,
		Inputs:    tx.Inputs,
		Outputs:   tx.Outputs,
		Pk:        tx.Pk,
		TxID:      txID,
		Signature: tx.Signature,
	}
	wire, err := node.GetRegistry().MarshalMessage(msg)
	require.NoError(t, err)
	require.NoError(t, node.Broadcast(wire))
	return time.Now()
}

func waitForTxApplied(nodes []z.TestNode, txID string, timeout time.Duration) (uint64, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, node := range nodes {
			chain := node.NamecoinChainState()
			if chain == nil {
				continue
			}
			state := chain.State()
			if state == nil {
				continue
			}
			if state.IsTxApplied(txID) {
				return state.CurrentHeight(), true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, false
}

func waitForDomain(nodes []z.TestNode, domain string, timeout time.Duration) (uint64, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, node := range nodes {
			chain := node.NamecoinChainState()
			if chain == nil {
				continue
			}
			state := chain.State()
			if state == nil {
				continue
			}
			if _, ok := state.NameLookup(domain); ok {
				return state.CurrentHeight(), true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, false
}

func waitForConfirmations(nodes []z.TestNode, inclusionHeight, confirmations uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allReached := true
		for _, node := range nodes {
			chain := node.NamecoinChainState()
			if chain == nil {
				allReached = false
				break
			}
			if chain.HeadHeight() < inclusionHeight+confirmations {
				allReached = false
				break
			}
		}
		if allReached {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func mustJSONMarshal(t *testing.T, val any) []byte {
	raw, err := json.Marshal(val)
	require.NoError(t, err)
	return raw
}

type trafficSnapshot struct {
	Node          string
	SentBytes     int64
	RecvBytes     int64
	SentMessages  int64
	RecvMessages  int64
	TotalMessages int64
}

type networkMetrics struct {
	TotalSentBytes    int64
	TotalRecvBytes    int64
	TotalSentMessages int64
	TotalRecvMessages int64
	ByNode            []trafficSnapshot
}

type countingTransport struct {
	sync.RWMutex
	incomings map[string]chan transport.Packet
	stats     map[string]*trafficStats
}

type trafficStats struct {
	sentBytes     atomic.Int64
	recvBytes     atomic.Int64
	sentMessages  atomic.Int64
	recvMessages  atomic.Int64
	recordedIns   []transport.Packet
	recordedOuts  []transport.Packet
	recordedMutex sync.Mutex
}

func newCountingTransport() *countingTransport {
	return &countingTransport{
		incomings: make(map[string]chan transport.Packet),
		stats:     make(map[string]*trafficStats),
	}
}

var counter uint32

func (t *countingTransport) CreateSocket(address string) (transport.ClosableSocket, error) {
	t.Lock()
	defer t.Unlock()

	if strings.HasSuffix(address, ":0") {
		address = address[:len(address)-2]
		port := atomic.AddUint32(&counter, 1)
		address = fmt.Sprintf("%s:%d", address, port)
	}
	t.incomings[address] = make(chan transport.Packet, 200)
	t.ensureStats(address)

	return &countingSocket{
		countingTransport: t,
		myAddr:            address,
	}, nil
}

func (t *countingTransport) ensureStats(addr string) {
	if _, ok := t.stats[addr]; !ok {
		t.stats[addr] = &trafficStats{}
	}
}

func (t *countingTransport) snapshot() networkMetrics {
	t.RLock()
	defer t.RUnlock()

	snap := networkMetrics{
		ByNode: make([]trafficSnapshot, 0, len(t.stats)),
	}
	for addr, st := range t.stats {
		nodeSnap := trafficSnapshot{
			Node:          addr,
			SentBytes:     st.sentBytes.Load(),
			RecvBytes:     st.recvBytes.Load(),
			SentMessages:  st.sentMessages.Load(),
			RecvMessages:  st.recvMessages.Load(),
			TotalMessages: st.sentMessages.Load() + st.recvMessages.Load(),
		}
		snap.TotalSentBytes += nodeSnap.SentBytes
		snap.TotalRecvBytes += nodeSnap.RecvBytes
		snap.TotalSentMessages += nodeSnap.SentMessages
		snap.TotalRecvMessages += nodeSnap.RecvMessages
		snap.ByNode = append(snap.ByNode, nodeSnap)
	}
	return snap
}

type countingSocket struct {
	*countingTransport
	myAddr string
}

func (s *countingSocket) Close() error {
	s.Lock()
	defer s.Unlock()

	delete(s.incomings, s.myAddr)
	return nil
}

func (s *countingSocket) Send(dest string, pkt transport.Packet, timeout time.Duration) error {
	s.RLock()
	to, ok := s.incomings[dest]
	s.RUnlock()
	if !ok {
		return xerrors.Errorf("%s is not listening", dest)
	}

	if timeout == 0 {
		timeout = math.MaxInt64
	}

	size := packetSize(pkt)
	stat := s.getStats(s.myAddr)
	stat.sentBytes.Add(int64(size))
	stat.sentMessages.Add(1)
	stat.recordOut(pkt)

	select {
	case to <- pkt.Copy():
	case <-time.After(timeout):
		return transport.TimeoutError(timeout)
	}
	return nil
}

func (s *countingSocket) Recv(timeout time.Duration) (transport.Packet, error) {
	s.RLock()
	myChan := s.incomings[s.myAddr]
	s.RUnlock()

	select {
	case <-time.After(timeout):
		return transport.Packet{}, transport.TimeoutError(timeout)
	case pkt := <-myChan:
		size := packetSize(pkt)
		stat := s.getStats(s.myAddr)
		stat.recvBytes.Add(int64(size))
		stat.recvMessages.Add(1)
		stat.recordIn(pkt)
		return pkt, nil
	}
}

func (s *countingSocket) GetAddress() string {
	return s.myAddr
}

func (s *countingSocket) GetIns() []transport.Packet {
	stat := s.getStats(s.myAddr)
	stat.recordedMutex.Lock()
	defer stat.recordedMutex.Unlock()
	return append([]transport.Packet(nil), stat.recordedIns...)
}

func (s *countingSocket) GetOuts() []transport.Packet {
	stat := s.getStats(s.myAddr)
	stat.recordedMutex.Lock()
	defer stat.recordedMutex.Unlock()
	return append([]transport.Packet(nil), stat.recordedOuts...)
}

func (s *countingSocket) getStats(addr string) *trafficStats {
	s.Lock()
	defer s.Unlock()
	s.ensureStats(addr)
	return s.stats[addr]
}

func (ts *trafficStats) recordIn(pkt transport.Packet) {
	ts.recordedMutex.Lock()
	defer ts.recordedMutex.Unlock()
	ts.recordedIns = append(ts.recordedIns, pkt.Copy())
}

func (ts *trafficStats) recordOut(pkt transport.Packet) {
	ts.recordedMutex.Lock()
	defer ts.recordedMutex.Unlock()
	ts.recordedOuts = append(ts.recordedOuts, pkt.Copy())
}

func packetSize(pkt transport.Packet) int {
	if b, err := pkt.Marshal(); err == nil {
		return len(b)
	}

	size := 0
	if pkt.Header != nil {
		size += len(pkt.Header.Source) + len(pkt.Header.RelayedBy) + len(pkt.Header.Destination) + len(pkt.Header.PacketID)
	}
	if pkt.Msg != nil {
		size += len(pkt.Msg.Type) + len(pkt.Msg.Payload)
	}
	return size
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func uniqueHeadHashes(nodes []z.TestNode) map[string]struct{} {
	heads := make(map[string]struct{})
	for _, node := range nodes {
		chain := node.NamecoinChainState()
		if chain == nil {
			continue
		}
		hash := chain.HeadHash()
		heads[string(hash)] = struct{}{}
	}
	return heads
}

// waitUntilConvergedStable waits for a single head hash shared by all nodes at
// or above targetHeight and requires it to remain stable for the given window.
func waitUntilConvergedStable(nodes []z.TestNode, targetHeight uint64, stabilityWindow time.Duration, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	var stableSince time.Time

	for time.Now().Before(deadline) {
		heads := uniqueHeadHashes(nodes)
		if len(heads) == 1 {
			allAtHeight := true
			for _, node := range nodes {
				chain := node.NamecoinChainState()
				if chain == nil || chain.HeadHeight() < targetHeight {
					allAtHeight = false
					break
				}
			}
			if allAtHeight {
				if stableSince.IsZero() {
					stableSince = time.Now()
				} else if time.Since(stableSince) >= stabilityWindow {
					return true
				}
			} else {
				stableSince = time.Time{}
			}
		} else {
			stableSince = time.Time{}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// waitUntilConverged waits until all nodes share a single head hash at or above targetHeight.
func waitUntilConverged(nodes []z.TestNode, targetHeight uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		heads := uniqueHeadHashes(nodes)
		if len(heads) == 1 {
			allAtHeight := true
			for _, node := range nodes {
				chain := node.NamecoinChainState()
				if chain == nil || chain.HeadHeight() < targetHeight {
					allAtHeight = false
					break
				}
			}
			if allAtHeight {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// forceCompetingBlocks mines two blocks at the same target height on different nodes to create a fork.
func forceCompetingBlocks(t *testing.T, nodes []z.TestNode, targetHeight uint64, timeout time.Duration) bool {
	if len(nodes) < 2 {
		return false
	}
	deadline := time.Now().Add(timeout)
	miners := []z.TestNode{nodes[0], nodes[1]}

	// Ensure both miners are running.
	for _, m := range miners {
		if ctl, ok := m.Peer.(interface {
			EnableMiner()
			StartMiner()
		}); ok {
			ctl.EnableMiner()
			ctl.StartMiner()
		}
	}

	for time.Now().Before(deadline) {
		for _, node := range miners {
			chain := node.NamecoinChainState()
			if chain == nil {
				continue
			}
			if chain.HeadHeight() >= targetHeight {
				heads := uniqueHeadHashes(nodes)
				if len(heads) >= 2 {
					return true
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
