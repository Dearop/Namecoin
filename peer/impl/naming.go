package impl

import (
    "log"
    "math/rand"
    "strings"
    "time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

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

	// Proposer: start at current step with configured PaxosID
	n.mu.RLock()
	step := n.currentStep
	n.mu.RUnlock()
	propID := n.conf.PaxosID
	self := n.conf.Socket.GetAddress()

	// Record value to propose
	n.mu.Lock()
	if n.proposerValue == nil {
		n.proposerValue = make(map[uint]types.PaxosValue)
	}
	n.proposerValue[step] = types.PaxosValue{Filename: name, Metahash: mh}
	n.mu.Unlock()

	// initialize proposer state
	n.mu.Lock()
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
	if n.proposerValue == nil {
		n.proposerValue = make(map[uint]types.PaxosValue)
	}
	n.proposerPhase[step] = 1
	n.proposerID[step] = propID
	n.proposerPromises[step] = make(map[uint]map[string]struct{})
	n.proposerAccepts[step] = make(map[uint]map[string]struct{})
	n.mu.Unlock()

	// define a function to send one prepare round and self-promise
	sendRound := func() {
		n.mu.Lock()
		id := n.proposerID[step]
		n.mu.Unlock()
		prepare := types.PaxosPrepareMessage{Step: step, ID: id, Source: self}
		if msg, err := n.conf.MessageRegistry.MarshalMessage(prepare); err == nil {
			_ = n.Broadcast(msg)
		}
	}

    // Initial round with staggering to reduce contention
    if n.conf.TotalPeers > 1 {
        total := int(n.conf.TotalPeers)
        leader := int(step%uint(total)) + 1
        offset := (int(n.conf.PaxosID) - leader + total) % total
        baseDelay := 40 * time.Millisecond
        jitter := time.Duration(rand.Intn(20)) * time.Millisecond
        time.Sleep(time.Duration(offset)*baseDelay + jitter)
    }
    // Initial round
    sendRound()

	// Periodic retries until consensus/TLC (handled later)
	retry := n.conf.PaxosProposerRetry
	if retry <= 0 {
		retry = time.Second * 5
	}
	go func() {
		ticker := time.NewTicker(retry)
		defer ticker.Stop()
		for {
			select {
			case <-n.stopCh:
				return
			case <-ticker.C:
				n.mu.Lock()
				phase := n.proposerPhase[step]
				id := n.proposerID[step]
				promises := len(n.proposerPromises[step][id])
				accepts := len(n.proposerAccepts[step][id])
				n.mu.Unlock()
				q := n.getQuorum()
				if phase == 3 { // done
					return
				}
			if phase == 1 && promises < q {
				// bump ID and retry phase 1
				n.mu.Lock()
				base := id
				if n.promisedID != nil && n.promisedID[step] > base {
					base = n.promisedID[step]
				}
				candidate := base + n.conf.TotalPeers
				ts := uint(time.Now().UnixNano())
				if ts > candidate {
					candidate = ts
				}
				log.Printf("[DEBUG] proposer %s step=%d bump phase=1 from=%d base=%d new=%d promises=%d",
					n.conf.Socket.GetAddress(), step, id, base, candidate, promises)
				n.proposerID[step] = candidate
				n.proposerPromises[step] = make(map[uint]map[string]struct{})
				n.mu.Unlock()
				sendRound()
				continue
			}
			if phase == 2 && accepts < q {
				// back to phase 1 with higher ID
				n.mu.Lock()
				n.proposerPhase[step] = 1
				base := id
				if n.promisedID != nil && n.promisedID[step] > base {
					base = n.promisedID[step]
				}
				candidate := base + n.conf.TotalPeers
				ts := uint(time.Now().UnixNano())
				if ts > candidate {
					candidate = ts
				}
				log.Printf("[DEBUG] proposer %s step=%d bump phase=2 from=%d base=%d new=%d accepts=%d",
					n.conf.Socket.GetAddress(), step, id, base, candidate, accepts)
				n.proposerID[step] = candidate
				n.proposerPromises[step] = make(map[uint]map[string]struct{})
				n.proposerAccepts[step] = make(map[uint]map[string]struct{})
				n.mu.Unlock()
				sendRound()
			}
			}
		}
	}()

	// Wait until TLC commit completes this step
	n.stepWaitMu.Lock()
	if n.stepWaiters == nil {
		n.stepWaiters = make(map[uint][]chan struct{})
	}
	ch := make(chan struct{})
	n.stepWaiters[step] = append(n.stepWaiters[step], ch)
	n.stepWaitMu.Unlock()

	log.Printf("[DEBUG] Tag start node=%s step=%d", self, step)
	<-ch
	log.Printf("[DEBUG] Tag done node=%s step=%d", self, step)
	return nil
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
