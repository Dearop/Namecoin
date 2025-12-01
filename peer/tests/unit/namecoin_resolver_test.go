package unit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

type stubChainReader struct {
	domains map[string]types.NameRecord
	height  uint64
}

func (s stubChainReader) SnapshotDomains() (map[string]types.NameRecord, uint64) {
	out := make(map[string]types.NameRecord, len(s.domains))
	for k, v := range s.domains {
		out[k] = v
	}
	return out, s.height
}

func TestNamecoinDNS_NXDomainMissing(t *testing.T) {
	dns := impl.NewNamecoinDNS()
	dns.BindReader(stubChainReader{domains: map[string]types.NameRecord{}, height: 5})

	resp := dns.Resolve("missing.bit")
	require.Equal(t, types.DNSStatusNXDomain, resp.Status)
	require.Zero(t, resp.TTL)
}

func TestNamecoinDNS_NXDomainExpired(t *testing.T) {
	dns := impl.NewNamecoinDNS()
	dns.BindReader(stubChainReader{
		domains: map[string]types.NameRecord{
			"expired.bit": {Value: "1.2.3.4", ExpiresAt: 3},
		},
		height: 5,
	})

	resp := dns.Resolve("expired.bit")
	require.Equal(t, types.DNSStatusNXDomain, resp.Status)
}

func TestNamecoinDNS_IPSuccess(t *testing.T) {
	dns := impl.NewNamecoinDNS()
	dns.BindReader(stubChainReader{
		domains: map[string]types.NameRecord{
			"live.bit": {Value: "1.2.3.4", ExpiresAt: 20},
		},
		height: 10,
	})

	resp := dns.Resolve("live.bit")
	require.Equal(t, types.DNSStatusSuccess, resp.Status)
	require.Equal(t, "1.2.3.4", resp.IP)
	require.Equal(t, uint64(10), resp.TTL)
	require.Empty(t, resp.TXTRecord)
}

func TestNamecoinDNS_TXTSuccess(t *testing.T) {
	dns := impl.NewNamecoinDNS()
	dns.BindReader(stubChainReader{
		domains: map[string]types.NameRecord{
			"text.bit": {Value: "hello world", ExpiresAt: 15},
		},
		height: 10,
	})

	resp := dns.Resolve("text.bit")
	require.Equal(t, types.DNSStatusSuccess, resp.Status)
	require.Equal(t, "hello world", resp.TXTRecord)
	require.Equal(t, uint64(5), resp.TTL)
	require.Empty(t, resp.IP)
}

func TestNamecoinDNS_InvalidEmptyValue(t *testing.T) {
	dns := impl.NewNamecoinDNS()
	dns.BindReader(stubChainReader{
		domains: map[string]types.NameRecord{
			"empty.bit": {Value: "   ", ExpiresAt: 42},
		},
		height: 1,
	})

	resp := dns.Resolve("empty.bit")
	require.Equal(t, types.DNSStatusInvalid, resp.Status)
	require.Zero(t, resp.TTL)
}
