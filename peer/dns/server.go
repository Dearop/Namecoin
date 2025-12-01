package dns

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"go.dedis.ch/cs438/types"
)

// Resolver resolves a domain into a DNSResponse.
type Resolver interface {
	Resolve(domain string) types.DNSResponse
}

// Server is a minimal UDP DNS server for .bit domains.
type Server struct {
	conn        net.PacketConn
	resolver    Resolver
	workerSem   chan struct{}
	wg          sync.WaitGroup
	stop        chan struct{}
	readTimeout time.Duration
}

const (
	maxDNSPacketSize = 512
	defaultTTL       = 60
	defaultWorkers   = 64
	maxTXTSize       = 255
)

// ListenAndServe starts a DNS server on addr (e.g., ":5354").
// It limits concurrent handlers to mitigate DoS.
func ListenAndServe(addr string, resolver Resolver) (*Server, error) {
	if resolver == nil {
		return nil, errors.New("dns: nil resolver")
	}
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{
		conn:        pc,
		resolver:    resolver,
		workerSem:   make(chan struct{}, defaultWorkers),
		stop:        make(chan struct{}),
		readTimeout: 5 * time.Second,
	}
	s.wg.Add(1)
	go s.loop()
	return s, nil
}

// Close stops the server and closes the socket.
func (s *Server) Close() error {
	close(s.stop)
	err := s.conn.Close()
	s.wg.Wait()
	return err
}

func (s *Server) loop() {
	defer s.wg.Done()
	buf := make([]byte, maxDNSPacketSize)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				select {
				case <-s.stop:
					return
				default:
					continue
				}
			}
			return
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])

		select {
		case s.workerSem <- struct{}{}:
			go func() {
				defer func() { <-s.workerSem }()
				s.handlePacket(packet, addr)
			}()
		default:
			// Too many concurrent requests; drop to avoid DoS.
		}
	}
}

func (s *Server) handlePacket(msg []byte, addr net.Addr) {
	resp, ok := s.buildResponse(msg)
	if !ok {
		return
	}
	_, _ = s.conn.WriteTo(resp, addr)
}

func (s *Server) buildResponse(msg []byte) ([]byte, bool) {
	if len(msg) < 12 || len(msg) > maxDNSPacketSize {
		return nil, false
	}

	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	rd := flags&0x0100 != 0
	opcode := (flags >> 11) & 0xF
	qdCount := binary.BigEndian.Uint16(msg[4:6])

	// Only handle standard queries with one question.
	if opcode != 0 || qdCount != 1 {
		return s.formatErrorResponse(id, rd), true
	}

	qname, qtype, qclass, off, perr := parseQuestion(msg, 12)
	if perr != nil {
		return s.formatErrorResponse(id, rd), true
	}

	// Only IN class is supported.
	if qclass != 1 {
		return s.notImplementedResponse(id, rd, msg[12:off]), true
	}

	// Restrict to .bit domains.
	nameLower := strings.ToLower(qname)
	if !strings.HasSuffix(nameLower, ".bit") {
		return s.nxResponse(id, rd, msg[12:off]), true
	}

	r := s.resolver.Resolve(nameLower)

	switch r.Status {
	case types.DNSStatusSuccess:
		// proceed to build answer
	case types.DNSStatusNXDomain:
		return s.nxResponse(id, rd, msg[12:off]), true
	case types.DNSStatusInvalid:
		return s.servFailResponse(id, rd, msg[12:off]), true
	default:
		return s.servFailResponse(id, rd, msg[12:off]), true
	}

	ttl := uint32(defaultTTL)
	if r.TTL > 0 && r.TTL < uint64(^uint32(0)) {
		ttl = uint32(r.TTL)
	}

	answer, ok := buildAnswer(qtype, ttl, r)
	if !ok {
		// Unsupported qtype: send empty NOERROR response.
		return s.emptyResponse(id, rd, msg[12:off]), true
	}

	// Build final message.
	ansCount := uint16(1)
	resp := make([]byte, 0, maxDNSPacketSize)
	resp = appendHeader(resp, id, rd, 0, ansCount)
	resp = append(resp, msg[12:off]...)
	resp = append(resp, answer...)
	return resp, true
}

// DNS encoding helpers.


// parseQuestion parses the question section of a DNS message.
// It returns the name, type, class, and offset of the end of the question section.
// It also returns an error if the question section is truncated or invalid.
// The name is a list of labels separated by dots.
// The type is a 16-bit unsigned integer.
// The class is a 16-bit unsigned integer.
// The offset is the index of the first byte of the next section.
func parseQuestion(msg []byte, off int) (string, uint16, uint16, int, error) {
	labels := make([]string, 0, 4)
	for {
		if off >= len(msg) {
			return "", 0, 0, off, errors.New("truncated name")
		}
		l := int(msg[off])
		off++
		if l == 0 {
			break
		}
		if l&0xC0 != 0 || l > 63 {
			return "", 0, 0, off, errors.New("invalid label")
		}
		if off+l > len(msg) {
			return "", 0, 0, off, errors.New("truncated label")
		}
		label := msg[off : off+l]
		off += l
		labels = append(labels, string(label))
	}
	if len(labels) == 0 {
		return "", 0, 0, off, errors.New("empty name")
	}
	name := strings.Join(labels, ".")
	if len(name) > 255 {
		return "", 0, 0, off, errors.New("name too long")
	}
	if off+4 > len(msg) {
		return "", 0, 0, off, errors.New("truncated question")
	}
	qtype := binary.BigEndian.Uint16(msg[off : off+2])
	qclass := binary.BigEndian.Uint16(msg[off+2 : off+4])
	off += 4
	return name, qtype, qclass, off, nil
}

