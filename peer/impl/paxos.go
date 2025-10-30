package impl

import (
	"crypto/sha256"
	"encoding/hex"
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
	// Only process messages for current step (read under lock)
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
	if prep.ID <= n.promisedID[step] {
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

	// Reply with a PROMISE via private broadcast to proposer source
	promise := types.PaxosPromiseMessage{Step: step, ID: prep.ID, AcceptedID: accID, AcceptedValue: accVal}
	wirePromise, err := n.conf.MessageRegistry.MarshalMessage(promise)
	if err != nil {
		return nil
	}
	recipients := map[string]struct{}{}
	dest := strings.TrimSpace(prep.Source)
	if dest != "" {
		recipients[dest] = struct{}{}
	}
	priv := types.PrivateMessage{Recipients: recipients, Msg: &wirePromise}
	if tmsg, err := n.conf.MessageRegistry.MarshalMessage(priv); err == nil {
		// Avoid making the private rumor take sequence 1 when prepare originates locally.
		// If the destination is self, delay slightly so the outer prepare Broadcast can complete.
		if dest == n.conf.Socket.GetAddress() {
			go func(m transport.Message) {
				time.Sleep(time.Millisecond)
				_ = n.Broadcast(m)
			}(tmsg)
		} else {
			_ = n.Broadcast(tmsg)
		}
	}
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
	if step != n.currentStep {
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
	if step != n.currentStep {
		return nil
	}
	src := ""
	if pkt.Header != nil {
		src = strings.TrimSpace(pkt.Header.Source)
	}
	n.mu.Lock()
	// Only process during phase 1 and for current proposal ID
	if n.proposerPhase == nil || n.proposerPhase[step] != 1 || n.proposerID == nil || n.proposerID[step] != prm.ID {
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
	// Snapshot selected value (adopted or initially set)
	value := n.proposerValue[step]
	n.mu.Unlock()

	if cnt >= n.getQuorum() {
		// Move to phase 2 and broadcast PROPOSE
		propose := types.PaxosProposeMessage{Step: step, ID: prm.ID, Value: value}
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
		if n.proposerAccepts[step][prm.ID] == nil {
			n.proposerAccepts[step][prm.ID] = make(map[string]struct{})
		}
		n.mu.Unlock()
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
	if step != n.currentStep {
		return nil
	}
	n.mu.Lock()
	// Track proposer accept quorum for proposer completion bookkeeping
	if n.proposerPhase == nil || n.proposerPhase[step] != 2 || n.proposerID == nil || n.proposerID[step] != acc.ID {
		n.mu.Unlock()
		return nil
	}
	if n.proposerAccepts == nil {
		n.proposerAccepts = make(map[uint]map[uint]map[string]struct{})
	}
	if n.proposerAccepts[step] == nil {
		n.proposerAccepts[step] = make(map[uint]map[string]struct{})
	}
	if n.proposerAccepts[step][acc.ID] == nil {
		n.proposerAccepts[step][acc.ID] = make(map[string]struct{})
	}
	src := ""
	if pkt.Header != nil {
		src = strings.TrimSpace(pkt.Header.Source)
	}
	if src != "" {
		n.proposerAccepts[step][acc.ID][src] = struct{}{}
	}
	propCnt := len(n.proposerAccepts[step][acc.ID])
	// Also track global accept counts for learner/TLC triggering
	if n.acceptCount == nil {
		n.acceptCount = make(map[uint]map[uint]map[string]struct{})
	}
	if n.acceptCount[step] == nil {
		n.acceptCount[step] = make(map[uint]map[string]struct{})
	}
	if n.acceptCount[step][acc.ID] == nil {
		n.acceptCount[step][acc.ID] = make(map[string]struct{})
	}
	if src != "" {
		n.acceptCount[step][acc.ID][src] = struct{}{}
	}
	tlcAlready := n.tlcBroadcasted[step]
	globalCnt := len(n.acceptCount[step][acc.ID])
	n.mu.Unlock()

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
	if globalCnt >= n.getQuorum() && !tlcAlready {
		// Small delay so ACCEPT gets recorded before TLC
		time.Sleep(time.Millisecond)
		block := n.buildBlock(step, acc.Value)
		n.broadcastTLCOnce(step, block)
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
	src := ""
	if pkt.Header != nil {
		src = strings.TrimSpace(pkt.Header.Source)
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
	if src != "" {
		n.tlcCount[step][src] = struct{}{}
	}
	// store latest block for this step (they should be the same)
	n.tlcBlock[step] = tlc.Block
	cnt := len(n.tlcCount[step])
	already := n.tlcBroadcasted[step]
	n.mu.Unlock()

	if cnt >= n.getQuorum() {
		// If quorum on current step, commit and maybe broadcast TLC once
		if step == n.currentStep {
			if !already {
				n.broadcastTLCOnce(step, tlc.Block)
			}
			n.commitStepAndAdvance(step, tlc.Block)
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

	tlc := types.TLCMessage{Step: step, Block: block}
	if msg, err := n.conf.MessageRegistry.MarshalMessage(tlc); err == nil {
		// Broadcast to neighbors
		_ = n.Broadcast(msg)
		// Count self's TLC
		n.mu.Lock()
		if n.tlcCount == nil {
			n.tlcCount = make(map[uint]map[string]struct{})
		}
		if n.tlcCount[step] == nil {
			n.tlcCount[step] = make(map[string]struct{})
		}
		n.tlcCount[step][n.conf.Socket.GetAddress()] = struct{}{}
		n.mu.Unlock()
	}
}

func (n *node) commitStepAndAdvance(step uint, block types.BlockchainBlock) {
	// Store block and update name store
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

	// Complete waiters for this step
	n.stepWaitMu.Lock()
	if lst := n.stepWaiters[step]; len(lst) > 0 {
		for _, ch := range lst {
			close(ch)
		}
		delete(n.stepWaiters, step)
	}
	n.stepWaitMu.Unlock()

	// Advance and try to catch up future steps with cached TLC quorum
	for {
		n.mu.Lock()
		if step != n.currentStep {
			// ensure we're only advancing from currentStep
			n.mu.Unlock()
		} else {
			n.currentStep++
			n.mu.Unlock()
		}
		next := step + 1
		n.mu.Lock()
		cnt := len(n.tlcCount[next])
		blk, ok := n.tlcBlock[next]
		already := n.tlcBroadcasted[next]
		n.mu.Unlock()
		if cnt >= n.getQuorum() && ok {
			if !already {
				n.broadcastTLCOnce(next, blk)
			}
			// commit next and continue loop
			step = next
			block = blk
			// loop iteration will commit storage
			store := n.conf.Storage.GetBlockchainStore()
			if buf, err := block.Marshal(); err == nil {
				store.Set(hex.EncodeToString(block.Hash), buf)
				store.Set(storage.LastBlockKey, block.Hash)
			}
			nameStore := n.conf.Storage.GetNamingStore()
			if strings.TrimSpace(block.Value.Filename) != "" && strings.TrimSpace(block.Value.Metahash) != "" {
				nameStore.Set(block.Value.Filename, []byte(block.Value.Metahash))
			}
			n.stepWaitMu.Lock()
			if lst := n.stepWaiters[step]; len(lst) > 0 {
				for _, ch := range lst {
					close(ch)
				}
				delete(n.stepWaiters, step)
			}
			n.stepWaitMu.Unlock()
			continue
		}
		break
	}
}
