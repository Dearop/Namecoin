package impl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/rs/xid"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// Upload implements peer.DataSharing
func (n *node) Upload(data io.Reader) (string, error) {
	if err := n.validateNode(false); err != nil {
		return "", err
	}
	if data == nil {
		return "", xerrors.Errorf("nil reader")
	}
	blobStore := n.conf.Storage.GetDataBlobStore()
	chunkSize := n.conf.ChunkSize
	if chunkSize == 0 {
		chunkSize = 8192
	}

	// Limit: files up to 2 MiB
	const maxSize = 2 * 1024 * 1024
	var total int

	buf := make([]byte, chunkSize)
	chunkHashes := make([]string, 0, 16)

	for {
		nr, err := io.ReadFull(data, buf)
		if err == io.EOF && nr == 0 {
			break
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", xerrors.Errorf("read error: %v", err)
		}
		chunk := append([]byte(nil), buf[:nr]...)
		total += nr
		if total > maxSize {
			return "", xerrors.Errorf("data too large: exceeds 2 MiB")
		}
		sum := sha256.Sum256(chunk)
		hexKey := hex.EncodeToString(sum[:])
		blobStore.Set(hexKey, chunk)
		chunkHashes = append(chunkHashes, hexKey)
		if err == io.ErrUnexpectedEOF {
			// processed last partial chunk
			break
		}
	}

	if len(chunkHashes) == 0 {
		return "", xerrors.Errorf("empty data")
	}

	// Build metafile: hex hashes separated by peer.MetafileSep
	var mf bytes.Buffer
	for i, h := range chunkHashes {
		if i > 0 {
			mf.WriteString(peer.MetafileSep)
		}
		mf.WriteString(h)
	}
	metafile := mf.Bytes()

	// Ensure metafile fits in one chunk
	if len(metafile) > int(chunkSize) {
		return "", xerrors.Errorf("metafile too large for one chunk")
	}

	ms := sha256.Sum256(metafile)
	metahash := hex.EncodeToString(ms[:])

	// Store metafile under its metahash
	blobStore.Set(metahash, metafile)

	return metahash, nil
}

// Download implements peer.DataSharing
func (n *node) Download(metahash string) ([]byte, error) {
	if err := n.validateNode(false); err != nil {
		return nil, err
	}
	mh := strings.TrimSpace(metahash)
	if mh == "" {
		return nil, xerrors.Errorf("empty metahash")
	}
	blob := n.conf.Storage.GetDataBlobStore()

	// Step 1: get metafile value (list of chunk keys)
	metafile := blob.Get(mh)
	if metafile == nil {
		// fetch remotely using catalog
		val, err := n.fetchRemote(mh)
		if err != nil {
			return nil, err
		}
		metafile = val
		blob.Set(mh, metafile)
	}

	// Parse metafile into chunk keys
	lines := strings.Split(string(metafile), peer.MetafileSep)
	if len(lines) == 0 {
		return nil, xerrors.Errorf("invalid metafile")
	}

	// Step 2: sequentially fetch chunks
	var out bytes.Buffer
	for _, key := range lines {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, xerrors.Errorf("invalid chunk key in metafile")
		}
		data := blob.Get(key)
		if data == nil {
			val, err := n.fetchRemote(key)
			if err != nil {
				return nil, err
			}
			// verify integrity
			sum := sha256.Sum256(val)
			if hex.EncodeToString(sum[:]) != key {
				// wrong data: drop and update catalog
				n.removeFromCatalog(key)
				return nil, xerrors.Errorf("tampered chunk")
			}
			blob.Set(key, val)
			data = val
		}
		out.Write(data)
	}
	return out.Bytes(), nil
}

