package impl

import (
	"sort"
	"strings"
	"time"

	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

// collectMissingRumors collects rumors that are missing from remote.
// It compares local and remote status to identify rumors that need to be sent.
func (n *node) collectMissingRumors(remote types.StatusMessage, local map[string]uint) []types.Rumor {
	if err := n.validateNode(false); err != nil {
		return nil
	}
	if remote == nil || local == nil {
		return nil
	}
	var out []types.Rumor
	for origin, lseq := range local {
		missing := n.collectMissingForOrigin(origin, lseq, remote[origin])
		if len(missing) > 0 {
			out = append(out, missing...)
		}
	}
	return out
}

// collectMissingForOrigin collects missing rumors for a specific origin.
// Returns rumors with sequence numbers between remoteSeq+1 and localSeq.
func (n *node) collectMissingForOrigin(origin string, localSeq uint, remoteSeq uint) []types.Rumor {
	origin = strings.TrimSpace(origin)
	if origin == "" || localSeq <= remoteSeq {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	byOrigin := n.rumors[origin]
	if byOrigin == nil {
		return nil
	}
	expected := int(localSeq - remoteSeq)
	if expected < 0 {
		expected = 0
	}
	out := make([]types.Rumor, 0, expected)
	for seq := remoteSeq + 1; seq <= localSeq; seq++ {
		rm, ok := byOrigin[seq]
		if !ok {
			continue
		}
		if rm.Origin == "" || rm.Sequence == 0 || rm.Msg == nil {
			continue
		}
		out = append(out, rm)
	}
	return out
}

// sendRumorsMessage sends a rumors message to a destination.
// It sorts rumors by origin before sending to ensure consistent ordering.
func (n *node) sendRumorsMessage(rumors []types.Rumor, source, self string) {
	if err := n.validateNode(false); err != nil {
		return
	}
	if len(rumors) == 0 {
		return
	}
	if len(rumors) == 0 || source == "" || self == "" {
		return
	}
	sort.Sort(types.RumorByOrigin(rumors))
	wire, err := n.conf.MessageRegistry.MarshalMessage(types.RumorsMessage{Rumors: rumors})
	if err != nil {
		return
	}
	header := transport.NewHeader(self, self, source)
	_ = n.conf.Socket.Send(source, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

// sendMissingRumorsTo sends missing rumors to a source peer.
// It collects missing rumors and sends them in a rumors message.
func (n *node) sendMissingRumorsTo(source, self string, remote types.StatusMessage, local map[string]uint) {
	if err := n.validateNode(false); err != nil {
		return
	}
	if remote == nil || local == nil {
		return
	}

	source = strings.TrimSpace(source)
	self = strings.TrimSpace(self)
	if source == "" || self == "" {
		return
	}

	rumors := n.collectMissingRumors(remote, local)
	n.sendRumorsMessage(rumors, source, self)
}

// processRumorIfExpected validates and processes a single rumor if it is the next expected one.
// It stores the rumor, updates routing, and returns true if the rumor was processed.
func (n *node) processRumorIfExpected(r types.Rumor, header *transport.Header) bool {
	if err := n.validateNode(false); err != nil {
		return false
	}
	if header == nil {
		return false
	}

	origin := strings.TrimSpace(r.Origin)
	if origin == "" || r.Sequence == 0 || r.Msg == nil {
		return false
	}
	if r.Msg.Type == "" {
		return false
	}

	n.mu.RLock()
	last := n.lastSeq[origin]
	n.mu.RUnlock()
	if last+1 != r.Sequence {
		return false
	}

	// process embedded message locally using a header whose source reflects the rumor origin
	msgHeader := *header
	if origin != "" {
		msgHeader.Source = origin
	}
	if strings.TrimSpace(msgHeader.Destination) == "" {
		msgHeader.Destination = n.conf.Socket.GetAddress()
	}
	_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: &msgHeader, Msg: r.Msg})

	// update last sequence, store rumor, and maybe routing
	n.mu.Lock()
	n.lastSeq[origin] = r.Sequence
	if n.rumors[origin] == nil {
		n.rumors[origin] = make(map[uint]types.Rumor)
	}
	n.rumors[origin][r.Sequence] = r

	relay := strings.TrimSpace(header.RelayedBy)
	if relay != "" && origin != n.conf.Socket.GetAddress() {
		currentRelay := n.routingTable[origin]
		if currentRelay != origin && currentRelay != relay {
			n.routingTable[origin] = relay
		}
	}
	n.mu.Unlock()
	return true
}
