package impl

import (
	"time"
)

// antiEntropyLoop runs the anti-entropy process periodically.
// It sends status messages to neighbors at the specified interval until the node stops.
func (n *node) antiEntropyLoop(interval time.Duration) {
	defer n.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.sendStatusToNeighbor()
		}
	}
}
