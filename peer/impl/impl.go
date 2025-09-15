package impl

import (
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
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
	if err := n.validateNode(true); err != nil {
		return err
	}
	if n.stopCh == nil {
		n.stopCh = make(chan struct{})
	}
	if n.routingTable == nil {
		n.ReWrMu.Lock()
		if n.routingTable == nil {
			n.routingTable = make(map[string]string)
		}
		n.ReWrMu.Unlock()
	}
	n.conf.MessageRegistry.RegisterMessageCallback(types.ChatMessage{}, n.handleChatMessage)
	n.wg.Add(1)
	go n.listenLoop()
	return nil
}

func (n *node) handleChatMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	chat, ok := m.(*types.ChatMessage)
	if !ok || chat == nil {
		return xerrors.Errorf("unexpected message type")
	}
	src := "unknown"
	if pkt.Header != nil && pkt.Header.Source != "" {
		src = pkt.Header.Source
	}
	if os.Getenv("GLOG") != "no" {
		log.Printf("[chat] from: %s, msg: %s", src, chat.Message)
	}
	return nil
}

// ensureConfigured returns the node address if the node and its socket are properly configured.
func (n *node) ensureConfigured() (string, error) {
	if n == nil || n.conf.Socket == nil {
		return "", xerrors.Errorf("socket not configured")
	}
	addr := n.conf.Socket.GetAddress()
	if addr == "" {
		return "", xerrors.Errorf("node address not set")
	}
	return addr, nil
}

// validateRecvPacket checks that a received packet has the required fields.
func (n *node) validateRecvPacket(packet transport.Packet) error {
	if packet.Header == nil {
		return xerrors.Errorf("missing header")
	}
	if packet.Msg == nil {
		return xerrors.Errorf("missing message")
	}
	if packet.Header.Destination == "" {
		return xerrors.Errorf("empty destination")
	}
	return nil
}

// lookupNextHop returns the relay address for a given destination, if present.
func (n *node) lookupNextHop(destination string) (string, bool) {
	n.ReWrMu.RLock()
	relay, ok := n.routingTable[destination]
	n.ReWrMu.RUnlock()
	if !ok || relay == "" {
		return "", false
	}
	return relay, true
}

func (n *node) listenLoop() {
	if err := n.validateNode(false); err != nil {
		return
	}
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
		}

		if err := n.validateRecvPacket(pkt); err != nil {
			continue
		}
		nodeAddr, err := n.ensureConfigured()
		if err != nil {
			continue
		}
		dst := pkt.Header.Destination
		if dst == nodeAddr {
			_ = n.conf.MessageRegistry.ProcessPacket(pkt)
			continue
		}

		nextHop, ok := n.lookupNextHop(dst)
		if !ok {
			continue
		}

		pkt.Header.RelayedBy = nodeAddr
		_ = n.conf.Socket.Send(nextHop, pkt, time.Second)
	}
}

// Stop implements peer.Service
func (n *node) Stop() error {
	if err := n.validateNode(false); err != nil {
		return err
	}
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
	if err := n.validateNode(false); err != nil {
		return err
	}
	nodeAddr := n.conf.Socket.GetAddress()
	if dest == "" {
		return xerrors.Errorf("empty destination")
	}
	nextHop, ok := n.lookupNextHop(dest)
	if !ok {
		return xerrors.Errorf("destination %s unknown", dest)
	}
	header := transport.NewHeader(nodeAddr, nodeAddr, dest)
	return n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &msg}, time.Second)
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	if err := n.validateNode(false); err != nil {
		return
	}
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
	if err := n.validateNode(false); err != nil {
		return peer.RoutingTable{}
	}
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
	if err := n.validateNode(false); err != nil {
		return
	}
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

// validateNode checks the receiver and its critical configuration.
// If requireRegistry is true, it also enforces a non-nil MessageRegistry.
func (n *node) validateNode(requireRegistry bool) error {
	if n == nil {
		return xerrors.Errorf("nil node")
	}
	if n.conf.Socket == nil {
		return xerrors.Errorf("socket not configured")
	}
	if n.conf.Socket.GetAddress() == "" {
		return xerrors.Errorf("node address not set")
	}
	if requireRegistry && n.conf.MessageRegistry == nil {
		return xerrors.Errorf("registry not configured")
	}
	return nil
}
