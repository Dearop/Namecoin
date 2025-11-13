package impl

import (
	"bytes"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// nextProposalID calculates the next proposal ID for a given slot
func nextProposalID(current, minimum uint, totalPeers, slot int) uint {
	target := current + uint(totalPeers)
	if target < minimum {
		target = minimum
	}
	rem := target % uint(totalPeers)
	if rem == 0 {
		rem = uint(totalPeers)
	}
	if rem != uint(slot) {
		if rem < uint(slot) {
			target += uint(slot) - rem
		} else {
			target += uint(totalPeers) - rem + uint(slot)
		}
	}
	if target <= current {
		target += uint(totalPeers)
	}
	return target
}

// initializeProposerState initializes proposer state for a step
func (n *node) initializeProposerState(step uint, value types.PaxosValue) {
	n.mu.Lock()
	if n.proposerValue == nil {
		n.proposerValue = make(map[uint]types.PaxosValue)
	}
	n.proposerValue[step] = value
	if n.proposerPhase == nil {
		n.proposerPhase = make(map[uint]int)
	}
	if n.proposerID == nil {
		n.proposerID = make(map[uint]uint)
	}
	if n.proposerPromises == nil {
		n.proposerPromises = make(map[uint]map[uint]map[string]struct{})
	}
	if n.proposerAccepts == nil {
		n.proposerAccepts = make(map[uint]map[uint]map[string]struct{})
	}
	if n.proposerHighestAcceptedID == nil {
		n.proposerHighestAcceptedID = make(map[uint]uint)
	}
	n.proposerPhase[step] = 1
	n.proposerID[step] = n.conf.PaxosID
	n.proposerPromises[step] = make(map[uint]map[string]struct{})
	n.proposerAccepts[step] = make(map[uint]map[string]struct{})
	n.proposerHighestAcceptedID[step] = 0
	n.mu.Unlock()
}

// sendPaxosPrepare sends a Paxos Prepare message
func (n *node) sendPaxosPrepare(step uint, self string) {
	n.mu.Lock()
	id := n.proposerID[step]
	n.mu.Unlock()
	prepare := types.PaxosPrepareMessage{Step: step, ID: id, Source: self}
	if msg, err := n.conf.MessageRegistry.MarshalMessage(prepare); err == nil {
		_ = n.Broadcast(msg)
	}
}

// delayForLeader calculates and applies delay based on leader election
func (n *node) delayForLeader(step uint) {
	if n.conf.TotalPeers <= 1 {
		return
	}
	total := int(n.conf.TotalPeers)
	leader := int(step%uint(total)) + 1
	offset := (int(n.conf.PaxosID) - leader + total) % total
	baseDelay := 40 * time.Millisecond
	jitter := time.Duration(rand.Intn(20)) * time.Millisecond
	time.Sleep(time.Duration(offset)*baseDelay + jitter)
}

// runProposerRetryLoop runs the retry loop for a proposer and returns a done channel
func (n *node) runProposerRetryLoop(step uint, slot int, totalPeers int, sendRound func()) chan struct{} {
	retry := n.conf.PaxosProposerRetry
	if retry <= 0 {
		retry = time.Second * 5
	}

	done := make(chan struct{})
	go func(localStep uint) {
		ticker := time.NewTicker(retry)
		defer ticker.Stop()
		for {
			select {
			case <-n.stopCh:
				return
			case <-done:
				return
			case <-ticker.C:
				if n.handleProposerRetryTick(localStep, slot, totalPeers, sendRound) {
					return
				}
			}
		}
	}(step)
	return done
}

// handleProposerRetryTick handles a single retry tick
func (n *node) handleProposerRetryTick(localStep uint, slot int, totalPeers int, sendRound func()) bool {
	n.mu.Lock()
	phase := n.proposerPhase[localStep]
	id := n.proposerID[localStep]
	promises := len(n.proposerPromises[localStep][id])
	accepts := len(n.proposerAccepts[localStep][id])
	currentPromised := uint(0)
	if n.promisedID != nil {
		currentPromised = n.promisedID[localStep]
	}
	n.mu.Unlock()
	leaderID := uint(0)
	if currentPromised > 0 && n.conf.TotalPeers > 0 {
		rem := currentPromised % uint(totalPeers)
		if rem == 0 {
			rem = uint(totalPeers)
		}
		leaderID = rem
	}
	q := n.getQuorum()
	if phase == 3 { // done
		return true
	}
	if phase < 3 && leaderID != 0 && leaderID != uint(slot) {
		return false
	}
	if phase == 1 && promises < q {
		n.bumpProposerPhase1(localStep, id, totalPeers, slot, promises)
		sendRound()
		return false
	}
	if phase == 2 && accepts < q {
		n.bumpProposerPhase2(localStep, id, totalPeers, slot, accepts)
		sendRound()
		return false
	}
	return false
}

// bumpProposerPhase1 bumps proposer to a new ID in phase 1
func (n *node) bumpProposerPhase1(localStep uint, id uint, totalPeers int, slot int, promises int) {
	n.mu.Lock()
	base := id
	if n.promisedID != nil && n.promisedID[localStep] > base {
		base = n.promisedID[localStep]
	}
	nextID := nextProposalID(id, base, totalPeers, slot)
	if os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] proposer %s step=%d bump phase=1 from=%d base=%d new=%d promises=%d",
			n.conf.Socket.GetAddress(), localStep, id, base, nextID, promises)
	}
	n.proposerID[localStep] = nextID
	n.proposerPromises[localStep] = make(map[uint]map[string]struct{})
	n.mu.Unlock()
}

