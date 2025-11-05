package impl

import (
	"sort"
	"strings"
	"time"

	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

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

	var out []types.Rumor
	for origin, lseq := range local {
		if origin == "" {
			continue
		}
		rseq := remote[origin]
		if lseq <= rseq {
			continue
		}
		// collect rumors (rseq+1 .. lseq)
		n.mu.RLock()
		for seq := rseq + 1; seq <= lseq; seq++ {
			if byOrigin := n.rumors[origin]; byOrigin != nil {
				if rm, ok := byOrigin[seq]; ok {
					if rm.Origin != "" && rm.Sequence > 0 && rm.Msg != nil {
						out = append(out, rm)
					}
				}
			}
		}
		n.mu.RUnlock()
	}
	if len(out) == 0 || source == "" {
		return
	}
	sort.Sort(types.RumorByOrigin(out))
	wire, err := n.conf.MessageRegistry.MarshalMessage(types.RumorsMessage{Rumors: out})
	if err != nil {
		return
	}
	if self == "" {
		return
	}
	header := transport.NewHeader(self, self, source)
	_ = n.conf.Socket.Send(source, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

// processRumorIfExpected validates and processes a single rumor if it is the next
// expected one from its origin. It also stores the rumor and updates routing
// based on the relayedBy header. It returns true if the rumor was processed.
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
