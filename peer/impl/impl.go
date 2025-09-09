package impl

import (
	"errors"
	"sync"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// NewPeer creates a new peer. You can change the content and location of this
// function but you MUST NOT change its signature and package location.
func NewPeer(conf peer.Configuration) peer.Peer {
	// here you must return a struct that implements the peer.Peer functions.
	// Therefore, you are free to rename and change it as you want.
	node := &node{conf: conf, stopCh: make(chan struct{})}
	node.ReWrMu.Lock()
	node.routingTable = make(map[string]string)
	if conf.Socket != nil {
		nodeAddr := conf.Socket.GetAddress()
		if nodeAddr != "" {
			node.routingTable[nodeAddr] = nodeAddr
		}
	}
	node.ReWrMu.Unlock()
	return node
}

// node implements a peer to build a Peerster system
//
// - implements peer.Peer
type node struct {
	peer.Peer
	conf         peer.Configuration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	routingTable map[string]string
	ReWrMu       sync.RWMutex
}

// Start implements peer.Service
func (n *node) Start() error {
	if n == nil {
		return xerrors.Errorf("nil node")
	}
	if n.conf.Socket == nil {
		return xerrors.Errorf("socket not configured")
	}
	if n.conf.MessageRegistry == nil {
		return xerrors.Errorf("registry not configured")
	}
	n.wg.Add(1)
	go n.listenLoop()
	return nil
}

func (n *node) listenLoop() {
	defer n.wg.Done()
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}
		pkt, err := n.conf.Socket.Recv(time.Second * 1)
		if err != nil {
			if errors.Is(err, transport.TimeoutError(0)) {
				continue
			}
			// unexpected recv error: keep loop alive
			continue
		}
		if pkt.Header == nil {
			continue
		}
		nodeAddr := n.conf.Socket.GetAddress()
		dst := pkt.Header.Destination
		if dst == nodeAddr {
			_ = n.conf.MessageRegistry.ProcessPacket(pkt)
			continue
		}

		n.ReWrMu.RLock()
		nextHop, ok := n.routingTable[dst]
		n.ReWrMu.RUnlock()
		if !ok || nextHop == "" {
			nextHop = dst
		}

		pkt.Header.RelayedBy = nodeAddr
		_ = n.conf.Socket.Send(nextHop, pkt, time.Second)
	}
}

// Stop implements peer.Service
func (n *node) Stop() error {
	select {
	case <-n.stopCh:
	default:
		close(n.stopCh)
	}
	n.wg.Wait()
	return nil
}

// Unicast implements peer.Messaging
func (n *node) Unicast(dest string, msg transport.Message) error {
	nodeAddr := n.conf.Socket.GetAddress()
	if nodeAddr == "" {
		return xerrors.Errorf("node address not set")
	}
	if dest == "" {
		return xerrors.Errorf("empty destination")
	}
	n.ReWrMu.RLock()
	nextHop, ok := n.routingTable[dest]
	n.ReWrMu.RUnlock()
	if !ok || nextHop == "" {
		return xerrors.Errorf("destination %s unknown", dest)
	}
	header := transport.NewHeader(nodeAddr, nodeAddr, dest)
	return n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &msg}, time.Second)
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	nodeAddr := n.conf.Socket.GetAddress()
	if nodeAddr == "" {
		return
	}
	n.ReWrMu.Lock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[nodeAddr] = nodeAddr
	for _, a := range addr {
		if a == "" || a == nodeAddr {
			continue
		}
		n.routingTable[a] = a
	}
	n.ReWrMu.Unlock()
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	n.ReWrMu.RLock()
	defer n.ReWrMu.RUnlock()
	copyTable := make(peer.RoutingTable, len(n.routingTable))
	for k, v := range n.routingTable {
		copyTable[k] = v
	}
	return copyTable
}

// SetRoutingEntry implements peer.Messaging
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	nodeAddr := n.conf.Socket.GetAddress()
	n.ReWrMu.Lock()
	defer n.ReWrMu.Unlock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[nodeAddr] = nodeAddr

	if origin == "" {
		return
	}
	if relayAddr == "" {
		delete(n.routingTable, origin)
		return
	}
	n.routingTable[origin] = relayAddr
}
