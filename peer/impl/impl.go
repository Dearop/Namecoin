package impl

import (
	"log"
	"os"
	"strings"
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
	node.mu.Lock()
	node.routingTable = make(map[string]string)
	if conf.Socket != nil {
		nodeAddr := conf.Socket.GetAddress()
		if nodeAddr != "" {
			node.routingTable[nodeAddr] = nodeAddr
		}
	}
	node.mu.Unlock()
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
	mu           sync.RWMutex
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
		n.mu.Lock()
		if n.routingTable == nil {
			n.routingTable = make(map[string]string)
		}
		n.mu.Unlock()
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
func (n *node) validateRecvPacket(pkt transport.Packet) error {
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}
	if pkt.Msg == nil {
		return xerrors.Errorf("missing message")
	}
	if pkt.Header.Destination == "" {
		return xerrors.Errorf("empty destination")
	}
	return nil
}

// lookupNextHop returns the relay address for a given destination, if present.
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

func (n *node) listenLoop() {
	if err := n.validateNode(false); err != nil {
		return
	}
	var errorCount int
	defer n.wg.Done()
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}
		pkt, err := n.conf.Socket.Recv(time.Second * 1)
		if err != nil {
			if xerrors.Is(err, transport.TimeoutError(0)) {
				continue
			}
			errorCount++
			if errorCount > 10 {
				_ = n.Stop()
				if os.Getenv("GLOG") != "no" {
					log.Printf("High error count, stopping node")
				}
				return
			}
			continue
		}
		if errorCount > 0 {
			errorCount = 0
		}
		n.processPacket(pkt)
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
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return xerrors.Errorf("empty destination")
	}
	// Basic fuzz-hardening on message: ensure type is non-empty
	if strings.TrimSpace(msg.Type) == "" {
		return xerrors.Errorf("invalid message: empty type")
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
	n.mu.Lock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[nodeAddr] = nodeAddr
	for _, peerAddr := range addr {
		peerAddr = strings.TrimSpace(peerAddr)
		if peerAddr == "" || peerAddr == nodeAddr {
			continue
		}
		n.routingTable[peerAddr] = peerAddr
	}
	n.mu.Unlock()
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	if err := n.validateNode(false); err != nil {
		return peer.RoutingTable{}
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	copyTable := make(peer.RoutingTable, len(n.routingTable))
	for dest, relay := range n.routingTable {
		copyTable[dest] = relay
	}
	return copyTable
}

// SetRoutingEntry implements peer.Messaging
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	if err := n.validateNode(false); err != nil {
		return
	}
	nodeAddr := n.conf.Socket.GetAddress()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[nodeAddr] = nodeAddr

	origin = strings.TrimSpace(origin)
	relayAddr = strings.TrimSpace(relayAddr)
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