// fetchRemote fetches data from remote peers using the catalog
func (n *node) fetchRemote(key string) ([]byte, error) {
	// pick random peer from catalog for key, ensure routable
	dest, ok := n.pickPeerForKey(key)
	if !ok {
		return nil, xerrors.Errorf("no catalog entry for %s", key)
	}
	nextHop, ok := n.lookupNextHop(dest)
	if !ok {
		return nil, xerrors.Errorf("no route to %s", dest)
	}

	// create pending channel
	reqID := xid.New().String()
	ch := make(chan types.DataReplyMessage, 1)
	n.dataReqMu.Lock()
	if n.pendingData == nil {
		n.pendingData = make(map[string]chan types.DataReplyMessage)
	}
	n.pendingData[reqID] = ch
	n.dataReqMu.Unlock()
	defer func() {
		n.dataReqMu.Lock()
		delete(n.pendingData, reqID)
		n.dataReqMu.Unlock()
	}()

	// send request with backoff
	back := n.conf.BackoffDataRequest
	if back.Initial <= 0 {
		back.Initial = time.Second * 2
	}
	if back.Factor == 0 {
		back.Factor = 2
	}
	if back.Retry == 0 {
		back.Retry = 5
	}

	req := types.DataRequestMessage{RequestID: reqID, Key: key}
	wire, err := n.conf.MessageRegistry.MarshalMessage(req)
	if err != nil {
		return nil, err
	}
	header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), dest)

	wait := back.Initial
	for i := uint(0); i < back.Retry; i++ {
		_ = n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &wire}, time.Second)
		select {
		case rep := <-ch:
			if rep.Key != key || rep.RequestID != reqID {
				continue
			}
			if rep.Value == nil {
				// remove bogus catalog entry
				n.removeFromCatalog(key)
				return nil, xerrors.Errorf("empty reply")
			}
			return rep.Value, nil
		case <-time.After(wait):
			wait *= time.Duration(back.Factor)
		}
	}
	return nil, xerrors.Errorf("timeout fetching %s", key)
}

// pickPeerForKey selects a random peer from the catalog for the given key
func (n *node) pickPeerForKey(key string) (string, bool) {
	n.mu.RLock()
	peers, exists := n.catalog[key]
	if !exists || len(peers) == 0 {
		n.mu.RUnlock()
		return "", false
	}
	// Create a copy of the map to avoid race conditions during iteration
	peersCopy := make(map[string]struct{})
	for peer := range peers {
		peersCopy[peer] = struct{}{}
	}
	n.mu.RUnlock()

	// pick random peer from the copy
	for peer := range peersCopy {
		return peer, true
	}
	return "", false
}

// removeFromCatalog removes a key from the catalog
func (n *node) removeFromCatalog(key string) {
	n.mu.Lock()
	delete(n.catalog, key)
	n.mu.Unlock()
}

// handleDataRequest handles incoming data requests
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

	// Deduplicate requests: check if we've already processed this request ID
	if _, exists := n.processedDataReq.LoadOrStore(req.RequestID, true); exists {
		// Already processed this request, silently ignore
		return nil
	}

	// Check if we can route back to the source
	dest := strings.TrimSpace(pkt.Header.Source)
	nextHop, ok := n.lookupNextHop(dest)
	if !ok {
		// Can't route back to source, silently drop
		return nil
	}

	// get data from local storage
	blobStore := n.conf.Storage.GetDataBlobStore()
	data := blobStore.Get(req.Key)

	// send reply
	reply := types.DataReplyMessage{
		RequestID: req.RequestID,
		Key:       req.Key,
		Value:     data,
	}
	wire, err := n.conf.MessageRegistry.MarshalMessage(reply)
	if err != nil {
		return err
	}
	header := transport.NewHeader(n.conf.Socket.GetAddress(), n.conf.Socket.GetAddress(), dest)
	return n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &wire}, time.Second)
}

// handleDataReply handles incoming data replies
func (n *node) handleDataReply(m types.Message, pkt transport.Packet) error {
	if err := n.validateNode(false); err != nil {
		return err
	}
	rep, ok := m.(*types.DataReplyMessage)
	if !ok || rep == nil {
		return xerrors.Errorf("unexpected message type")
	}
	// notify waiting request
	n.dataReqMu.Lock()
	ch := n.pendingData[rep.RequestID]
	n.dataReqMu.Unlock()
	if ch != nil {
		// Non-blocking send to drop duplicates (channel has buffer of 1)
		select {
		case ch <- *rep:
		default:
		}
	}
	return nil
}
