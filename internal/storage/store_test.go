package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GODGIRII/sequence/internal/spaces"
)

func TestRollbackAndPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := errors.New("abort")
	err = s.Update(func(state *spaces.State) error {
		state.Accounts["discarded"] = spaces.Account{ID: "discarded"}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	if err := s.Read(func(state *spaces.State) error {
		if len(state.Accounts) != 0 {
			t.Fatal("aborted transaction persisted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("database is accessible to other users")
	}
}
