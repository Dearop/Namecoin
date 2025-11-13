package impl

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

// getQuorum returns quorum size based on configuration TotalPeers and optional PaxosThreshold
func (n *node) getQuorum() int {
	total := n.conf.TotalPeers
	if total == 0 {
		total = 1
	}
	if n.conf.PaxosThreshold != nil {
		return n.conf.PaxosThreshold(total)
	}
	// default N/2 + 1
	return int(total/2) + 1
}

// handlePaxosPrepare processes incoming PaxosPrepareMessage (acceptor role)
func (n *node) handlePaxosPrepare(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	var prep *types.PaxosPrepareMessage
	if p, ok := m.(*types.PaxosPrepareMessage); ok && p != nil {
		prep = p
	} else if v, ok := m.(types.PaxosPrepareMessage); ok {
		prep = &v
	} else {
		return nil
	}
	step := prep.Step
	n.mu.RLock()
	cur := n.currentStep
	n.mu.RUnlock()
	if step != cur {
		return nil
	}
	// Initialize maps
	n.mu.Lock()
	if n.promisedID == nil {
		n.promisedID = make(map[uint]uint)
	}
	if n.acceptedID == nil {
		n.acceptedID = make(map[uint]uint)
	}
	if n.acceptedValue == nil {
		n.acceptedValue = make(map[uint]*types.PaxosValue)
	}
	// Promise only if ID is higher than any promised so far
	prevPromise := n.promisedID[step]
	if step >= 4 && os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] prepare node=%s step=%d id=%d from=%s promised=%d",
			n.conf.Socket.GetAddress(), step, prep.ID, strings.TrimSpace(prep.Source), prevPromise)
	}
	if prep.ID <= prevPromise {
		n.mu.Unlock()
		return nil
	}
	n.promisedID[step] = prep.ID
	accID := n.acceptedID[step]
	var accVal *types.PaxosValue
	if v, ok := n.acceptedValue[step]; ok && v != nil {
		// copy value
		vv := *v
		accVal = &vv
	}
	n.mu.Unlock()
	if step >= 4 && os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] prepare node=%s step=%d accepted id=%d newPromise=%d",
			n.conf.Socket.GetAddress(), step, prep.ID, prep.ID)
	}

	// Reply with a PROMISE via private broadcast to proposer source
	promise := types.PaxosPromiseMessage{Step: step, ID: prep.ID, AcceptedID: accID, AcceptedValue: accVal}
	wirePromise, err := n.conf.MessageRegistry.MarshalMessage(promise)
	if err != nil {
		return err
	}
	recipients := map[string]struct{}{}
	dest := strings.TrimSpace(prep.Source)
	if dest != "" {
		recipients[dest] = struct{}{}
	}
	priv := types.PrivateMessage{Recipients: recipients, Msg: &wirePromise}
	tmsg, err := n.conf.MessageRegistry.MarshalMessage(priv)
	if err != nil {
		return err
	}
	n.sendPromiseMessage(dest, wirePromise, tmsg)
	return nil
}

// handlePaxosPropose processes incoming PaxosProposeMessage (acceptor role)
func (n *node) handlePaxosPropose(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	var prop *types.PaxosProposeMessage
	if p, ok := m.(*types.PaxosProposeMessage); ok && p != nil {
		prop = p
	} else if v, ok := m.(types.PaxosProposeMessage); ok {
		prop = &v
	} else {
		return nil
	}
	step := prop.Step
	n.mu.RLock()
	cur := n.currentStep
	n.mu.RUnlock()
	if step != cur {
		return nil
	}
	n.mu.Lock()
	if n.promisedID == nil {
		n.promisedID = make(map[uint]uint)
	}
	if n.acceptedID == nil {
		n.acceptedID = make(map[uint]uint)
	}
	if n.acceptedValue == nil {
		n.acceptedValue = make(map[uint]*types.PaxosValue)
	}
	if prop.ID != n.promisedID[step] {
		n.mu.Unlock()
		return nil
	}
	n.acceptedID[step] = prop.ID
	vv := prop.Value
	n.acceptedValue[step] = &vv
	n.mu.Unlock()

	// Broadcast ACCEPT: if the propose originated from self, delay slightly so
	// that the PROPOSE rumor is recorded before ACCEPT for ordering in tests.
	accept := types.PaxosAcceptMessage{Step: step, ID: prop.ID, Value: prop.Value}
	if msg, err := n.conf.MessageRegistry.MarshalMessage(accept); err == nil {
		self := n.conf.Socket.GetAddress()
		src := ""
		if pkt.Header != nil {
			src = strings.TrimSpace(pkt.Header.Source)
		}
		if src == self {
			go func(m transport.Message) {
				time.Sleep(time.Millisecond)
				_ = n.Broadcast(m)
			}(msg)
		} else {
			_ = n.Broadcast(msg)
		}
	}
	return nil
}

