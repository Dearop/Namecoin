package impl

import (
	"strings"

	"go.dedis.ch/cs438/peer"
	"golang.org/x/xerrors"
)

// Tag implements peer.DataSharing.
// Simplified: store name->metahash locally, rejecting duplicates.
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
	if existing := store.Get(name); len(existing) > 0 && string(existing) != mh {
		return xerrors.Errorf("name already taken")
	}
	store.Set(name, []byte(mh))
	return nil
}

// Resolve implements peer.DataSharing. It looks up a name in the local naming
// store and returns the associated metahash, or empty string if not found.
func (n *node) Resolve(name string) (metahash string) {
	if err := n.validateNode(false); err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	val := n.conf.Storage.GetNamingStore().Get(name)
	if len(val) == 0 {
		return ""
	}
	return string(val)
}

var _ peer.DataSharing
