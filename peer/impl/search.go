package impl

import (
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// listNeighbors returns direct neighbors (origin==relay and not self)
func (n *node) listNeighbors() []string {
	if err := n.validateNode(false); err != nil {
		return []string{}
	}

	nodeAddr := n.conf.Socket.GetAddress()
	n.mu.RLock()
	defer n.mu.RUnlock()
	res := make([]string, 0)
	for origin, relay := range n.routingTable {
		if origin != "" && relay != "" && origin == relay && origin != nodeAddr {
			res = append(res, origin)
		}
	}
	return res
}

type budgetItem struct {
	dest string
	b    int
}

func planBudget(total int, neighbors []string) []budgetItem {
	if total <= 0 || len(neighbors) == 0 {
		return nil
	}
	if neighbors == nil {
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
	res := make([]budgetItem, 0, k)
	for i := 0; i < k; i++ {
		if i >= len(neighbors) {
			break
		}
		b := base
		if rem > 0 {
			b++
			rem--
		}
		if b <= 0 {
			continue
		}
		res = append(res, budgetItem{dest: neighbors[i], b: b})
	}
	return res
}

func (n *node) createSearchWait(k int) (string, chan types.SearchReplyMessage) {
	if k <= 0 {
		k = 1 // minimum buffer size
	}
	if k > 1000 {
		k = 1000 // maximum buffer size to prevent memory issues
	}

	reqID := xid.New().String()
	n.searchMu.Lock()
	if n.pendingSearch == nil {
		n.pendingSearch = make(map[string]chan types.SearchReplyMessage)
	}
	ch := make(chan types.SearchReplyMessage, k)
	n.pendingSearch[reqID] = ch
	n.searchMu.Unlock()
	return reqID, ch
}

func (n *node) clearSearchWait(reqID string) {
	if reqID == "" {
		return
	}

	n.searchMu.Lock()
	if n.pendingSearch != nil {
		delete(n.pendingSearch, reqID)
	}
	n.searchMu.Unlock()
}

func (n *node) sendSearchRequests(reqID, pattern string, plan []budgetItem) {
	if reqID == "" || pattern == "" || plan == nil {
		return
	}

	if err := n.validateNode(false); err != nil {
		return
	}

	nodeAddr := n.conf.Socket.GetAddress()
	for _, item := range plan {
		if item.dest == "" || item.b <= 0 {
			continue
		}

		if nextHop, ok := n.lookupNextHop(item.dest); ok {
			req := types.SearchRequestMessage{RequestID: reqID, Origin: nodeAddr, Pattern: pattern, Budget: uint(item.b)}
			if wire, err := n.conf.MessageRegistry.MarshalMessage(req); err == nil {
				header := transport.NewHeader(nodeAddr, nodeAddr, item.dest)
				_ = n.conf.Socket.Send(nextHop, transport.Packet{Header: &header, Msg: &wire}, time.Second)
			}
		}
	}
}

func (n *node) collectLocalNames(reg regexp.Regexp) map[string]struct{} {
	if err := n.validateNode(false); err != nil {
		return make(map[string]struct{})
	}

	nameStore := n.conf.Storage.GetNamingStore()
	blobStore := n.conf.Storage.GetDataBlobStore()
	local := make(map[string]struct{})

	if nameStore == nil || blobStore == nil {
		return local
	}

	nameStore.ForEach(func(name string, val []byte) bool {
		if name == "" || len(val) == 0 {
			return true
		}
		if blobStore.Get(string(val)) == nil {
			return true
		}
		if reg.MatchString(name) {
			local[name] = struct{}{}
		}
		return true
	})
	return local
}

func (n *node) waitCollectSearch(ch chan types.SearchReplyMessage, timeout time.Duration,
	reg regexp.Regexp) map[string]struct{} {
	if ch == nil || timeout <= 0 {
		return make(map[string]struct{})
	}

	names := make(map[string]struct{})
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case rep := <-ch:
			if rep.Responses == nil {
				continue
			}
			for _, fi := range rep.Responses {
				name := strings.TrimSpace(fi.Name)
				if name != "" && reg.MatchString(name) {
					names[name] = struct{}{}
				}
			}
		case <-timer.C:
			return names
		}
	}
}

// SearchAll implements peer.DataSharing
func (n *node) SearchAll(reg regexp.Regexp, budget uint, timeout time.Duration) ([]string, error) {
	if err := n.validateNode(false); err != nil {
		return nil, err
	}
	local := n.collectLocalNames(reg)

	// Send to neighbors according to budget
	if budget > 0 {
		neighbors := n.listNeighbors()

		if len(neighbors) > 0 {
			// shuffle neighbors
			rand.Shuffle(len(neighbors), func(i, j int) { neighbors[i], neighbors[j] = neighbors[j], neighbors[i] })

			// split budget as evenly as possible across up to len(neighbors)
			k := len(neighbors)
			use := int(budget)
			if use < k {
				k = use
			}
			reqID, ch := n.createSearchWait(k)
			plan := planBudget(use, neighbors)
			n.sendSearchRequests(reqID, reg.String(), plan)
			remotes := n.waitCollectSearch(ch, timeout, reg)
			n.clearSearchWait(reqID)
			for name := range remotes {
				local[name] = struct{}{}
			}
		}
	}

	// build sorted list
	res := make([]string, 0, len(local))
	for name := range local {
		res = append(res, name)
	}
	sort.Strings(res)
	return res, nil
}

