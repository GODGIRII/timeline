// Package spaces defines persistent records and timeline write rules.
package spaces

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/GODGIRII/sequence/internal/auth"
)

type Account struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	PasswordHash string `json:"password_hash"`
}

type Session struct {
	AccountID string    `json:"account_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Event struct {
	ID        string          `json:"id"`
	SpaceID   string          `json:"space_id"`
	ActorID   string          `json:"actor_id"`
	Revision  uint64          `json:"revision"`
	Document  json.RawMessage `json:"document"`
	CreatedAt time.Time       `json:"created_at"`
}

type Operation struct {
	Fingerprint string `json:"fingerprint"`
	Event       Event  `json:"event"`
}

type Space struct {
	ID         string               `json:"id"`
	Key        string               `json:"key"`
	Name       string               `json:"name"`
	CreatedAt  time.Time            `json:"created_at"`
	Members    map[string]string    `json:"members"`
	Requests   map[string]string    `json:"requests"`
	Revision   uint64               `json:"revision"`
	Document   json.RawMessage      `json:"document"`
	Events     []Event              `json:"events"`
	Operations map[string]Operation `json:"operations"`
}

type State struct {
	Version  int                `json:"version"`
	Accounts map[string]Account `json:"accounts"`
	Sessions map[string]Session `json:"sessions"`
	Spaces   map[string]*Space  `json:"spaces"`
}

func NewState() *State {
	return &State{Version: 1, Accounts: map[string]Account{}, Sessions: map[string]Session{}, Spaces: map[string]*Space{}}
}

var ErrConflict = errors.New("revision conflict")
var ErrOperation = errors.New("operation ID was already used with different input")

// Apply deduplicates by actor and operation ID before checking the base revision.
// The caller must persist the complete transaction before returning the event.
func (s *Space) Apply(actor, operationID string, base uint64, document json.RawMessage) (Event, error) {
	input, _ := json.Marshal(struct {
		Base     uint64
		Document json.RawMessage
	}{base, document})
	fingerprint := auth.Digest(string(input))
	key := actor + ":" + operationID
	if old, ok := s.Operations[key]; ok {
		if old.Fingerprint != fingerprint {
			return Event{}, ErrOperation
		}
		return old.Event, nil
	}
	if base != s.Revision {
		return Event{}, ErrConflict
	}
	s.Revision++
	s.Document = append(json.RawMessage(nil), document...)
	event := Event{ID: auth.Token(), SpaceID: s.ID, ActorID: actor, Revision: s.Revision, Document: s.Document, CreatedAt: time.Now().UTC()}
	s.Events = append(s.Events, event)
	s.Operations[key] = Operation{Fingerprint: fingerprint, Event: event}
	return event, nil
}
