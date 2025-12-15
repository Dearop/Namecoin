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

// handleChatMessage processes incoming chat messages and logs them.
// It validates the message type and packet before processing.
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

// handleRumorsMessage processes incoming rumors messages and handles gossip propagation.
// It accepts expected rumors, sends an ack back, and forwards newly accepted rumors once.
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

// handleNamecoinTransactionMessage processes transaction message broadcasted by peers
func (n *node) handleNamecoinTransactionMessage(message types.Message, packet transport.Packet) error {
	msg, ok := message.(*types.NamecoinTransactionMessage)
	if !ok {
		return xerrors.Errorf("unexpected message type")
	}

	// Validate signed transaction and derive UTXO inputs/outputs before accepting into mempool.
	signed := SignedTransaction{
		Type:      msg.Type,
		From:      msg.From,
		Amount:    msg.Amount,
		Payload:   msg.Payload,
		Pk:        msg.Pk,
		TxID:      msg.TxID,
		Signature: msg.Signature,
	}
	if err := n.transactionService.ValidateTransaction(&signed); err != nil {
		return err
	}

	inputs, outputs, err := n.transactionService.VerifyBalance(signed.TxID, signed.From, signed.Amount)
	if err != nil {
		return err
	}

	tx := types.Tx{
		From:      signed.From,
		Type:      signed.Type,
		Inputs:    inputs,
		Outputs:   outputs,
		Amount:    signed.Amount,
		Payload:   signed.Payload,
		Pk:        signed.Pk,
		TxID:      signed.TxID,
		Signature: signed.Signature,
	}
	if err := n.transactionService.ValidateTxCommand(&tx); err != nil {
		return err
	}

	// Ensure the TxID provided over the network matches the one computed by the backend.
	txID, err := BuildTransactionID(&tx)
	if err != nil {
		return err
	}
	if txID != signed.TxID {
		return xerrors.Errorf("txId mismatch: expected %s, got %s", txID, signed.TxID)
	}

	n.namecoinConsensus.txBuffer.Add(tx, txID)
	return nil
}

// handleNamecoinBlockMessage processes block message broadcasted by peers
func (n *node) handleNamecoinBlockMessage(message types.Message, packet transport.Packet) error {
	msg, ok := message.(*types.NamecoinBlockMessage)
	if !ok {
		return xerrors.Errorf("unexpected message type")
	}

	log.Printf("[DEBUG] Received block at height %d with %d transactions",
		msg.Block.Header.Height, len(msg.Block.Transactions))

	changed, err := n.NamecoinChainService.AppendBlockToLongestChain(&msg.Block)
	if err != nil {
		log.Printf("[ERROR] Failed to append block at height %d: %v", msg.Block.Header.Height, err)
		// Notify all transactions in this block that they failed
		log.Printf("[DEBUG] Notifying %d transactions in FAILED received block", len(msg.Block.Transactions))
		for _, val := range msg.Block.Transactions {
			txID, txErr := BuildTransactionID(&val)
			if txErr == nil {
				n.notifyTxConfirmation(txID, err)
			}
		}
		return err
	}

	if !changed {
		// The longest chain has not been changed, no need to stop miner because some branch side branch popped up
		// this was causing a miner rerun and decreasing overall miner efficiency, sometimes producing blocks out of order
		return nil
	}

	// Remove tx from txBuffer
	for _, val := range msg.Block.Transactions {
		txID, err := BuildTransactionID(&val)
		if err != nil {
			return err
		}
		n.namecoinConsensus.txBuffer.Remove(txID)
	}

	n.StopMiner()
	n.minerMu.Lock()
	disabled := n.minerDisabled
	n.minerMu.Unlock()
	if !disabled {
		n.StartMiner()
	}

	log.Printf("[DEBUG] Successfully applied received block at height %d, notifying %d transactions",
		msg.Block.Header.Height, len(msg.Block.Transactions))
	// Notify all transactions in this block that they succeeded
	for _, val := range msg.Block.Transactions {
		txID, txErr := BuildTransactionID(&val)
		if txErr == nil {
			n.notifyTxConfirmation(txID, nil)
		}
	}

	return nil
}

// handlePrivateMessage processes private messages intended for this node.
// It checks if the destination is in the recipients set and processes the embedded message.
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
		if os.Getenv("GLOG") != "no" {
			nodeAddr := n.conf.Socket.GetAddress()
			src := strings.TrimSpace(pkt.Header.Source)
			log.Printf("[DEBUG] private deliver node=%s dest=%s src=%s type=%s", nodeAddr, dest, src, pm.Msg.Type)
		}
	}
	// Process embedded message with same header
	_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: pkt.Header, Msg: pm.Msg})
	return nil
}

// handleStatusMessage processes incoming status messages for anti-entropy.
// It compares local and remote status and responds accordingly.
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

// handleAckMessage processes acknowledgment messages for rumor delivery.
// It stops waiting for the ack and processes the embedded status message.
func (n *node) handleAckMessage(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	if pkt.Header == nil {
		// Ignore malformed packets without a header to avoid panics under fuzzing.
		return nil
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
	if err == nil && pkt.Header != nil {
		// reuse header to process locally
		_ = n.conf.MessageRegistry.ProcessPacket(transport.Packet{Header: pkt.Header, Msg: &wire})
	}
	return nil
}

// handleSearchReply processes search reply messages from other peers.
// It updates the naming store and catalog, then notifies waiting search operations.
func (n *node) handleSearchReply(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	if pkt.Header == nil {
		// Ignore malformed packets without a header to avoid panics under fuzzing.
		return nil
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
