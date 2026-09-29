package transport

import (
	"net/http"
	"strconv"

	"github.com/GODGIRII/sequence/internal/spaces"
	"github.com/GODGIRII/sequence/internal/storage"
	"github.com/GODGIRII/sequence/internal/timeline"
)

// Authorization and item access use one transaction, including retry lookup.
func (s *Server) itemAccess(r *http.Request, write bool, fn func(*storage.TimelineRecords, spaces.Account) error) error {
	work := func(state *spaces.State, tx *storage.TimelineTx) error {
		a, err := identity(state, sessionID(r))
		if err != nil {
			return err
		}
		roles := []string{"owner", "editor", "viewer"}
		if write {
			roles = []string{"owner", "editor"}
		}
		if _, err := permitted(state, r.PathValue("space"), a.ID, roles...); err != nil {
			return err
		}
		return fn(tx.Space(r.PathValue("space")), a)
	}
	if write {
		return s.store.UpdateTimeline(work)
	}
	return s.store.ReadTimeline(work)
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OperationID string `json:"operation_id"`
		timeline.Fields
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	if input.Status == "" {
		input.Status = "open"
	}
	s.applyItem(w, r, timeline.Command{Action: "create", OperationID: input.OperationID, Fields: &input.Fields}, 201)
}

func (s *Server) updateItem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OperationID string `json:"operation_id"`
		BaseVersion uint64 `json:"base_version"`
		timeline.Fields
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	s.applyItem(w, r, timeline.Command{Action: "update", ItemID: r.PathValue("item"), OperationID: input.OperationID, BaseVersion: input.BaseVersion, Fields: &input.Fields}, 200)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OperationID string `json:"operation_id"`
		BaseVersion uint64 `json:"base_version"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	s.applyItem(w, r, timeline.Command{Action: "delete", ItemID: r.PathValue("item"), OperationID: input.OperationID, BaseVersion: input.BaseVersion}, 200)
}

func (s *Server) applyItem(w http.ResponseWriter, r *http.Request, command timeline.Command, status int) {
	var result timeline.Activity
	err := s.itemAccess(r, true, func(records *storage.TimelineRecords, a spaces.Account) error {
		var err error
		result, err = timeline.Apply(records, r.PathValue("space"), timeline.Actor{ID: a.ID, DisplayName: a.DisplayName}, command)
		return err
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, status, map[string]any{"item": result.Item, "activity": result})
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	var result timeline.Item
	err := s.itemAccess(r, false, func(records *storage.TimelineRecords, _ spaces.Account) error {
		var err error
		result, err = records.Item(r.PathValue("item"))
		if err == nil && result.Deleted {
			return timeline.ErrNotFound
		}
		return err
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}

func pageLimit(r *http.Request) (int, error) {
	values, ok := r.URL.Query()["limit"]
	if !ok {
		return 50, nil
	}
	if len(values) != 1 {
		return 0, fail(400, "limit must be an integer from 1 to 200")
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < 1 || n > 200 {
		return 0, fail(400, "limit must be an integer from 1 to 200")
	}
	return n, nil
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		report(w, err)
		return
	}
	q := r.URL.Query()
	filter := storage.ItemFilter{Limit: limit, After: q.Get("after"), Type: q.Get("type"), Priority: q.Get("priority"), Status: q.Get("status")}
	if (filter.Type != "" && filter.Type != "task" && filter.Type != "event") || (filter.Priority != "" && filter.Priority != "low" && filter.Priority != "medium" && filter.Priority != "high") || (filter.Status != "" && filter.Status != "open" && filter.Status != "done") || len(filter.After) > 128 {
		report(w, fail(400, "invalid item filter or cursor"))
		return
	}
	var result storage.ItemPage
	err = s.itemAccess(r, false, func(records *storage.TimelineRecords, _ spaces.Account) error {
		var err error
		result, err = records.Items(filter)
		return err
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}

func (s *Server) listActivities(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		report(w, err)
		return
	}
	var after uint64
	values, hasAfter := r.URL.Query()["after"]
	if hasAfter {
		if len(values) != 1 {
			report(w, fail(400, "invalid activity cursor"))
			return
		}
		after, err = strconv.ParseUint(values[0], 10, 64)
		if err != nil {
			report(w, fail(400, "invalid activity cursor"))
			return
		}
	}
	var result storage.ActivityPage
	err = s.itemAccess(r, false, func(records *storage.TimelineRecords, _ spaces.Account) error {
		if hasAfter && after > records.Sequence() {
			return fail(400, "activity cursor is ahead of this space")
		}
		var err error
		result, err = records.Activities(after, limit, !hasAfter)
		return err
	})
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}