// checkLocalFullMatch checks if we have a local full match for the pattern
func (n *node) checkLocalFullMatch(pattern regexp.Regexp) string {
	if err := n.validateNode(false); err != nil {
		return ""
	}

	nameStore := n.conf.Storage.GetNamingStore()
	blobStore := n.conf.Storage.GetDataBlobStore()

	if nameStore == nil || blobStore == nil {
		return ""
	}

	found := ""
	nameStore.ForEach(func(name string, val []byte) bool {
		if found != "" { // early stop
			return false
		}
		if name == "" || len(val) == 0 || !pattern.MatchString(name) {
			return true
		}
		mh := string(val)
		meta := blobStore.Get(mh)
		if meta == nil {
			return true
		}
		chunkKeys := strings.Split(string(meta), "\n")
		for _, ck := range chunkKeys {
			ck = strings.TrimSpace(ck)
			if ck == "" || blobStore.Get(ck) == nil {
				return true // missing chunk
			}
		}
		found = name
		return false
	})
	return found
}

// normalizeExpandingRingConfig sets defaults for expanding ring search
func normalizeExpandingRingConfig(conf peer.ExpandingRing) peer.ExpandingRing {
	if conf.Initial == 0 {
		conf.Initial = 1
	}
	if conf.Factor == 0 {
		conf.Factor = 2
	}
	if conf.Retry == 0 {
		conf.Retry = 1
	}
	if conf.Timeout <= 0 {
		conf.Timeout = time.Second
	}
	if conf.Initial > 1000 {
		conf.Initial = 1000
	}
	if conf.Factor > 10 {
		conf.Factor = 10
	}
	if conf.Retry > 10 {
		conf.Retry = 10
	}
	if conf.Timeout > 30*time.Second {
		conf.Timeout = 30 * time.Second
	}

	return conf
}

// isFullMatch checks if a FileInfo represents a full match (all chunks present)
func isFullMatch(fi types.FileInfo) bool {
	if len(fi.Chunks) == 0 {
		return false
	}
	if fi.Chunks == nil {
		return false
	}

	for _, c := range fi.Chunks {
		if len(c) == 0 {
			return false
		}
	}
	return true
}

// processSearchReply processes a search reply and returns first full match
func processSearchReply(rep types.SearchReplyMessage, pattern regexp.Regexp) string {
	if rep.Responses == nil {
		return ""
	}

	for _, fi := range rep.Responses {
		if fi.Name == "" {
			continue
		}
		if !pattern.MatchString(fi.Name) {
			continue
		}
		if isFullMatch(fi) {
			return fi.Name
		}
	}
	return ""
}

// waitForFullMatch waits for search replies and returns first full match
func (n *node) waitForFullMatch(ch chan types.SearchReplyMessage, timeout time.Duration,
	pattern regexp.Regexp) string {
	if ch == nil || timeout <= 0 {
		return ""
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case rep := <-ch:
			if result := processSearchReply(rep, pattern); result != "" {
				return result
			}
		case <-timer.C:
			return ""
		}
	}
}

// SearchFirst implements peer.DataSharing
func (n *node) SearchFirst(pattern regexp.Regexp, conf peer.ExpandingRing) (string, error) {
	if err := n.validateNode(false); err != nil {
		return "", err
	}

	// 1) Local full match first
	if found := n.checkLocalFullMatch(pattern); found != "" {
		return found, nil
	}

	// 2) Expanding-ring search
	conf = normalizeExpandingRingConfig(conf)
	neighbors := n.listNeighbors()
	if len(neighbors) == 0 {
		return "", nil
	}

	budget := conf.Initial
	retries := conf.Retry
	for retries > 0 {
		rand.Shuffle(len(neighbors), func(i, j int) {
			neighbors[i], neighbors[j] = neighbors[j], neighbors[i]
		})
		k := len(neighbors)
		use := int(budget)
		if use < k {
			k = use
		}
		reqID, ch := n.createSearchWait(k)
		plan := planBudget(use, neighbors)
		n.sendSearchRequests(reqID, pattern.String(), plan)

		result := n.waitForFullMatch(ch, conf.Timeout, pattern)
		n.clearSearchWait(reqID)
		if result != "" {
			return result, nil
		}

		// next ring
		retries--
		budget *= conf.Factor
	}

	return "", nil
}

// splitBudget computes per-neighbor budgets, distributing evenly with remainder.
func splitBudget(total int, neighbors []string) []struct {
	dest string
	b    int
} {
	if total <= 0 || len(neighbors) == 0 {
		return nil
	}
	if neighbors == nil {
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
		if i >= len(neighbors) {
			break
		}
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
	if pattern == "" {
		return []types.FileInfo{}
	}

	if err := n.validateNode(false); err != nil {
		return []types.FileInfo{}
	}

	nameStore := n.conf.Storage.GetNamingStore()
	blobStore := n.conf.Storage.GetDataBlobStore()
	if nameStore == nil || blobStore == nil {
		return []types.FileInfo{}
	}

	responses := make([]types.FileInfo, 0)

	// compile regex pattern
	reg, err := regexp.Compile(pattern)
	if err != nil {
		return responses
	}

	nameStore.ForEach(func(name string, val []byte) bool {
		if name == "" || len(val) == 0 {
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
	if req == nil || pkt.Header == nil {
		return
	}

	if req.Budget <= 1 {
		return
	}
	if err := n.validateNode(false); err != nil {
		return
	}

	remaining := int(req.Budget) - 1
	nodeAddr := n.conf.Socket.GetAddress()
	from := strings.TrimSpace(pkt.Header.Source)
	n.mu.RLock()
	neighbors := make([]string, 0)
	for origin, relay := range n.routingTable {
		if origin != "" && relay != "" && origin == relay && origin != nodeAddr && origin != from {
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
		if item.dest == "" || item.b <= 0 {
			continue
		}

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
	if req == nil || pkt.Header == nil {
		return xerrors.Errorf("invalid request or packet")
	}
	if err := n.validateNode(false); err != nil {
		return err
	}

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

// handleSearchRequest handles incoming search requests
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
