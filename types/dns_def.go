package types

// DNS state is declared in namecoin_state.go and uses the NameRecord struct

type DNSRequest struct {
	Domain string
}

// DNSStatus captures the outcome of a resolution attempt.
type DNSStatus string

// Enum for DNSStatus following architecture
const (
	DNSStatusSuccess  DNSStatus = "SUCCESS"
	DNSStatusNXDomain DNSStatus = "NXDOMAIN"
	DNSStatusInvalid  DNSStatus = "INVALID"
)

// As in architecture, we return either IP or TXT record
type DNSResponse struct {
	Domain    string
	IP        string
	TXTRecord string
	Status    DNSStatus
	TTL       uint64
}
