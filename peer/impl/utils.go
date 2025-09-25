package impl

import (
	"strings"
	"time"
	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

func (n *node) maybeContinueMongering(exclude string) {
	if n.conf.ContinueMongering <= 0 {
		return
	}
	// probabilistic trigger, but also rate-limit using AntiEntropyInterval
	if float64(time.Now().UnixNano()%1000)/1000.0 >= n.conf.ContinueMongering {
		return
	}
	if n.conf.AntiEntropyInterval > 0 {
		n.lastStatusMu.Lock()
		if !n.lastStatusAt.IsZero() && time.Since(n.lastStatusAt) < n.conf.AntiEntropyInterval {
			n.lastStatusMu.Unlock()
			return
		}
		n.lastStatusAt = time.Now()
		n.lastStatusMu.Unlock()
	}
	// pick neighbor not equal to exclude
	nodeAddr := n.conf.Socket.GetAddress()
	n.mu.RLock()
	neighbors := make([]string, 0, len(n.routingTable))
	for origin, relay := range n.routingTable {
		if origin == relay && origin != nodeAddr && origin != exclude {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()
	if len(neighbors) == 0 {
		return
	}
	dest := neighbors[int(time.Now().UnixNano())%len(neighbors)]
	status := n.buildStatus()
	wire, err := n.conf.MessageRegistry.MarshalMessage(status)
	if err != nil {
		return
	}
	header := transport.NewHeader(nodeAddr, nodeAddr, dest)
	_ = n.conf.Socket.Send(dest, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

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
	if strings.TrimSpace(pkt.Header.Destination) == "" {
		return xerrors.Errorf("empty destination")
	}
	return nil
}

func (n *node) lookupNextHop(dest string) (string, bool) {
	n.mu.RLock()
	relay, ok := n.routingTable[dest]
	n.mu.RUnlock()
	if !ok || relay == "" {
		return "", false
	}
	return relay, true
}

func (n *node) processPacket(pkt transport.Packet) {
	if err := n.validateRecvPacket(pkt); err != nil {
		return
	}
	nodeAddr, err := n.ensureConfigured()
	if err != nil {
		return
	}
	dest := strings.TrimSpace(pkt.Header.Destination)
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