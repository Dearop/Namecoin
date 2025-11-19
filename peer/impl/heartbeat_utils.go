package impl

import (
	"time"

	"go.dedis.ch/cs438/types"
)

// sendHeartbeat sends an empty heartbeat message to all neighbors.
// It broadcasts an empty message to maintain network connectivity.
func (n *node) sendHeartbeat() {
	if n == nil {
		return
	}
	if err := n.validateNode(false); err != nil {
		return
	}
	empty := types.EmptyMessage{}
	data, err := n.conf.MessageRegistry.MarshalMessage(empty)
	if err != nil {
		return
	}
	_ = n.Broadcast(data)
}

// heartbeatLoop runs a periodic heartbeat sender at the specified interval.
// It sends heartbeats until the node is stopped.
func (n *node) heartbeatLoop(interval time.Duration) {
	if n == nil {
		return
	}
	defer n.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.sendHeartbeat()
		}
	}
}
