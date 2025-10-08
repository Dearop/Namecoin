package impl

import (
	"time"
)

// antiEntropyLoop runs the anti-entropy process periodically
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
