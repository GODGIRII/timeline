package storage

import (
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/GODGIRII/sequence/internal/spaces"
	"github.com/GODGIRII/sequence/internal/timeline"
	bolt "go.etcd.io/bbolt"
)

var itemsBucket = []byte("items_v1")
var activityBucket = []byte("activities_v1")
var operationsBucket = []byte("item_operations_v1")

// TimelineTx exposes individually stored records alongside a consistent view of
// authentication metadata. It is valid only inside its transaction callback.
type TimelineTx struct{ tx *bolt.Tx }
type TimelineRecords struct {
	tx    *bolt.Tx
	space string
}

func (s *Store) ReadTimeline(fn func(*spaces.State, *TimelineTx) error) error {
	return s.timelineTransaction(false, false, fn)
}
func (s *Store) UpdateTimeline(fn func(*spaces.State, *TimelineTx) error) error {
	return s.timelineTransaction(true, false, fn)
}
func (t *TimelineTx) Space(id string) *TimelineRecords { return &TimelineRecords{t.tx, id} }

func (r *TimelineRecords) bucket(name []byte) *bolt.Bucket {
	return r.tx.Bucket(name).Bucket([]byte(r.space))
}

func (r *TimelineRecords) Item(id string) (timeline.Item, error) {
	var item timeline.Item
	b := r.bucket(itemsBucket)
	if b == nil {
		return item, timeline.ErrNotFound
	}
	raw := b.Get([]byte(id))
	if raw == nil {
		return item, timeline.ErrNotFound
	}
	err := json.Unmarshal(raw, &item)
	return item, err
}

func operationKey(actor, operation string) []byte {
	// JSON encodes arbitrary operation IDs without delimiter collisions.
	key, _ := json.Marshal([2]string{actor, operation})
	return key
}

func (r *TimelineRecords) Operation(actor, operation string) (timeline.Operation, bool, error) {
	var result timeline.Operation
	b := r.bucket(operationsBucket)
	if b == nil {
		return result, false, nil
	}
	raw := b.Get(operationKey(actor, operation))
	if raw == nil {
		return result, false, nil
	}
	err := json.Unmarshal(raw, &result)
	return result, true, err
}

func putJSON(b *bolt.Bucket, key []byte, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return b.Put(key, raw)
}

func sequenceKey(sequence uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, sequence)
	return key
}

func (r *TimelineRecords) Commit(item timeline.Item, activity timeline.Activity, operation, fingerprint string) (timeline.Activity, error) {
	if !r.tx.Writable() {
		return timeline.Activity{}, errors.New("read-only timeline transaction")
	}
	buckets := make([]*bolt.Bucket, 3)
	for i, name := range [][]byte{itemsBucket, activityBucket, operationsBucket} {
		b, err := r.tx.Bucket(name).CreateBucketIfNotExists([]byte(r.space))
		if err != nil {
			return timeline.Activity{}, err
		}
		buckets[i] = b
	}
	sequence, err := buckets[1].NextSequence()
	if err != nil {
		return timeline.Activity{}, err
	}
	activity.Sequence = sequence
	if err := putJSON(buckets[0], []byte(item.ID), item); err != nil {
		return timeline.Activity{}, err
	}
	if err := putJSON(buckets[1], sequenceKey(sequence), activity); err != nil {
		return timeline.Activity{}, err
	}
	if err := putJSON(buckets[2], operationKey(activity.Actor.ID, operation), timeline.Operation{Fingerprint: fingerprint, Activity: activity}); err != nil {
		return timeline.Activity{}, err
	}
	return activity, nil
}

func (r *TimelineRecords) Sequence() uint64 {
	if b := r.bucket(activityBucket); b != nil {
		return b.Sequence()
	}
	return 0
}

type ItemFilter struct {
	Type, Priority, Status, After string
	Limit                         int
}
type ItemPage struct {
	Items     []timeline.Item `json:"items"`
	Sequence  uint64          `json:"sequence"`
	NextAfter string          `json:"next_after"`
	HasMore   bool            `json:"has_more"`
}

func (r *TimelineRecords) Items(filter ItemFilter) (ItemPage, error) {
	page := ItemPage{Items: []timeline.Item{}, Sequence: r.Sequence()}
	b := r.bucket(itemsBucket)
	if b == nil {
		return page, nil
	}
	c := b.Cursor()
	k, v := c.First()
	if filter.After != "" {
		k, v = c.Seek([]byte(filter.After))
		if string(k) == filter.After {
			k, v = c.Next()
		}
	}
	for ; k != nil; k, v = c.Next() {
		var item timeline.Item
		if err := json.Unmarshal(v, &item); err != nil {
			return page, err
		}
		if item.Deleted || (filter.Type != "" && item.Type != filter.Type) || (filter.Priority != "" && item.Priority != filter.Priority) || (filter.Status != "" && item.Status != filter.Status) {
			continue
		}
		if filter.Limit > 0 && len(page.Items) == filter.Limit {
			page.HasMore = true
			break
		}
		page.Items = append(page.Items, item)
		page.NextAfter = item.ID
	}
	return page, nil
}

type ActivityPage struct {
	Activities []timeline.Activity `json:"activities"`
	Sequence   uint64              `json:"sequence"`
	NextAfter  uint64              `json:"next_after"`
	HasMore    bool                `json:"has_more"`
}

// Activities returns ascending events after a cursor. With recent=true, return
// the most recent limit events in ascending order for a reconnect snapshot.
func (r *TimelineRecords) Activities(after uint64, limit int, recent bool) (ActivityPage, error) {
	latest := r.Sequence()
	if recent {
		after = 0
		if latest > uint64(limit) {
			after = latest - uint64(limit)
		}
	}
	page := ActivityPage{Activities: []timeline.Activity{}, Sequence: latest, NextAfter: after}
	b := r.bucket(activityBucket)
	if b == nil || after >= latest {
		return page, nil
	}
	c := b.Cursor()
	for k, v := c.Seek(sequenceKey(after + 1)); k != nil; k, v = c.Next() {
		if len(page.Activities) == limit {
			page.HasMore = true
			break
		}
		var event timeline.Activity
		if err := json.Unmarshal(v, &event); err != nil {
			return page, err
		}
		page.Activities = append(page.Activities, event)
		page.NextAfter = event.Sequence
	}
	return page, nil
}
