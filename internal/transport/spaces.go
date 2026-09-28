package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GODGIRII/timeline/internal/auth"
	"github.com/GODGIRII/timeline/internal/spaces"
)

func (s *Server) listSpaces(w http.ResponseWriter, r *http.Request) {
	result := []any{}
	err := s.access(r, false, func(state *spaces.State, a spaces.Account) error {
		for _, space := range state.Spaces {
			if space.Members[a.ID] != "" {
				result = append(result, publicSpace(space, a.ID))
			}
		}
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}

func (s *Server) createSpace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 {
		report(w, fail(400, "name must be 1–100 bytes"))
		return
	}
	var result any
	err := s.access(r, true, func(state *spaces.State, a spaces.Account) error {
		space := &spaces.Space{ID: auth.Token(), Key: auth.Token(), Name: input.Name, CreatedAt: time.Now().UTC(), Members: map[string]string{a.ID: "owner"}, Requests: map[string]string{}, Document: json.RawMessage(`{}`), Events: []spaces.Event{}, Operations: map[string]spaces.Operation{}}
		state.Spaces[space.ID] = space
		result = publicSpace(space, a.ID)
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 201, result)
}

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	if !s.allow(r, "join", 30) {
		report(w, fail(429, "too many join attempts"))
		return
	}
	var input struct {
		Key string `json:"key"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	var result any
	err := s.access(r, true, func(state *spaces.State, a spaces.Account) error {
		for _, space := range state.Spaces {
			if space.Key != input.Key {
				continue
			}
			if role := space.Members[a.ID]; role != "" {
				result = map[string]any{"status": "approved", "space": publicSpace(space, a.ID)}
				return nil
			}
			space.Requests[a.ID] = "pending"
			result = map[string]string{"status": "pending"}
			return nil
		}
		return fail(404, "space unavailable")
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	var result any
	err := s.access(r, false, func(state *spaces.State, a spaces.Account) error {
		space, err := permitted(state, r.PathValue("space"), a.ID, "owner", "editor", "viewer")
		if err != nil {
			return err
		}
		result = publicSpace(space, a.ID)
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	var result any
	err := s.access(r, false, func(state *spaces.State, a spaces.Account) error {
		space, err := permitted(state, r.PathValue("space"), a.ID, "owner")
		if err != nil {
			return err
		}
		members := []any{}
		requests := []any{}
		for id, role := range space.Members {
			members = append(members, map[string]any{"account": publicAccount(state.Accounts[id]), "role": role})
		}
		for id, status := range space.Requests {
			requests = append(requests, map[string]any{"account": publicAccount(state.Accounts[id]), "status": status})
		}
		result = map[string]any{"members": members, "requests": requests}
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}

// Assign a role to approve a pending request or change an existing membership.
// "revoked" removes access; "rejected" rejects a pending request.
func (s *Server) setMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Role string `json:"role"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	if input.Role != "editor" && input.Role != "viewer" && input.Role != "revoked" && input.Role != "rejected" {
		report(w, fail(400, "invalid role"))
		return
	}
	err := s.access(r, true, func(state *spaces.State, a spaces.Account) error {
		space, err := permitted(state, r.PathValue("space"), a.ID, "owner")
		if err != nil {
			return err
		}
		target := r.PathValue("account")
		if space.Members[target] == "owner" {
			return fail(409, "owner membership cannot be changed")
		}
		if space.Members[target] == "" && space.Requests[target] != "pending" {
			return fail(404, "member or pending request not found")
		}
		if input.Role == "rejected" {
			if space.Requests[target] != "pending" {
				return fail(409, "no pending request")
			}
			space.Requests[target] = "rejected"
		} else if input.Role == "revoked" {
			delete(space.Members, target)
			space.Requests[target] = "revoked"
		} else {
			space.Members[target] = input.Role
			space.Requests[target] = "approved"
		}
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, map[string]string{"status": input.Role})
}

func (s *Server) rotateKey(w http.ResponseWriter, r *http.Request) {
	var key string
	err := s.access(r, true, func(state *spaces.State, a spaces.Account) error {
		space, err := permitted(state, r.PathValue("space"), a.ID, "owner")
		if err != nil {
			return err
		}
		key = auth.Token()
		space.Key = key
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, map[string]string{"key": key})
}

func (s *Server) writeDocument(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OperationID  string          `json:"operation_id"`
		BaseRevision *uint64         `json:"base_revision"`
		Document     json.RawMessage `json:"document"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	if len(input.OperationID) < 1 || len(input.OperationID) > 128 || input.BaseRevision == nil || len(input.Document) == 0 || len(input.Document) > 64<<10 || input.Document[0] != '{' {
		report(w, fail(400, "operation_id, base_revision and an object document (up to 64 KiB) are required"))
		return
	}
	var result spaces.Event
	err := s.access(r, true, func(state *spaces.State, a spaces.Account) error {
		space, err := permitted(state, r.PathValue("space"), a.ID, "owner", "editor")
		if err != nil {
			return err
		}
		result, err = space.Apply(a.ID, input.OperationID, *input.BaseRevision, input.Document)
		if errors.Is(err, spaces.ErrConflict) || errors.Is(err, spaces.ErrOperation) {
			return fail(409, err.Error())
		}
		return err
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}
