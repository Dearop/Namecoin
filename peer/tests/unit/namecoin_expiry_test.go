package unit

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func buildTx(t *testing.T, typ, from string, payload any) types.Tx {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return types.Tx{
		From:    from,
		Type:    typ,
		Payload: raw,
	}
}

func TestNamecoinExpiry_PruneAndReregister(t *testing.T) {
	st := impl.NewState()
	const (
		owner  = "owner1"
		domain = "foo.bit"
		salt   = "pepper"
	)
	commit := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))

	txNew := buildTx(t, impl.NameNewCommandName, owner, impl.NameNew{Commitment: commit, TTL: impl.DefaultDomainTTLBlocks})
	txFirst := buildTx(t, impl.NameFirstUpdateCommandName, owner, impl.NameFirstUpdate{
		Domain: domain,
		Salt:   salt,
		IP:     "1.2.3.4",
	})

	st.EnsureAccount(owner)
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: 1},
		Transactions: []types.Tx{txNew, txFirst},
	}))

	rec, ok := st.SnapshotDomainsMap()[domain]
	require.True(t, ok)
	require.Equal(t, owner, rec.Owner)
	require.Equal(t, uint64(1+impl.DefaultDomainTTLBlocks), rec.ExpiresAt)

	expireHeight := impl.DefaultDomainTTLBlocks + 2
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: expireHeight},
		Transactions: nil,
	}))

	_, ok = st.SnapshotDomainsMap()[domain]
	require.False(t, ok, "domain should be pruned after expiry")

	// Re-register after expiry should succeed with a new commitment.
	salt2 := "pepper2"
	commit2 := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt2))
	txNew2 := buildTx(t, impl.NameNewCommandName, owner, impl.NameNew{Commitment: commit2, TTL: impl.DefaultDomainTTLBlocks})
	txFirst2 := buildTx(t, impl.NameFirstUpdateCommandName, owner, impl.NameFirstUpdate{
		Domain: domain,
		Salt:   salt2,
		IP:     "2.2.2.2",
	})
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: expireHeight + 1},
		Transactions: []types.Tx{txNew2, txFirst2},
	}))

	rec, ok = st.SnapshotDomainsMap()[domain]
	require.True(t, ok, "domain should be re-registered")
	require.Equal(t, expireHeight+1+impl.DefaultDomainTTLBlocks, rec.ExpiresAt)
	require.Equal(t, "2.2.2.2", rec.IP)
}

func TestNamecoinExpiry_UpdateRefreshesTTL(t *testing.T) {
	st := impl.NewState()
	const (
		owner  = "owner2"
		domain = "bar.bit"
		salt   = "salt2"
	)
	commit := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))

	txNew := buildTx(t, impl.NameNewCommandName, owner, impl.NameNew{Commitment: commit, TTL: impl.DefaultDomainTTLBlocks})
	txFirst := buildTx(t, impl.NameFirstUpdateCommandName, owner, impl.NameFirstUpdate{
		Domain: domain,
		Salt:   salt,
		IP:     "5.6.7.8",
	})

	st.EnsureAccount(owner)
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: 2},
		Transactions: []types.Tx{txNew, txFirst},
	}))

	updateHeight := uint64(5)
	txUpdate := buildTx(t, impl.NameUpdateCommandName, owner, impl.NameUpdate{
		Domain: domain,
		IP:     "9.9.9.9",
	})

	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: updateHeight},
		Transactions: []types.Tx{txUpdate},
	}))

	rec, ok := st.SnapshotDomainsMap()[domain]
	require.True(t, ok)
	require.Equal(t, "9.9.9.9", rec.IP)
	require.Equal(t, updateHeight+impl.DefaultDomainTTLBlocks, rec.ExpiresAt, "TTL should refresh on update")
}

func TestNamecoinExpiry_StaggeredPrunePartial(t *testing.T) {
	st := impl.NewState()
	st.EnsureAccount("ownerA")
	st.EnsureAccount("ownerB")

	// Register two domains at different heights so they expire on different blocks.
	registerDomain(t, st, 1, "ownerA", "alpha.bit", "saltA", "10.0.0.1")
	registerDomain(t, st, 2, "ownerB", "beta.bit", "saltB", "10.0.0.2")

	// Prune at the first expiry height: alpha should drop, beta should remain.
	pruneHeight := impl.DefaultDomainTTLBlocks + 1
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: pruneHeight},
		Transactions: nil,
	}))

	_, okA := st.SnapshotDomainsMap()["alpha.bit"]
	require.False(t, okA, "alpha should be pruned at its expiry")
	_, okB := st.SnapshotDomainsMap()["beta.bit"]
	require.True(t, okB, "beta should remain until its later expiry")
}

func TestNamecoinExpiry_StressManyDomains(t *testing.T) {
	st := impl.NewState()
	st.EnsureAccount("ownerX")

	const count = 100
	for i := 0; i < count; i++ {
		domain := fmt.Sprintf("stress-%d.bit", i)
		salt := fmt.Sprintf("salt-%d", i)
		registerDomain(t, st, 1, "ownerX", domain, salt, "192.0.2.1")
	}

	// Jump to expiry height and prune; all should be gone.
	expireHeight := impl.DefaultDomainTTLBlocks + 5
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: expireHeight},
		Transactions: nil,
	}))

	for i := 0; i < count; i++ {
		domain := fmt.Sprintf("stress-%d.bit", i)
		if _, ok := st.SnapshotDomainsMap()[domain]; ok {
			t.Fatalf("expected domain %s to be pruned", domain)
		}
	}
}

// registerDomain registers a domain with NameNew + NameFirstUpdate in a single block at given height.
func registerDomain(t *testing.T, st *impl.NamecoinState, height uint64, owner, domain, salt, ip string) {
	t.Helper()
	commit := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))
	txNew := buildTx(t, impl.NameNewCommandName, owner, impl.NameNew{Commitment: commit, TTL: impl.DefaultDomainTTLBlocks})
	txFirst := buildTx(t, impl.NameFirstUpdateCommandName, owner, impl.NameFirstUpdate{
		Domain: domain,
		Salt:   salt,
		IP:     ip,
	})
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: height},
		Transactions: []types.Tx{txNew, txFirst},
	}))
}