// handlePaxosPromise processes incoming PaxosPromiseMessage (proposer role)
func (n *node) handlePaxosPromise(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	var prm *types.PaxosPromiseMessage
	if p, ok := m.(*types.PaxosPromiseMessage); ok && p != nil {
		prm = p
	} else if v, ok := m.(types.PaxosPromiseMessage); ok {
		prm = &v
	} else {
		return nil
	}
	step := prm.Step
	n.mu.RLock()
	cur := n.currentStep
	n.mu.RUnlock()
	if step != cur {
		return nil
	}
	src := ""
	if pkt.Header != nil {
		src = strings.TrimSpace(pkt.Header.Source)
	}
	n.mu.Lock()
	// Only process during phase 1 and for current proposal ID
	if !n.shouldProcessPromise(step, prm.ID) {
		n.logIgnoredPromise(step, prm.ID, src)
		n.mu.Unlock()
		return nil
	}
	if n.proposerPromises == nil {
		n.proposerPromises = make(map[uint]map[uint]map[string]struct{})
	}
	if n.proposerPromises[step] == nil {
		n.proposerPromises[step] = make(map[uint]map[string]struct{})
	}
	if n.proposerPromises[step][prm.ID] == nil {
		n.proposerPromises[step][prm.ID] = make(map[string]struct{})
	}
	if src != "" {
		n.proposerPromises[step][prm.ID][src] = struct{}{}
	}
	// Adopt highest AcceptedID value if any
	if n.proposerHighestAcceptedID == nil {
		n.proposerHighestAcceptedID = make(map[uint]uint)
	}
	if n.proposerValue == nil {
		n.proposerValue = make(map[uint]types.PaxosValue)
	}
	if prm.AcceptedValue != nil && prm.AcceptedID > n.proposerHighestAcceptedID[step] {
		n.proposerHighestAcceptedID[step] = prm.AcceptedID
		n.proposerValue[step] = *prm.AcceptedValue
	}
	cnt := len(n.proposerPromises[step][prm.ID])
	phase := 0
	if n.proposerPhase != nil {
		phase = n.proposerPhase[step]
	}
	// Snapshot selected value (adopted or initially set)
	value := n.proposerValue[step]
	n.mu.Unlock()

	if step >= 4 && os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] promise node=%s step=%d id=%d cnt=%d quorum=%d src=%s phase=%d",
			n.conf.Socket.GetAddress(), step, prm.ID, cnt, n.getQuorum(), src, phase)
	}
	if cnt >= n.getQuorum() {
		n.broadcastProposeAndAdvancePhase(step, prm.ID, value)
	}
	return nil
}

// handlePaxosAccept processes ACCEPT messages (learner role). TLC handling will be implemented later.
func (n *node) handlePaxosAccept(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	var acc *types.PaxosAcceptMessage
	if a, ok := m.(*types.PaxosAcceptMessage); ok && a != nil {
		acc = a
	} else if v, ok := m.(types.PaxosAcceptMessage); ok {
		acc = &v
	} else {
		return nil
	}
	step := acc.Step
	n.mu.RLock()
	cur := n.currentStep
	n.mu.RUnlock()
	if step != cur {
		return nil
	}
	src := ""
	if pkt.Header != nil {
		src = strings.TrimSpace(pkt.Header.Source)
	}
	propCnt, globalCnt, tlcAlready := n.trackAcceptCounts(step, acc.ID, src)

	if step >= 4 && os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] accept node=%s step=%d id=%d src=%s cnt=%d quorum=%d tlcBroadcasted=%v",
			n.conf.Socket.GetAddress(), step, acc.ID, src, globalCnt, n.getQuorum(), tlcAlready)
	}
	// If proposer reached quorum, mark done
	if propCnt >= n.getQuorum() {
		n.mu.Lock()
		if n.proposerPhase == nil {
			n.proposerPhase = make(map[uint]int)
		}
		n.proposerPhase[step] = 3
		n.mu.Unlock()
	}
	// Any peer: on accept quorum for current step, broadcast TLC once with block
	// Delay TLC broadcast slightly to ensure Accept message is sent first
	if globalCnt >= n.getQuorum() && !tlcAlready {
		block := n.buildBlock(step, acc.Value)
		// Use goroutine to ensure Accept broadcast completes before TLC broadcast
		go func() {
			time.Sleep(time.Millisecond)
			n.broadcastTLCOnce(step, block)
		}()
	}
	return nil
}