func appendHeader(buf []byte, id uint16, rd bool, rcode uint16, anCount uint16) []byte {
	flags := uint16(0x8000) // QR=1
	if rd {
		flags |= 0x0100
	}
	flags |= rcode & 0xF
	// AA=1 to indicate authoritative answers from local resolver.
	flags |= 0x0400
	tmp := make([]byte, 12)
	binary.BigEndian.PutUint16(tmp[0:2], id)
	binary.BigEndian.PutUint16(tmp[2:4], flags)
	binary.BigEndian.PutUint16(tmp[4:6], 1)
	binary.BigEndian.PutUint16(tmp[6:8], anCount)
	// NSCOUNT, ARCOUNT remain zero.
	return append(buf, tmp...)
}

// buildAnswer builds the answer section of the DNS response.
func buildAnswer(qtype uint16, ttl uint32, r types.DNSResponse) ([]byte, bool) {
	const (
		typeA    = 1
		typeTXT  = 16
		typeAAAA = 28
	)

	switch qtype {
	case typeA, typeAAAA:
		ip := net.ParseIP(r.IP)
		if ip == nil {
			return nil, false
		}
		var data []byte
		if qtype == typeAAAA {
			ip16 := ip.To16()
			if ip16 == nil || strings.Contains(r.IP, ".") {
				return nil, false
			}
			data = ip16
		} else {
			ip4 := ip.To4()
			if ip4 == nil {
				return nil, false
			}
			data = ip4
		}
		ans := make([]byte, 0, 16)
		ans = appendNamePtr(ans)
		ans = appendTypeClassTTL(ans, qtype, ttl)
		ans = appendRData(ans, data)
		return ans, true

	case typeTXT:
		txt := []byte(r.TXTRecord)
		if len(txt) == 0 {
			return nil, false
		}
		if len(txt) > maxTXTSize {
			txt = txt[:maxTXTSize]
		}
		data := make([]byte, 0, 1+len(txt))
		data = append(data, byte(len(txt)))
		data = append(data, txt...)
		ans := make([]byte, 0, 16+len(txt))
		ans = appendNamePtr(ans)
		ans = appendTypeClassTTL(ans, typeTXT, ttl)
		ans = appendRData(ans, data)
		return ans, true
	}
	return nil, false
}

// appendNamePtr appends the name pointer to the buffer.
func appendNamePtr(buf []byte) []byte {
	// Pointer to offset 12 (start of question name).
	return append(buf, 0xC0, 0x0C)
}

// appendTypeClassTTL appends the type, class, and TTL to the buffer.
func appendTypeClassTTL(buf []byte, qtype uint16, ttl uint32) []byte {
	tmp := make([]byte, 8)
	binary.BigEndian.PutUint16(tmp[0:2], qtype)
	binary.BigEndian.PutUint16(tmp[2:4], 1) // IN
	binary.BigEndian.PutUint32(tmp[4:8], ttl)
	buf = append(buf, tmp...)
	return buf
}

// appendRData appends the data to the buffer.
func appendRData(buf []byte, data []byte) []byte {
	tmp := make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, uint16(len(data)))
	buf = append(buf, tmp...)
	return append(buf, data...)
}

// formatErrorResponse formats a DNS error response.
func (s *Server) formatErrorResponse(id uint16, rd bool) []byte {
	return appendHeader(nil, id, rd, 1, 0) // FORMERR
}

// servFailResponse formats a DNS server failure response.
func (s *Server) servFailResponse(id uint16, rd bool, question []byte) []byte {
	resp := appendHeader(nil, id, rd, 2, 0)
	resp = append(resp, question...)
	return resp
}

// nxResponse formats a DNS NXDOMAIN response.
func (s *Server) nxResponse(id uint16, rd bool, question []byte) []byte {
	resp := appendHeader(nil, id, rd, 3, 0)
	resp = append(resp, question...)
	return resp
}

func (s *Server) notImplementedResponse(id uint16, rd bool, question []byte) []byte {
	resp := appendHeader(nil, id, rd, 4, 0)
	resp = append(resp, question...)
	return resp
}

func (s *Server) emptyResponse(id uint16, rd bool, question []byte) []byte {
	resp := appendHeader(nil, id, rd, 0, 0)
	resp = append(resp, question...)
	return resp
}
