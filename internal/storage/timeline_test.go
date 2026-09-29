package storage

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/GODGIRII/sequence/internal/spaces"
	"github.com/GODGIRII/sequence/internal/timeline"
	bolt "go.etcd.io/bbolt"
)

func TestTimelineRollbackAndLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	// Simulate the first-layer format: only the metadata bucket exists.
	legacy, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = legacy.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bucket)
		if err != nil {
			return err
		}
		state := spaces.NewState()
		state.Accounts["owner"] = spaces.Account{ID: "owner", DisplayName: "Owner"}
		return putJSON(b, stateKey, state)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cmd := timeline.Command{Action: "create", OperationID: "create", Fields: &timeline.Fields{Type: "task", Title: "Report", Deadline: timeline.Deadline{Date: "2026-10-01"}, Priority: "high", Status: "open"}}
	abort := errors.New("abort transaction")
	var attempted timeline.Activity
	err = s.UpdateTimeline(func(state *spaces.State, tx *TimelineTx) error {
		if state.Accounts["owner"].DisplayName != "Owner" {
			t.Fatal("legacy account lost")
		}
		var err error
		attempted, err = timeline.Apply(tx.Space("space"), "space", timeline.Actor{ID: "owner", DisplayName: "Owner"}, cmd)
		if err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	err = s.ReadTimeline(func(_ *spaces.State, tx *TimelineTx) error {
		records := tx.Space("space")
		if records.Sequence() != 0 {
			t.Fatal("sequence survived rollback")
		}
		if _, err := records.Item(attempted.Item.ID); !errors.Is(err, timeline.ErrNotFound) {
			t.Fatal("item survived rollback")
		}
		if _, found, err := records.Operation("owner", "create"); err != nil || found {
			t.Fatal("operation survived rollback")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.UpdateTimeline(func(_ *spaces.State, tx *TimelineTx) error {
		activity, err := timeline.Apply(tx.Space("space"), "space", timeline.Actor{ID: "owner", DisplayName: "Owner"}, cmd)
		if err == nil && activity.Sequence != 1 {
			t.Fatal("failed transaction left a sequence gap")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
