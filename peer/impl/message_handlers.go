package impl

import (
    "log"
    "os"
    "strings"
    "time"

	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

func (n *node) handleChatMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	chat, ok := m.(*types.ChatMessage)
	if !ok || chat == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil || pkt.Msg == nil {
		return xerrors.Errorf("invalid packet")
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

func (n *node) handleRumorsMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	rumorsMsg, ok := m.(*types.RumorsMessage)
	if !ok || rumorsMsg == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}

	// Process expected rumors and collect newly accepted ones
	forwardable := n.acceptExpectedRumors(rumorsMsg.Rumors, pkt.Header)

	// Send ack back to source (directly) with the receiver's current status
	source := strings.TrimSpace(pkt.Header.Source)
	if source != "" {
		ack := types.AckMessage{
			AckedPacketID: pkt.Header.PacketID,
			Status:        n.buildStatus(),
		}
		wireAck, err := n.conf.MessageRegistry.MarshalMessage(ack)
		if err == nil {
			header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), source)
			_ = n.conf.Socket.Send(source, transport.Packet{Header: &header, Msg: &wireAck}, time.Second)
		}
	}
	// Forward only the newly accepted rumors once to a random neighbor (not the source)
	n.forwardAcceptedRumorsOnce(forwardable, pkt.Header)
	return nil
}

func (n *node) handlePrivateMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	pm, ok := m.(*types.PrivateMessage)
	if !ok || pm == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil || pm.Msg == nil {
		return xerrors.Errorf("invalid private message")
	}
	dest := strings.TrimSpace(pkt.Header.Destination)
	// process only if destination is in recipients set
	if _, ok := pm.Recipients[dest]; !ok {
		return nil
	}
	if strings.HasPrefix(pm.Msg.Type, "paxos") || pm.Msg.Type == "paxospromise" {
		log.Printf("[DEBUG] private deliver node=%s dest=%s src=%s type=%s", n.conf.Socket.GetAddress(), dest, strings.TrimSpace(pkt.Header.Source), pm.Msg.Type)
	}
	// Process embedded message with same header
	_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: pkt.Header, Msg: pm.Msg})
	return nil
}

func (n *node) handleStatusMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	remote, ok := m.(types.StatusMessage)
	if !ok {
		p, ok2 := m.(*types.StatusMessage)
		if !ok2 || p == nil {
			return xerrors.Errorf("unexpected message type")
		}
		remote = *p
	}
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}
	self := n.conf.Socket.GetAddress()
	source := strings.TrimSpace(pkt.Header.Source)

	n.respondToStatus(source, self, remote)
	return nil
}

func (n *node) handleAckMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	ack, ok := m.(*types.AckMessage)
	if !ok || ack == nil {
		return xerrors.Errorf("unexpected message type")
	}
	// Stop waiting for this ack if it's pending
	n.pmu.Lock()
	p, ok := n.pendingAcks[pkt.Header.PacketID]
	if ok && p != nil && p.timer != nil {
		p.timer.Stop()
		delete(n.pendingAcks, pkt.Header.PacketID)
	}
	n.pmu.Unlock()

	// Process the embedded status using the registry
	wire, err := n.conf.MessageRegistry.MarshalMessage(ack.Status)
	if err == nil {
		// reuse header to process locally
		_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: pkt.Header, Msg: &wire})
	}
	return nil
}

func (n *node) handleSearchReply(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	rep, ok := m.(*types.SearchReplyMessage)
	if !ok || rep == nil {
		return xerrors.Errorf("unexpected message type")
	}
	// Update naming store and catalog
	nameStore := n.conf.Storage.GetNamingStore()
	for _, fi := range rep.Responses {
		if strings.TrimSpace(fi.Name) != "" && strings.TrimSpace(fi.Metahash) != "" {
			nameStore.Set(fi.Name, []byte(fi.Metahash))
			// metahash -> src
			n.UpdateCatalog(fi.Metahash, strings.TrimSpace(pkt.Header.Source))
			// chunks availability
			for _, c := range fi.Chunks {
				if len(c) > 0 {
					n.UpdateCatalog(string(c), strings.TrimSpace(pkt.Header.Source))
				}
			}
		}
	}
	// notify waiting search
	n.searchMu.Lock()
	ch := n.pendingSearch[rep.RequestID]
	n.searchMu.Unlock()
	if ch != nil {
		// Use a goroutine to avoid blocking the message handler
		go func() {
			select {
			case ch <- *rep:
			case <-time.After(time.Second):
				// Timeout to prevent goroutine leak
			}
		}()
	}
	return nil
}
