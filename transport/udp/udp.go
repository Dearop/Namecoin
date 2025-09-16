package udp

import (
	"net"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// It is advised to define a constant (max) size for all relevant byte buffers, e.g:
// const bufSize = 65000

// NewUDP returns a new udp transport implementation.
func NewUDP() transport.Transport {
	return &UDP{}
}

// UDP implements a transport layer using UDP
//
// - implements transport.Transport
type UDP struct{}

// CreateSocket implements transport.Transport
func (n *UDP) CreateSocket(address string) (transport.ClosableSocket, error) {
	if address == "" {
		return nil, xerrors.Errorf("empty address")
	}
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	return &Socket{
		conn: conn,
		addr: conn.LocalAddr().String(),
		ins:  make([]transport.Packet, 0),
		outs: make([]transport.Packet, 0),
	}, nil
}

// Socket implements a network socket using UDP.
//
// - implements transport.Socket
// - implements transport.ClosableSocket
type Socket struct {
	conn   *net.UDPConn
	addr   string
	mu     sync.Mutex
	closed bool
	insMu  sync.Mutex
	ins    []transport.Packet
	outsMu sync.Mutex
	outs   []transport.Packet
}

func (s *Socket) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return xerrors.Errorf("socket already closed")
	}
	s.closed = true
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// Send implements transport.Socket
func (s *Socket) Send(dest string, pkt transport.Packet, timeout time.Duration) error {
	if s == nil || s.conn == nil {
		return xerrors.Errorf("socket not initialized")
	}
	if dest == "" {
		return xerrors.Errorf("empty destination")
	}
	if err := validateSendPacket(pkt); err != nil {
		return err
	}
	buf, err := pkt.Marshal()
	if err != nil {
		return err
	}

	udpAddr, err := net.ResolveUDPAddr("udp", dest)
	if err != nil {
		return err
	}

	if timeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(timeout))
	} else {
		_ = s.conn.SetWriteDeadline(time.Time{})
	}

	_, err = s.conn.WriteToUDP(buf, udpAddr)
	if err != nil {
		var ne net.Error
		if xerrors.As(err, &ne) && ne.Timeout() {
			return transport.TimeoutError(timeout)
		}
		return err
	}

	s.outsMu.Lock()
	s.outs = append(s.outs, pkt.Copy())
	s.outsMu.Unlock()

	return nil
}

// Recv implements transport.Socket. It blocks until a packet is received, or
// the timeout is reached. In the case the timeout is reached, return a
// TimeoutErr.
func (s *Socket) Recv(timeout time.Duration) (transport.Packet, error) {
	if s == nil || s.conn == nil {
		return transport.Packet{}, xerrors.Errorf("socket not initialized")
	}
	if timeout < 0 {
		return transport.Packet{}, xerrors.Errorf("negative timeout")
	}
	buf := make([]byte, 65000)

	if timeout > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(timeout))
	} else {
		_ = s.conn.SetReadDeadline(time.Time{})
	}

	n, _, err := s.conn.ReadFromUDP(buf)
	if err != nil {
		var ne net.Error
		if xerrors.As(err, &ne) && ne.Timeout() {
			return transport.Packet{}, transport.TimeoutError(timeout)
		}
		return transport.Packet{}, err
	}
	if n == 0 {
		return transport.Packet{}, xerrors.Errorf("empty udp datagram")
	}

	var pkt transport.Packet
	if err := pkt.Unmarshal(buf[:n]); err != nil {
		return transport.Packet{}, err
	}
	if err := validateRecvPacket(pkt); err != nil {
		return transport.Packet{}, err
	}

	s.insMu.Lock()
	s.ins = append(s.ins, pkt.Copy())
	s.insMu.Unlock()

	return pkt, nil
}

// validateSendPacket ensures a packet is valid to be sent.
func validateSendPacket(pkt transport.Packet) error {
	if pkt.Header == nil {
		return xerrors.Errorf("invalid packet: missing header")
	}
	if pkt.Msg == nil {
		return xerrors.Errorf("invalid packet: missing message")
	}
	if pkt.Header.Destination == "" {
		return xerrors.Errorf("invalid packet: empty destination")
	}
	return nil
}

// validateRecvPacket ensures a packet received from the network is structurally valid.
func validateRecvPacket(pkt transport.Packet) error {
	if pkt.Header == nil {
		return xerrors.Errorf("received packet without header")
	}
	if pkt.Msg == nil {
		return xerrors.Errorf("received packet without message")
	}
	return nil
}

// GetAddress implements transport.Socket. It returns the address assigned. Can
// be useful in the case one provided a :0 address, which makes the system use a
// random free port.
func (s *Socket) GetAddress() string {
	return s.addr
}

// GetIns implements transport.Socket
func (s *Socket) GetIns() []transport.Packet {
	s.insMu.Lock()
	defer s.insMu.Unlock()

	res := make([]transport.Packet, len(s.ins))
	for i, p := range s.ins {
		res[i] = p.Copy()
	}
	return res
}

// GetOuts implements transport.Socket
func (s *Socket) GetOuts() []transport.Packet {
	s.outsMu.Lock()
	defer s.outsMu.Unlock()

	res := make([]transport.Packet, len(s.outs))
	for i, p := range s.outs {
		res[i] = p.Copy()
	}
	return res
}
