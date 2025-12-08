package integration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func TestNamecoinExpiryResolver_PruneMany(t *testing.T) {
	st := impl.NewState()
	expiryHeight := uint64(5)

	// Seed a batch of domains all expiring at the same height.
	const count = 50
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("batch-%d.bit", i)
		st.SetDomain(types.NameRecord{
			Owner:     "ownerX",
			IP:        "203.0.113.1",
			Domain:    name,
			ExpiresAt: expiryHeight,
		})
	}

	resolver := impl.NewNamecoinDNS()
	resolver.BindReader(st)

	// Before expiry, all domains should resolve successfully.
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("batch-%d.bit", i)
		resp := resolver.Resolve(name)
		require.Equal(t, types.DNSStatusSuccess, resp.Status, "expected success for %s before expiry", name)
	}

	// Advance chain height to expiry to prune.
	require.NoError(t, st.ApplyBlock(&types.Block{
		Header:       types.BlockHeader{Height: expiryHeight},
		Transactions: nil,
	}))

	// After expiry, resolver should return NXDOMAIN for all.
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("batch-%d.bit", i)
		resp := resolver.Resolve(name)
		require.Equal(t, types.DNSStatusNXDomain, resp.Status, "expected NXDOMAIN for %s after expiry", name)
	}
}