// handleTLC processes TLC messages (to be implemented later)
func (n *node) handleTLC(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	var tlc *types.TLCMessage
	if t, ok := m.(*types.TLCMessage); ok && t != nil {
		tlc = t
	} else if v, ok := m.(types.TLCMessage); ok {
		tlc = &v
	} else {
		return nil
	}
	step := tlc.Step
	n.mu.RLock()
	cur := n.currentStep
	n.mu.RUnlock()
	if step < cur {
		return nil // ignore past steps
	}
	src := ""
	if pkt.Header != nil {
		src = strings.TrimSpace(pkt.Header.Source)
	}
	// If source is empty (shouldn't happen, but handle it), use our own address
	if src == "" {
		src = n.conf.Socket.GetAddress()
	}
	n.mu.Lock()
	if n.tlcCount == nil {
		n.tlcCount = make(map[uint]map[string]struct{})
	}
	if n.tlcBlock == nil {
		n.tlcBlock = make(map[uint]types.BlockchainBlock)
	}
	if n.tlcCount[step] == nil {
		n.tlcCount[step] = make(map[string]struct{})
	}
	// Count this TLC message (using map to avoid duplicates from same source)
	n.tlcCount[step][src] = struct{}{}
	// store latest block for this step (they should be the same)
	n.tlcBlock[step] = tlc.Block
	cnt := len(n.tlcCount[step])
	already := n.tlcBroadcasted[step]
	n.mu.Unlock()

	if step >= 4 && os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] handleTLC node=%s step=%d src=%s cnt=%d quorum=%d already=%v cur=%d",
			n.conf.Socket.GetAddress(), step, src, cnt, n.getQuorum(), already, cur)
	}
	// Always store TLC messages for future steps (step > cur) - they will be processed sequentially
	// when we catch up in commitStepAndAdvance. This ensures we can catch up even if messages arrive out of order.
	if cnt >= n.getQuorum() {
		// If quorum on current step, commit and maybe broadcast TLC once
		if step == cur {
			if !already {
				// We haven't broadcast yet, so broadcast first and commit immediately.
				// commitStepAndAdvance is idempotent if invoked again when the locally
				// processed broadcast re-enters handleTLC.
				n.broadcastTLCOnce(step, tlc.Block)
				n.commitStepAndAdvance(step, tlc.Block)
			} else {
				// Already broadcast by us or someone else, just commit
				n.commitStepAndAdvance(step, tlc.Block)
			}
		}
		// If future step, we will commit when we catch up (in commitStepAndAdvance loop)
	}
	return nil
}

// buildBlock constructs a block for given step from value and previous last hash
func (n *node) buildBlock(step uint, value types.PaxosValue) types.BlockchainBlock {
	prev := n.conf.Storage.GetBlockchainStore().Get(storage.LastBlockKey)
	block := types.BlockchainBlock{Index: step, Value: value}
	if len(prev) == 32 {
		block.PrevHash = append([]byte{}, prev...)
	} else {
		block.PrevHash = make([]byte, 32)
	}
	h := sha256.New()
	// Hash = SHA256(Index || Filename || Metahash || Prevhash)
	// Index as decimal string
	h.Write([]byte(strconv.Itoa(int(step))))
	h.Write([]byte(value.Filename))
	h.Write([]byte(value.Metahash))
	h.Write(block.PrevHash)
	block.Hash = h.Sum(nil)
	return block
}

func (n *node) broadcastTLCOnce(step uint, block types.BlockchainBlock) {
	n.mu.Lock()
	if n.tlcBroadcasted == nil {
		n.tlcBroadcasted = make(map[uint]bool)
	}
	if n.tlcBroadcasted[step] {
		n.mu.Unlock()
		return
	}
	n.tlcBroadcasted[step] = true
	n.mu.Unlock()

	if step >= 4 && os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] broadcastTLC node=%s step=%d", n.conf.Socket.GetAddress(), step)
	}
	tlc := types.TLCMessage{Step: step, Block: block}
	if msg, err := n.conf.MessageRegistry.MarshalMessage(tlc); err == nil {
		// Broadcast to neighbors (will also process locally via handleTLC)
		_ = n.Broadcast(msg)
		// handleTLC will process the local broadcast, count it, check for quorum, and commit if needed
	}
}

