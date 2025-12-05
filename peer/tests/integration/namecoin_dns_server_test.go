package integration

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/dns"
	"go.dedis.ch/cs438/types"
)

type staticResolver map[string]types.DNSResponse

func (s staticResolver) Resolve(domain string) types.DNSResponse {
	if resp, ok := s[domain]; ok {
		return resp
	}
	return types.DNSResponse{Domain: domain, Status: types.DNSStatusNXDomain}
}

func TestNamecoinDNSServer_ARecordSuccess(t *testing.T) {
	skipIfWIndows(t)

	resolver := staticResolver{
		"live.bit": {
			Domain: "live.bit",
			IP:     "1.2.3.4",
			Status: types.DNSStatusSuccess,
		},
	}
	srv, addr := startDNSServer(t, resolver)
	defer func() { _ = srv.Close() }()

	reqID := uint16(0x1234)
	req := buildDNSQuery(reqID, "live.bit", 1) // type A

	resp := sendDNSQuery(t, addr, req)
	require.GreaterOrEqual(t, len(resp), 12, "response must have header")
	require.Equal(t, reqID, binary.BigEndian.Uint16(resp[0:2]), "transaction ID must match")

	rcode := binary.BigEndian.Uint16(resp[2:4]) & 0x000F
	require.Equal(t, uint16(0), rcode, "expected NOERROR")

	ansCount := binary.BigEndian.Uint16(resp[6:8])
	require.Equal(t, uint16(1), ansCount, "expected one answer")
}

func TestNamecoinDNSServer_NXDomain(t *testing.T) {
	skipIfWIndows(t)

	resolver := staticResolver{}
	srv, addr := startDNSServer(t, resolver)
	defer func() { _ = srv.Close() }()

	reqID := uint16(0xABCD)
	req := buildDNSQuery(reqID, "missing.bit", 1)

	resp := sendDNSQuery(t, addr, req)
	require.GreaterOrEqual(t, len(resp), 12, "response must have header")
	rcode := binary.BigEndian.Uint16(resp[2:4]) & 0x000F
	require.Equal(t, uint16(3), rcode, "expected NXDOMAIN")
}

func startDNSServer(t *testing.T, resolver dns.Resolver) (*dns.Server, *net.UDPAddr) {
	t.Helper()
	srv, err := dns.ListenAndServe("127.0.0.1:0", resolver)
	if err != nil && strings.Contains(err.Error(), "operation not permitted") {
		t.Skip("UDP bind not permitted in this environment")
	}
	require.NoError(t, err)

	udpAddr, err := net.ResolveUDPAddr("udp", srv.Addr())
	require.NoError(t, err)
	return srv, udpAddr
}

func buildDNSQuery(id uint16, domain string, qtype uint16) []byte {
	name := encodeQName(domain)
	buf := make([]byte, 12, 12+len(name)+4)
	binary.BigEndian.PutUint16(buf[0:2], id)
	binary.BigEndian.PutUint16(buf[2:4], 0x0100) // RD=1
	binary.BigEndian.PutUint16(buf[4:6], 1)      // QDCOUNT
	// ANCOUNT, NSCOUNT, ARCOUNT remain zero
	buf = append(buf, name...)
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint16(tmp[0:2], qtype)
	binary.BigEndian.PutUint16(tmp[2:4], 1) // class IN
	buf = append(buf, tmp...)
	return buf
}

func encodeQName(domain string) []byte {
	out := make([]byte, 0, len(domain)+2)
	for _, label := range strings.Split(domain, ".") {
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	out = append(out, 0)
	return out
}

func sendDNSQuery(t *testing.T, addr *net.UDPAddr, req []byte) []byte {
	t.Helper()
	resp, err := sendDNSQueryWithErr(addr, req)
	require.NoError(t, err)
	return resp
}

func sendDNSQueryWithErr(addr *net.UDPAddr, req []byte) ([]byte, error) {
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return nil, err
	}
	if _, err = conn.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func TestNamecoinDNSServer_ConcurrencyCap(t *testing.T) {
	skipIfWIndows(t)

	resolver := staticResolver{
		"live.bit": {
			Domain: "live.bit",
			IP:     "1.2.3.4",
			Status: types.DNSStatusSuccess,
		},
	}
	srv, addr := startDNSServer(t, resolver)
	defer func() { _ = srv.Close() }()

	const parallel = 64 // matches default worker semaphore
	errCh := make(chan error, parallel)
	for i := 0; i < parallel; i++ {
		go func(id uint16) {
			_, err := sendDNSQueryWithErr(addr, buildDNSQuery(id, "live.bit", 1))
			errCh <- err
		}(uint16(i + 1))
	}

	for i := 0; i < parallel; i++ {
		require.NoError(t, <-errCh)
	}
}
