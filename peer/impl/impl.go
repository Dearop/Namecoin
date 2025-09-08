package impl

import (
	"errors"
	"sync"
	"time"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
)

// NewPeer creates a new peer. You can change the content and location of this
// function but you MUST NOT change its signature and package location.
func NewPeer(conf peer.Configuration) peer.Peer {
	// here you must return a struct that implements the peer.Peer functions.
	// Therefore, you are free to rename and change it as you want.
	node := &node{conf: conf, stopCh: make(chan struct{})}
	node.read_write_Mutex.Lock()
	node.routingTable = make(map[string]string)
	node_addr := conf.Socket.GetAddress()
	node.routingTable[node_addr] = node_addr
	node.read_write_Mutex.Unlock()
	return node
}

// node implements a peer to build a Peerster system
//
// - implements peer.Peer
type node struct {
	peer.Peer
	conf peer.Configuration
	stopCh chan struct{}
	wg sync.WaitGroup
	routingTable map[string]string
	read_write_Mutex sync.RWMutex
}

// Start implements peer.Service
func (n *node) Start() error {
	n.wg.Add(1)
	go func() {
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
				continue
			}
			if pkt.Header == nil {
				continue
			}
			node_addr := n.conf.Socket.GetAddress()
			dst := pkt.Header.Destination
			if dst == node_addr {
				_ = n.conf.MessageRegistry.ProcessPacket(pkt)
				continue
			}

			n.read_write_Mutex.RLock()
			nextHop, ok := n.routingTable[dst]
			n.read_write_Mutex.RUnlock()
			if !ok || nextHop == "" {
				nextHop = dst
			}

			pkt.Header.RelayedBy = node_addr
			_ = n.conf.Socket.Send(nextHop, pkt, time.Second)
		}
	}()
	return nil
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
	node_addr := n.conf.Socket.GetAddress()
	if node_addr == "" {
		return nil
	}
	n.read_write_Mutex.RLock()
	defer n.read_write_Mutex.RUnlock()
	header := transport.NewHeader(node_addr, node_addr, dest)
	return n.conf.Socket.Send(dest, transport.Packet{Header: &header, Msg: &msg}, time.Second)
}

// AddPeer implements peer.Messaging
func (n *node) AddPeer(addr ...string) {
	node_addr := n.conf.Socket.GetAddress()
	if node_addr == "" {
		return
	}
	n.read_write_Mutex.Lock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[node_addr] = node_addr
	for _, a := range addr {
		if a == "" || a == node_addr {
			continue
		}
		n.routingTable[a] = a
	}
	n.read_write_Mutex.Unlock()
}

// GetRoutingTable implements peer.Messaging
func (n *node) GetRoutingTable() peer.RoutingTable {
	n.read_write_Mutex.RLock()
	defer n.read_write_Mutex.RUnlock()
	copyTable := make(peer.RoutingTable, len(n.routingTable))
	for k, v := range n.routingTable {
		copyTable[k] = v
	}
	return copyTable
}

// SetRoutingEntry implements peer.Messaging
func (n *node) SetRoutingEntry(origin, relayAddr string) {
	node_addr := n.conf.Socket.GetAddress()
	n.read_write_Mutex.Lock()
	defer n.read_write_Mutex.Unlock()
	if n.routingTable == nil {
		n.routingTable = make(map[string]string)
	}
	n.routingTable[node_addr] = node_addr

	if origin == "" {
		return
	}
	if relayAddr == "" {
		delete(n.routingTable, origin)
		return
	}
	n.routingTable[origin] = relayAddr
}