// trackAcceptCounts tracks both proposer and global accept counts
func (n *node) trackAcceptCounts(step uint, id uint, src string) (propCnt int, globalCnt int, tlcAlready bool) {
	n.mu.Lock()
	// Track proposer accept quorum for proposer completion bookkeeping (only if we're the proposer)
	if n.proposerPhase != nil && n.proposerPhase[step] == 2 && n.proposerID != nil && n.proposerID[step] == id {
		if n.proposerAccepts == nil {
			n.proposerAccepts = make(map[uint]map[uint]map[string]struct{})
		}
		if n.proposerAccepts[step] == nil {
			n.proposerAccepts[step] = make(map[uint]map[string]struct{})
		}
		if n.proposerAccepts[step][id] == nil {
			n.proposerAccepts[step][id] = make(map[string]struct{})
		}
		if src != "" {
			n.proposerAccepts[step][id][src] = struct{}{}
		}
		propCnt = len(n.proposerAccepts[step][id])
	}
	// Track global accept counts for learner/TLC triggering (ALL nodes should do this)
	if n.acceptCount == nil {
		n.acceptCount = make(map[uint]map[uint]map[string]struct{})
	}
	if n.acceptCount[step] == nil {
		n.acceptCount[step] = make(map[uint]map[string]struct{})
	}
	if n.acceptCount[step][id] == nil {
		n.acceptCount[step][id] = make(map[string]struct{})
	}
	if src != "" {
		n.acceptCount[step][id][src] = struct{}{}
	}
	tlcAlready = n.tlcBroadcasted[step]
	globalCnt = len(n.acceptCount[step][id])
	n.mu.Unlock()
	return propCnt, globalCnt, tlcAlready
}

// shouldProcessPromise checks if a Promise should be processed
func (n *node) shouldProcessPromise(step uint, promiseID uint) bool {
	return n.proposerPhase != nil && n.proposerPhase[step] == 1 &&
		n.proposerID != nil && n.proposerID[step] == promiseID
}

// logIgnoredPromise logs when a Promise is ignored
func (n *node) logIgnoredPromise(step uint, promiseID uint, src string) {
	if step < 4 || os.Getenv("GLOG") == "no" {
		return
	}
	phase := func() interface{} {
		if n.proposerPhase == nil {
			return nil
		}
		return n.proposerPhase[step]
	}()
	proposerID := func() interface{} {
		if n.proposerID == nil {
			return nil
		}
		return n.proposerID[step]
	}()
	log.Printf("[DEBUG] promise ignored node=%s step=%d id=%d src=%s phase=%v proposerID=%v",
		n.conf.Socket.GetAddress(), step, promiseID, src, phase, proposerID)
}

// broadcastProposeAndAdvancePhase broadcasts a Propose message and advances to phase 2
func (n *node) broadcastProposeAndAdvancePhase(step uint, id uint, value types.PaxosValue) {
	// Move to phase 2 and broadcast PROPOSE
	propose := types.PaxosProposeMessage{Step: step, ID: id, Value: value}
	if msg, err := n.conf.MessageRegistry.MarshalMessage(propose); err == nil {
		_ = n.Broadcast(msg)
	}
	n.mu.Lock()
	if n.proposerPhase == nil {
		n.proposerPhase = make(map[uint]int)
	}
	n.proposerPhase[step] = 2
	if n.proposerAccepts == nil {
		n.proposerAccepts = make(map[uint]map[uint]map[string]struct{})
	}
	if n.proposerAccepts[step] == nil {
		n.proposerAccepts[step] = make(map[uint]map[string]struct{})
	}
	if n.proposerAccepts[step][id] == nil {
		n.proposerAccepts[step][id] = make(map[string]struct{})
	}
	n.mu.Unlock()
}

