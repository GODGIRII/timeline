// Package timeline contains task/event rules independent of HTTP and storage.
package timeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GODGIRII/sequence/internal/auth"
)

var (
	ErrNotFound  = errors.New("item unavailable")
	ErrConflict  = errors.New("item version conflict")
	ErrOperation = errors.New("operation ID already used with different input")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func invalid(message string) error       { return &ValidationError{message} }

// Deadline is either a calendar date or an instant with an explicit UTC offset.
// Date-only values deliberately have no implicit midnight or timezone.
type Deadline struct {
	Date string `json:"date,omitempty"`
	At   string `json:"at,omitempty"`
}

func (d Deadline) Validate() error {
	if (d.Date == "") == (d.At == "") {
		return invalid("deadline requires exactly one of date or at")
	}
	if d.Date != "" {
		if _, err := time.Parse("2006-01-02", d.Date); err != nil {
			return invalid("deadline.date must be a valid YYYY-MM-DD date")
		}
	} else if _, err := time.Parse(time.RFC3339Nano, d.At); err != nil {
		return invalid("deadline.at must be RFC3339 with a timezone offset")
	}
	return nil
}

type Fields struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Deadline    Deadline `json:"deadline"`
	Priority    string   `json:"priority"`
	Status      string   `json:"status"`
}

func (f Fields) Validate() error {
	if f.Type != "task" && f.Type != "event" {
		return invalid("type must be task or event")
	}
	if strings.TrimSpace(f.Title) == "" || len(f.Title) > 200 {
		return invalid("title must be 1–200 bytes and contain non-whitespace text")
	}
	if len(f.Description) > 5000 {
		return invalid("description must be at most 5000 bytes")
	}
	if f.Priority != "low" && f.Priority != "medium" && f.Priority != "high" {
		return invalid("priority must be low, medium or high")
	}
	if f.Status != "open" && f.Status != "done" {
		return invalid("status must be open or done")
	}
	return f.Deadline.Validate()
}

type Item struct {
	ID      string `json:"id"`
	SpaceID string `json:"space_id"`
	Fields
	Version   uint64    `json:"version"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Deleted   bool      `json:"deleted"`
}

type Actor struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type Activity struct {
	ID            string    `json:"id"`
	SpaceID       string    `json:"space_id"`
	Sequence      uint64    `json:"sequence"`
	Kind          string    `json:"kind"`
	Actor         Actor     `json:"actor"`
	Item          Item      `json:"item"`
	Previous      *Item     `json:"previous,omitempty"`
	ChangedFields []string  `json:"changed_fields"`
	Message       string    `json:"message"`
	CreatedAt     time.Time `json:"created_at"`
}

type Command struct {
	Action      string  `json:"action"`
	ItemID      string  `json:"item_id"`
	OperationID string  `json:"operation_id"`
	BaseVersion uint64  `json:"base_version"`
	Fields      *Fields `json:"fields,omitempty"`
}

type Operation struct {
	Fingerprint string   `json:"fingerprint"`
	Activity    Activity `json:"activity"`
}

// Repository is scoped to one space and one database transaction. Commit must
// save the item, activity, sequence and retry result atomically.
type Repository interface {
	Operation(actor, operation string) (Operation, bool, error)
	Item(id string) (Item, error)
	Commit(item Item, activity Activity, operationID, fingerprint string) (Activity, error)
}

func Apply(repo Repository, spaceID string, actor Actor, cmd Command) (Activity, error) {
	if len(cmd.OperationID) == 0 || len(cmd.OperationID) > 128 {
		return Activity{}, invalid("operation_id must be 1–128 bytes")
	}
	switch cmd.Action {
	case "create":
		if cmd.ItemID != "" || cmd.BaseVersion != 0 {
			return Activity{}, invalid("create does not accept an item ID or base version")
		}
	case "update", "delete":
		if cmd.ItemID == "" || cmd.BaseVersion == 0 {
			return Activity{}, invalid("base_version must be positive")
		}
	default:
		return Activity{}, invalid("unknown action")
	}
	if cmd.Action != "delete" {
		if cmd.Fields == nil {
			return Activity{}, invalid("item fields required")
		}
		if err := cmd.Fields.Validate(); err != nil {
			return Activity{}, err
		}
	} else if cmd.Fields != nil {
		return Activity{}, invalid("delete does not accept item fields")
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return Activity{}, err
	}
	fingerprint := auth.Digest(string(raw))
	old, found, err := repo.Operation(actor.ID, cmd.OperationID)
	if err != nil {
		return Activity{}, err
	}
	if found {
		if old.Fingerprint != fingerprint {
			return Activity{}, ErrOperation
		}
		return old.Activity, nil
	}
	now := time.Now().UTC()
	item := Item{ID: auth.Token(), SpaceID: spaceID, Version: 1, CreatedBy: actor.ID, CreatedAt: now}
	activity := Activity{ID: auth.Token(), SpaceID: spaceID, Actor: actor, Kind: "item.created", CreatedAt: now, ChangedFields: []string{"type", "title", "description", "deadline", "priority", "status"}}
	verb := "added"
	if cmd.Action != "create" {
		item, err = repo.Item(cmd.ItemID)
		if err != nil {
			return Activity{}, err
		}
		if item.Deleted {
			return Activity{}, ErrNotFound
		}
		if item.Version != cmd.BaseVersion {
			return Activity{}, ErrConflict
		}
		previous := item
		activity.Previous = &previous
		item.Version++
		if cmd.Action == "delete" {
			item.Deleted = true
			activity.Kind = "item.deleted"
			activity.ChangedFields = []string{"deleted"}
			verb = "removed"
		} else {
			activity.Kind = "item.updated"
			activity.ChangedFields = changed(item.Fields, *cmd.Fields)
			verb = "updated"
			if item.Status != cmd.Fields.Status {
				if cmd.Fields.Status == "done" {
					activity.Kind = "item.completed"
					verb = "completed"
				} else {
					activity.Kind = "item.reopened"
					verb = "reopened"
				}
			}
		}
	}
	if cmd.Fields != nil {
		item.Fields = *cmd.Fields
	}
	item.UpdatedBy = actor.ID
	item.UpdatedAt = now
	activity.Item = item
	activity.Message = fmt.Sprintf("%s %s %q", actor.DisplayName, verb, item.Title)
	return repo.Commit(item, activity, cmd.OperationID, fingerprint)
}

func changed(old, next Fields) []string {
	result := []string{}
	if old.Type != next.Type {
		result = append(result, "type")
	}
	if old.Title != next.Title {
		result = append(result, "title")
	}
	if old.Description != next.Description {
		result = append(result, "description")
	}
	if old.Deadline != next.Deadline {
		result = append(result, "deadline")
	}
	if old.Priority != next.Priority {
		result = append(result, "priority")
	}
	if old.Status != next.Status {
		result = append(result, "status")
	}
	return result
}
