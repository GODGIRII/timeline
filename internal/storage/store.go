// Package storage provides atomic, durable transactions for the single-server MVP.
package storage

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/GODGIRII/sequence/internal/spaces"
	bolt "go.etcd.io/bbolt"
)

type Store struct {
	db *bolt.DB
	// Serialize delivery with permission changes: after revocation commits, no
	// previously authorized callback can send another event.
	mu sync.Mutex
}

var bucket = []byte("timeline")
var stateKey = []byte("state")

func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucket, itemsBucket, activityBucket, operationsBucket} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { s.mu.Lock(); defer s.mu.Unlock(); return s.db.Close() }

func (s *Store) transaction(write bool, fn func(*spaces.State) error) error {
	return s.timelineTransaction(write, true, func(state *spaces.State, _ *TimelineTx) error { return fn(state) })
}

func (s *Store) timelineTransaction(write, saveMetadata bool, fn func(*spaces.State, *TimelineTx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	work := func(tx *bolt.Tx) error {
		state := spaces.NewState()
		if raw := tx.Bucket(bucket).Get(stateKey); raw != nil {
			if err := json.Unmarshal(raw, state); err != nil {
				return err
			}
			if state.Version != 1 {
				return fmt.Errorf("unsupported database schema %d", state.Version)
			}
		}
		if err := fn(state, &TimelineTx{tx: tx}); err != nil {
			return err
		}
		if !write || !saveMetadata {
			return nil
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return tx.Bucket(bucket).Put(stateKey, raw)
	}
	if write {
		return s.db.Update(work)
	}
	return s.db.View(work)
}

func (s *Store) Read(fn func(*spaces.State) error) error   { return s.transaction(false, fn) }
func (s *Store) Update(fn func(*spaces.State) error) error { return s.transaction(true, fn) }