// sendPromiseMessage sends a Promise message to the destination
func (n *node) sendPromiseMessage(dest string, wirePromise transport.Message, tmsg transport.Message) {
	if dest != "" && dest != n.conf.Socket.GetAddress() && n.conf.TotalPeers > 1 {
		_ = n.Unicast(dest, tmsg)
	}
	// If the destination is self, process Promise directly first to count it immediately,
	// then broadcast synchronously so the rumor ordering matches expectations.
	if dest == n.conf.Socket.GetAddress() {
		// Process Promise directly to ensure it's counted immediately
		// The source should be self (the acceptor that is sending the Promise)
		selfAddr := n.conf.Socket.GetAddress()
		localHeader := transport.NewHeader(selfAddr, selfAddr, selfAddr)
		_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: &localHeader, Msg: &wirePromise})
		// Broadcast immediately so the promise rumor is sequenced before later phases.
		_ = n.Broadcast(tmsg)
		return
	}
	// Optimistically deliver to proposer (without generating extra network traffic)
	if dest != "" {
		selfAddr := n.conf.Socket.GetAddress()
		header := transport.NewHeader(selfAddr, selfAddr, dest)
		_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: &header, Msg: &wirePromise})
	}
	_ = n.Broadcast(tmsg)
}

// commitBlock stores a block and updates naming store
func (n *node) commitBlock(block types.BlockchainBlock) {
	store := n.conf.Storage.GetBlockchainStore()
	if buf, err := block.Marshal(); err == nil {
		store.Set(hex.EncodeToString(block.Hash), buf)
		store.Set(storage.LastBlockKey, block.Hash)
	}
	// Naming store update
	nameStore := n.conf.Storage.GetNamingStore()
	if strings.TrimSpace(block.Value.Filename) != "" && strings.TrimSpace(block.Value.Metahash) != "" {
		nameStore.Set(block.Value.Filename, []byte(block.Value.Metahash))
	}
}

// completeStepWaiters closes all waiters for a step
func (n *node) completeStepWaiters(step uint) {
	n.stepWaitMu.Lock()
	if lst := n.stepWaiters[step]; len(lst) > 0 {
		if os.Getenv("GLOG") != "no" {
			log.Printf("[DEBUG] commit node=%s step=%d closing %d waiters",
				n.conf.Socket.GetAddress(), step, len(lst))
		}
		for _, ch := range lst {
			close(ch)
		}
		delete(n.stepWaiters, step)
	} else {
		if os.Getenv("GLOG") != "no" {
			log.Printf("[DEBUG] commit node=%s step=%d no waiters",
				n.conf.Socket.GetAddress(), step)
		}
	}
	n.stepWaitMu.Unlock()
}

// cleanupProposerState cleans up proposer state for a completed step
func (n *node) cleanupProposerState(step uint) {
	n.mu.Lock()
	if n.proposerPhase != nil {
		n.proposerPhase[step] = 3
	}
	if n.proposerPromises != nil {
		delete(n.proposerPromises, step)
	}
	if n.proposerAccepts != nil {
		delete(n.proposerAccepts, step)
	}
	if n.proposerID != nil {
		delete(n.proposerID, step)
	}
	if n.proposerHighestAcceptedID != nil {
		delete(n.proposerHighestAcceptedID, step)
	}
	if n.proposerValue != nil {
		delete(n.proposerValue, step)
	}
	n.mu.Unlock()
}

func (n *node) commitStepAndAdvance(step uint, block types.BlockchainBlock) {
	// Advance and try to catch up future steps with cached TLC quorum
	// We commit steps sequentially, starting with the given step
	for {
		// Verify we're committing the correct step
		n.mu.Lock()
		if step != n.currentStep {
			// If step is in the past, we've already moved beyond it
			// If step is in the future, we can't commit it yet
			n.mu.Unlock()
			break
		}
		n.mu.Unlock()

		// Store block and update name store for current step
		n.commitBlock(block)

		// Complete waiters for this step
		n.completeStepWaiters(step)

		// Mark proposer phase as completed so retry loops can stop
		n.cleanupProposerState(step)

		// Advance currentStep and check if we can commit the next step
		n.mu.Lock()
		n.currentStep++
		nextStep := n.currentStep
		cnt := len(n.tlcCount[nextStep])
		blk, ok := n.tlcBlock[nextStep]
		already := n.tlcBroadcasted[nextStep]
		n.mu.Unlock()

		if cnt >= n.getQuorum() && ok {
			if !already {
				n.broadcastTLCOnce(nextStep, blk)
			}
			// commit next in next iteration
			step = nextStep
			block = blk
			continue
		}
		break
	}
}
