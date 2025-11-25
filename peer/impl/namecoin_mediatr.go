package impl

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type NameCoinCommand interface {
	Name() string
}
type NameNew struct {
	Commitment string `json:"commitment"` // H(salt + domain)
}

type NameFirstUpdate struct {
	Domain string `json:"domain"` // The real domain name being registered
	Salt   string `json:"salt"`   // Must match the original commitment
	IP     string `json:"ip"`     // IP address the user wants to bind
}

type NameUpdate struct {
	Domain string `json:"domain"` // Registered domain
	IP     string `json:"ip"`     // New IP address
}

type CommandType interface {
	NameNew | NameFirstUpdate | NameUpdate
}

func ResolveNameCoinCommand[T CommandType](command string, payload json.RawMessage) (T, error) {
	switch command {
	case NameNew{}.Name():
		var nameNew NameNew
		if err := json.Unmarshal(payload, &nameNew); err != nil {
			return any(nameNew).(T), fmt.Errorf("invalid NameNew payload: %w", err)
		}

		return any(nameNew).(T), nil
	case NameFirstUpdate{}.Name():
		var firstUpdate NameFirstUpdate
		if err := json.Unmarshal(payload, &firstUpdate); err != nil {
			return any(firstUpdate).(T), fmt.Errorf("invalid NameFirstUpdate payload: %w", err)
		}

		return any(firstUpdate).(T), nil
	case NameUpdate{}.Name():
		var updateName NameUpdate
		if err := json.Unmarshal(payload, &updateName); err != nil {
			return any(updateName).(T), fmt.Errorf("invalid NameUpdate payload: %w", err)
		}

		return any(updateName).(T), nil
	default:
		var zero T
		return zero, fmt.Errorf("unknown command: %s", command)
	}
}

// Name implements NameCoinCommand
func (n NameNew) Name() string {
	return reflect.TypeOf(&NameNew{}).Elem().Name()
}

// Name implements NameCoinCommand
func (n NameFirstUpdate) Name() string {
	return reflect.TypeOf(&NameFirstUpdate{}).Elem().Name()
}

// Name implements NameCoinCommand
func (n NameUpdate) Name() string {
	return reflect.TypeOf(&NameUpdate{}).Elem().Name()
}
