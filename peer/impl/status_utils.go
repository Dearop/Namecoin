package impl

import (
	"time"

	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

func (n *node) computeStatusDeltas(remote types.StatusMessage) (
	haveForThem bool,
	needFromThem bool,
	local map[string]uint,
) {
	if remote == nil {
		remote = make(types.StatusMessage)
	}

	// snapshot local
	n.mu.RLock()
	local = make(map[string]uint, len(n.lastSeq))
	for k, v := range n.lastSeq {
		if k != "" {
			local[k] = v
		}
	}
	n.mu.RUnlock()
	// do comparisons
	for origin, lseq := range local {
		if origin == "" {
			continue
		}
		rseq, ok := remote[origin]
		if !ok {
			if lseq > 0 {
				haveForThem = true
			}
			continue
		}
		if lseq > rseq {
			haveForThem = true
		}
	}
	for origin, rseq := range remote {
		if origin == "" {
			continue
		}
		lseq := local[origin]
		if rseq > lseq {
			needFromThem = true
		}
	}
	return haveForThem, needFromThem, local
}

func (n *node) sendStatusToNeighbor() {
	if err := n.validateNode(false); err != nil {
		return
	}
	if n.conf.AntiEntropyInterval <= 0 {
		return
	}
	// throttle: at most one status per AntiEntropyInterval
	n.lastStatusMu.Lock()
	if !n.lastStatusAt.IsZero() && n.conf.AntiEntropyInterval > 0 {
		if time.Since(n.lastStatusAt) < n.conf.AntiEntropyInterval {
			n.lastStatusMu.Unlock()
			return
		}
	}
	n.lastStatusAt = time.Now()
	n.lastStatusMu.Unlock()

	nodeAddr := n.conf.Socket.GetAddress()
	// choose neighbor deterministically
	n.mu.RLock()
	neighbors := make([]string, 0, len(n.routingTable))
	for origin, relay := range n.routingTable {
		if origin == relay && origin != nodeAddr {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()
	if len(neighbors) == 0 {
		return
	}
	dest := neighbors[int(time.Now().UnixNano())%len(neighbors)]
	// per-destination rate limit: avoid blasting same neighbor repeatedly
	if n.conf.AntiEntropyInterval > 0 {
		n.statusRateMu.Lock()
		if n.lastStatusTo == nil {
			n.lastStatusTo = make(map[string]time.Time)
		}
		if ts, ok := n.lastStatusTo[dest]; ok {
			if time.Since(ts) < n.conf.AntiEntropyInterval {
				n.statusRateMu.Unlock()
				return
			}
		}
		n.lastStatusTo[dest] = time.Now()
		n.statusRateMu.Unlock()
	}
	status := n.buildStatus()
	wire, err := n.conf.MessageRegistry.MarshalMessage(status)
	if err != nil {
		return
	}
	header := transport.NewHeader(nodeAddr, nodeAddr, dest)
	_ = n.conf.Socket.Send(dest, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

func (n *node) buildStatus() types.StatusMessage {
	n.mu.RLock()
	defer n.mu.RUnlock()
	status := make(types.StatusMessage, len(n.lastSeq))
	for origin, seq := range n.lastSeq {
		if origin != "" && seq > 0 {
			status[origin] = seq
		}
	}
	return status
}

func (n *node) probabilisticallyMonger(exclude string) {
	if n == nil {
		return
	}

	if err := n.validateNode(false); err != nil {
		return
	}

	if n.conf.ContinueMongering <= 0 {
		return
	}
	if n.conf.ContinueMongering > 1.0 {
		return
	}

	// probabilistic trigger, but also rate-limit using AntiEntropyInterval
	if float64(time.Now().UnixNano()%1000)/1000.0 >= n.conf.ContinueMongering {
		return
	}
	if n.conf.AntiEntropyInterval > 0 {
		n.lastStatusMu.Lock()
		if !n.lastStatusAt.IsZero() && time.Since(n.lastStatusAt) < n.conf.AntiEntropyInterval {
			n.lastStatusMu.Unlock()
			return
		}
		n.lastStatusAt = time.Now()
		n.lastStatusMu.Unlock()
	}
	// pick neighbor not equal to exclude
	nodeAddr := n.conf.Socket.GetAddress()
	n.mu.RLock()
	neighbors := make([]string, 0, len(n.routingTable))
	for origin, relay := range n.routingTable {
		if origin != "" && relay != "" && origin == relay && origin != nodeAddr && origin != exclude {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()
	if len(neighbors) == 0 {
		return
	}
	dest := neighbors[int(time.Now().UnixNano())%len(neighbors)]
	status := n.buildStatus()
	wire, err := n.conf.MessageRegistry.MarshalMessage(status)
	if err != nil {
		return
	}
	header := transport.NewHeader(nodeAddr, nodeAddr, dest)
	_ = n.conf.Socket.Send(dest, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}
