package impl

import (
	"sync"
)

// lookupNextHop returns the relay address for a given destination if known.
func (n *node) lookupNextHop(dest string) (string, bool) {
	n.mu.RLock()
	relay, ok := n.routingTable[dest]
	n.mu.RUnlock()
	if !ok || relay == "" {
		return "", false
	}
	return relay, true
}

var _ sync.Locker
