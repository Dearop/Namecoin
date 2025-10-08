package impl

import (
	"strings"

	"go.dedis.ch/cs438/peer"
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
	store := n.conf.Storage.GetNamingStore()
	store.Set(name, []byte(mh))
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
