package types

import "go.dedis.ch/cs438/transport"

// ChatMessage is a message sent to exchange text messages between nodes.
//
// - implements types.Message
// - implemented in HW0
type ChatMessage struct {
	Message string
}

type RumorsMessage struct {
	Rumors []transport.Rumor
}

// StatusEntry captures, for one origin, the last processed rumor sequence.
type StatusEntry struct {
	Origin   string
	Sequence uint64
}

// StatusMessage contains a peer view: last sequence seen per origin.
// Use either a compact map or a list; we choose a list for stable ordering.
type StatusMessage struct {
	View []StatusEntry
}

// AckMessage acknowledges a RumorsMessage packet and piggybacks a status view.
type AcknowledgmentMessage struct {
	AckedPacketID string
	Status        StatusMessage
}

// EmptyMessage carries no payload ==> used for liveness announcements.
type EmptyMessage struct{}

// PrivateMessage wraps an embedded message and a set of intended recipients.
// It is propagated to everyone, but only listed recipients should process Msg.
// Non encrypted for current implementation.
type PrivateMessage struct {
	Recipients []string
	Msg        transport.Message
}
