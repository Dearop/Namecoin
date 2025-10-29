package impl

import (
	"time"

	"go.dedis.ch/cs438/types"
)

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
