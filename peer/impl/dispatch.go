package impl

import (
	"strings"
	"time"

	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

func (n *node) ensureConfigured() (string, error) {
	if n == nil || n.conf.Socket == nil {
		return "", xerrors.Errorf("socket not configured")
	}
	addr := n.conf.Socket.GetAddress()
	if strings.TrimSpace(addr) == "" {
		return "", xerrors.Errorf("node address not set")
	}
	return addr, nil
}

func (n *node) validateRecvPacket(pkt transport.Packet) error {
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}
	if pkt.Msg == nil {
		return xerrors.Errorf("missing message")
	}
	// Optimize: check length first, only trim if needed
	dest := pkt.Header.Destination
	if len(dest) == 0 {
		return xerrors.Errorf("empty destination")
	}
	// Only trim if we detect whitespace
	if dest[0] == ' ' || dest[len(dest)-1] == ' ' {
		dest = strings.TrimSpace(dest)
		if dest == "" {
			return xerrors.Errorf("empty destination")
		}
	}
	return nil
}

func (n *node) processPacket(pkt transport.Packet) {
	if n == nil {
		return
	}
	if err := n.validateRecvPacket(pkt); err != nil {
		return
	}
	nodeAddr, err := n.ensureConfigured()
	if err != nil {
		return
	}
	// Optimize: only trim if needed
	dest := pkt.Header.Destination
	if len(dest) > 0 && (dest[0] == ' ' || dest[len(dest)-1] == ' ') {
		dest = strings.TrimSpace(dest)
	}
	if dest == "" {
		return
	}
	if dest == nodeAddr {
		_ = n.conf.MessageRegistry.ProcessPacket(pkt)
		return
	}
	nextHop, ok := n.lookupNextHop(dest)
	if !ok {
		return
	}
	pkt.Header.RelayedBy = nodeAddr
	_ = n.conf.Socket.Send(nextHop, pkt, time.Second)
}