// bumpProposerPhase2 bumps proposer back to phase 1 with new ID
func (n *node) bumpProposerPhase2(localStep uint, id uint, totalPeers int, slot int, accepts int) {
	n.mu.Lock()
	base := id
	if n.promisedID != nil && n.promisedID[localStep] > base {
		base = n.promisedID[localStep]
	}
	nextID := nextProposalID(id, base, totalPeers, slot)
	if os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] proposer %s step=%d bump phase=2 from=%d base=%d new=%d accepts=%d",
			n.conf.Socket.GetAddress(), localStep, id, base, nextID, accepts)
	}
	n.proposerPhase[localStep] = 1
	n.proposerID[localStep] = nextID
	n.proposerPromises[localStep] = make(map[uint]map[string]struct{})
	n.proposerAccepts[localStep] = make(map[uint]map[string]struct{})
	n.mu.Unlock()
}

// waitForStepCompletion waits for a step to complete
func (n *node) waitForStepCompletion(step uint, self string) {
	ch := n.registerStepWaiter(step)
	if ch == nil {
		return
	}

	if os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] Tag start node=%s step=%d", self, step)
	}
	<-ch
	if os.Getenv("GLOG") != "no" {
		log.Printf("[DEBUG] Tag done node=%s step=%d", self, step)
	}
}

// registerStepWaiter registers a waiter for a step and returns the channel
func (n *node) registerStepWaiter(step uint) chan struct{} {
	n.stepWaitMu.Lock()
	if n.stepWaiters == nil {
		n.stepWaiters = make(map[uint][]chan struct{})
	}
	n.mu.RLock()
	if n.currentStep > step {
		n.mu.RUnlock()
		n.stepWaitMu.Unlock()
		return nil
	}
	n.mu.RUnlock()
	ch := make(chan struct{})
	n.stepWaiters[step] = append(n.stepWaiters[step], ch)
	n.stepWaitMu.Unlock()
	return ch
}

// Tag implements peer.DataSharing
func (n *node) Tag(name string, mh string) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	mh = strings.TrimSpace(mh)
	if name == "" || mh == "" {
		return xerrors.Errorf("invalid name or metahash")
	}
	// If consensus disabled or single peer, tag directly without network
	if n.conf.TotalPeers <= 1 {
		store := n.conf.Storage.GetNamingStore()
		store.Set(name, []byte(mh))
		return nil
	}
	// If name already exists locally, reject (uniqueness)
	if v := n.conf.Storage.GetNamingStore().Get(name); len(v) > 0 {
		return xerrors.Errorf("name already taken")
	}

	value := types.PaxosValue{Filename: name, Metahash: mh}
	self := n.conf.Socket.GetAddress()

	totalPeers := int(n.conf.TotalPeers)
	if totalPeers == 0 {
		totalPeers = 1
	}
	slot := int(n.conf.PaxosID) % totalPeers
	if slot == 0 {
		slot = totalPeers
	}

	for {
		store := n.conf.Storage.GetNamingStore()
		if current := store.Get(name); len(current) > 0 {
			if bytes.Equal(current, []byte(mh)) {
				return nil
			}
			return xerrors.Errorf("name already taken")
		}

		n.mu.RLock()
		step := n.currentStep
		n.mu.RUnlock()

		n.initializeProposerState(step, value)
		sendRound := func() {
			n.sendPaxosPrepare(step, self)
		}

		n.delayForLeader(step)
		sendRound()
		done := n.runProposerRetryLoop(step, slot, totalPeers, sendRound)
		n.waitForStepCompletion(step, self)
		close(done)

		if current := store.Get(name); len(current) > 0 {
			if bytes.Equal(current, []byte(mh)) {
				return nil
			}
			return xerrors.Errorf("name already taken")
		}
		// Name not yet chosen, advance to next step and retry
	}
}

// Resolve implements peer.DataSharing
func (n *node) Resolve(name string) (metahash string) {
	if err := n.validateNode(false); err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	store := n.conf.Storage.GetNamingStore()
	val := store.Get(name)
	if len(val) == 0 {
		return ""
	}
	return string(val)
}

var _ peer.DataSharing
