package types

import (
	"fmt"
)

// -----------------------------------------------------------------------------
// ChatMessage

// NewEmpty implements types.Message.
func (c ChatMessage) NewEmpty() Message {
	return &ChatMessage{}
}

// Name implements types.Message.
func (ChatMessage) Name() string {
	return "chat"
}

// String implements types.Message.
func (c ChatMessage) String() string {
	return fmt.Sprintf("<%s>", c.Message)
}

// HTML implements types.Message.
func (c ChatMessage) HTML() string {
	return c.String()
}

// -----------------------------------------------------------------------------
// EmptyMessage

// NewEmpty implements types.Message.
func (e EmptyMessage) NewEmpty() Message {
	return &EmptyMessage{}
}

// Name implements types.Message.
func (EmptyMessage) Name() string {
	return "empty"
}

// String implements types.Message.
func (EmptyMessage) String() string {
	return "<empty>"
}

// HTML implements types.Message.
func (e EmptyMessage) HTML() string {
	return e.String()
}

// -----------------------------------------------------------------------------
// RumorsMessage

// NewEmpty implements types.Message.
func (r RumorsMessage) NewEmpty() Message {
	return &RumorsMessage{}
}

// Name implements types.Message.
func (RumorsMessage) Name() string {
	return "rumors"
}

// String implements types.Message.
func (r RumorsMessage) String() string {
	return fmt.Sprintf("<rumors:%d>", len(r.Rumors))
}

// HTML implements types.Message.
func (r RumorsMessage) HTML() string {
	return r.String()
}

// -----------------------------------------------------------------------------
// StatusMessage

// NewEmpty implements types.Message.
func (s StatusMessage) NewEmpty() Message {
	return &StatusMessage{}
}

// Name implements types.Message.
func (StatusMessage) Name() string {
	return "status"
}

// String implements types.Message.
func (s StatusMessage) String() string {
	return fmt.Sprintf("<status:%d>", len(s.View))
}

// HTML implements types.Message.
func (s StatusMessage) HTML() string {
	return s.String()
}

// -----------------------------------------------------------------------------
// AcknowledgmentMessage

// NewEmpty implements types.Message.
func (a AcknowledgmentMessage) NewEmpty() Message {
	return &AcknowledgmentMessage{}
}

// Name implements types.Message.
func (AcknowledgmentMessage) Name() string {
	return "ack"
}

// String implements types.Message.
func (a AcknowledgmentMessage) String() string {
	return fmt.Sprintf("<ack:%s>", a.AckedPacketID)
}

// HTML implements types.Message.
func (a AcknowledgmentMessage) HTML() string {
	return a.String()
}

// -----------------------------------------------------------------------------
// PrivateMessage

// NewEmpty implements types.Message.
func (p PrivateMessage) NewEmpty() Message {
	return &PrivateMessage{}
}

// Name implements types.Message.
func (PrivateMessage) Name() string {
	return "private"
}

// String implements types.Message.
func (p PrivateMessage) String() string {
	return fmt.Sprintf("<private:%d>", len(p.Recipients))
}

// HTML implements types.Message.
func (p PrivateMessage) HTML() string {
	return p.String()
}
