package impl

import (
	"strings"

	"go.dedis.ch/cs438/peer"
)

// GetCatalog implements peer.DataSharing
func (n *node) GetCatalog() peer.Catalog {
	if err := n.validateNode(false); err != nil {
		return peer.Catalog{}
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make(peer.Catalog, len(n.catalog))
	for k, peers := range n.catalog {
		copyBag := make(map[string]struct{}, len(peers))
		for p := range peers {
			copyBag[p] = struct{}{}
		}
		out[k] = copyBag
	}
	return out
}

// UpdateCatalog implements peer.DataSharing
func (n *node) UpdateCatalog(key string, peerAddr string) {
	if err := n.validateNode(false); err != nil {
		return
	}
	key = strings.TrimSpace(key)
	peerAddr = strings.TrimSpace(peerAddr)
	if key == "" || peerAddr == "" {
		return
	}
	self := n.conf.Socket.GetAddress()
	if peerAddr == strings.TrimSpace(self) {
		// do not record ourselves in the catalog
		return
	}
	n.mu.Lock()
	if n.catalog == nil {
		n.catalog = make(map[string]map[string]struct{})
	}
	if n.catalog[key] == nil {
		n.catalog[key] = make(map[string]struct{})
	}
	n.catalog[key][peerAddr] = struct{}{}
	n.mu.Unlock()
}
