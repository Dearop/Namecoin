package impl

import (
	"net"
	"strings"

	"go.dedis.ch/cs438/types"
)

// chainReader abstracts the minimal view needed for DNS resolution.
type chainReader interface {
	SnapshotDomains() (map[string]types.NameRecord, uint64)
}

type NamecoinDNS struct {
	requestChan  chan types.DNSRequest
	responseChan chan types.DNSResponse
	reader       chainReader
}

func NewNamecoinDNS() *NamecoinDNS {
	return &NamecoinDNS{
		requestChan:  make(chan types.DNSRequest, 16),
		responseChan: make(chan types.DNSResponse, 16),
	}
}

// HandleRequest enqueues a DNS lookup request.
func (dns *NamecoinDNS) HandleRequest(request types.DNSRequest) {
	dns.requestChan <- request
}

// Responses exposes the response channel for consumers to read replies.
func (dns *NamecoinDNS) Responses() <-chan types.DNSResponse {
	return dns.responseChan
}

// Start begins processing requests using the node's Namecoin chain/state with locks held.
func (dns *NamecoinDNS) Start(n *node) {
	if n != nil {
		dns.reader = n.namecoin
	}
	go func() {
		for request := range dns.requestChan {
			dns.responseChan <- dns.resolve(request)
		}
	}()
}

// BindReader allows tests or callers to inject a chain reader directly.
func (dns *NamecoinDNS) BindReader(r chainReader) {
	dns.reader = r
}

func (dns *NamecoinDNS) resolve(request types.DNSRequest) types.DNSResponse {
	resp := types.DNSResponse{
		Domain: request.Domain,
		Status: types.DNSStatusInvalid,
	}

	if dns.reader == nil {
		return resp
	}

	domains, height := dns.reader.SnapshotDomains()
	rec, ok := domains[request.Domain]

	if !ok {
		resp.Status = types.DNSStatusNXDomain
		return resp
	}
	// Check if the domain is expired
	if rec.ExpiresAt != 0 && rec.ExpiresAt <= height {
		resp.Status = types.DNSStatusNXDomain
		return resp
	}
	// Add Time to Live granularity to the response
	value := strings.TrimSpace(rec.Value)
	if value == "" {
		resp.Status = types.DNSStatusInvalid
		return resp
	}
	// Check for valid IP address (could do in IP assimilation to avoid response complexity)
	if ip := net.ParseIP(value); ip != nil {
		resp.IP = ip.String()
		if rec.ExpiresAt > height {
			resp.TTL = rec.ExpiresAt - height
		}
		resp.Status = types.DNSStatusSuccess
		return resp
	}

	resp.TXTRecord = value
	if rec.ExpiresAt > height {
		resp.TTL = rec.ExpiresAt - height
	}
	resp.Status = types.DNSStatusSuccess
	return resp
}

// Resolve performs a synchronous resolution using the bound node (if any).
func (dns *NamecoinDNS) Resolve(domain string) types.DNSResponse {
	return dns.resolve(types.DNSRequest{Domain: domain})
}
