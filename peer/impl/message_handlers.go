package impl

import (
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"go.dedis.ch/cs438/peer"
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

func (n *node) handleDataRequest(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	req, ok := m.(*types.DataRequestMessage)
	if !ok || req == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}

	// Check for duplicate requests using RequestID
	reqKey := req.RequestID + ":" + req.Key
	n.dataReqMu.Lock()
	if n.pendingData == nil {
		n.pendingData = make(map[string]chan types.DataReplyMessage)
	}
	if _, exists := n.pendingData[reqKey]; exists {
		// Duplicate request, ignore
		n.dataReqMu.Unlock()
		return nil
	}
	// Mark as processed
	n.pendingData[reqKey] = nil
	n.dataReqMu.Unlock()

	key := strings.TrimSpace(req.Key)
	store := n.conf.Storage.GetDataBlobStore()
	val := store.Get(key)
	reply := types.DataReplyMessage{RequestID: req.RequestID, Key: req.Key, Value: val}
	wire, err := n.conf.MessageRegistry.MarshalMessage(reply)
	if err != nil {
		return err
	}
	src := strings.TrimSpace(pkt.Header.Source)
	if src == "" {
		return nil
	}
	// route reply using routing table
	nextHop, ok := n.lookupNextHop(src)
	if !ok {
		return nil
	}
	header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), src)
	return n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

func (n *node) handleDataReply(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	rep, ok := m.(*types.DataReplyMessage)
	if !ok || rep == nil {
		return xerrors.Errorf("unexpected message type")
	}
	n.dataReqMu.Lock()
	ch := n.pendingData[rep.RequestID]
	n.dataReqMu.Unlock()
	if ch != nil {
		select {
		case ch <- *rep:
		default:
		}
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
		select {
		case ch <- *rep:
		default:
		}
	}
	return nil
}

// splitBudget computes per-neighbor budgets, distributing evenly with remainder.
func splitBudget(total int, neighbors []string) []struct {
	dest string
	b    int
} {
	if total <= 0 || len(neighbors) == 0 {
		return nil
	}
	k := len(neighbors)
	if total < k {
		k = total
	}
	base, rem := 0, 0
	if k > 0 {
		base = total / k
		rem = total % k
	}
	res := make([]struct {
		dest string
		b    int
	}, 0, k)
	for i := 0; i < k; i++ {
		b := base
		if rem > 0 {
			b++
			rem--
		}
		if b <= 0 {
			continue
		}
		res = append(res, struct {
			dest string
			b    int
		}{dest: neighbors[i], b: b})
	}
	return res
}

// buildLocalFileInfos builds file infos for names matching pattern and having metafile.
func (n *node) buildLocalFileInfos(pattern string) []types.FileInfo {
	nameStore := n.conf.Storage.GetNamingStore()
	blobStore := n.conf.Storage.GetDataBlobStore()
	responses := make([]types.FileInfo, 0)

	// compile regex pattern
	reg, err := regexp.Compile(pattern)
	if err != nil {
		return responses
	}

	nameStore.ForEach(func(name string, val []byte) bool {
		if len(val) == 0 {
			return true
		}
		if !reg.MatchString(name) {
			return true
		}
		mh := string(val)
		meta := blobStore.Get(mh)
		if meta == nil {
			return true
		}
		chunkKeys := strings.Split(string(meta), peer.MetafileSep)
		fi := types.FileInfo{Name: name, Metahash: mh}
		fi.Chunks = make([][]byte, len(chunkKeys))
		for i, ck := range chunkKeys {
			ck = strings.TrimSpace(ck)
			if ck == "" {
				continue
			}
			if blobStore.Get(ck) != nil {
				fi.Chunks[i] = []byte(ck)
			}
		}
		responses = append(responses, fi)
		return true
	})
	return responses
}

// forwardSearchRequest forwards search request to neighbors if budget permits
func (n *node) forwardSearchRequest(req *types.SearchRequestMessage, pkt transport.Packet) {
	if req.Budget <= 1 {
		return
	}
	remaining := int(req.Budget) - 1
	nodeAddr := n.conf.Socket.GetAddress()
	from := strings.TrimSpace(pkt.Header.Source)
	n.mu.RLock()
	neighbors := make([]string, 0)
	for origin, relay := range n.routingTable {
		if origin == relay && origin != nodeAddr && origin != from {
			neighbors = append(neighbors, origin)
		}
	}
	n.mu.RUnlock()
	if len(neighbors) == 0 {
		return
	}
	rand.Shuffle(len(neighbors), func(i, j int) {
		neighbors[i], neighbors[j] = neighbors[j], neighbors[i]
	})
	plan := splitBudget(remaining, neighbors)
	for _, item := range plan {
		if nextHop, ok := n.lookupNextHop(item.dest); ok {
			fwd := types.SearchRequestMessage{
				RequestID: req.RequestID,
				Origin:    req.Origin,
				Pattern:   req.Pattern,
				Budget:    uint(item.b),
			}
			wire, err := n.conf.MessageRegistry.MarshalMessage(fwd)
			if err == nil {
				header := transport.NewHeader(nodeAddr, nodeAddr, item.dest)
				_ = n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &wire}, time.Second)
			}
		}
	}
}

// sendSearchReply sends search reply back to the original requester
func (n *node) sendSearchReply(req *types.SearchRequestMessage, responses []types.FileInfo,
	pkt transport.Packet) error {
	reply := types.SearchReplyMessage{RequestID: req.RequestID, Responses: responses}
	wire, err := n.conf.MessageRegistry.MarshalMessage(reply)
	if err != nil {
		return err
	}
	src := strings.TrimSpace(pkt.Header.Source)
	if src == "" {
		return nil
	}
	header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), req.Origin)
	return n.conf.Socket.Send(src, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

func (n *node) handleSearchRequest(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	req, ok := m.(*types.SearchRequestMessage)
	if !ok || req == nil {
		return xerrors.Errorf("unexpected message type")
	}
	if pkt.Header == nil {
		return xerrors.Errorf("missing header")
	}

	// 1) Forward if budget permits
	n.forwardSearchRequest(req, pkt)

	// 2) Check local naming store and build FileInfo list
	pattern := strings.TrimSpace(req.Pattern)
	responses := n.buildLocalFileInfos(pattern)

	// 3) Reply directly to packet's source
	return n.sendSearchReply(req, responses, pkt)
}
