# Tasks, events, and live activity: backend MVP

The MVP has three features: authorized members can list tasks/events with a
deadline and priority; the space admin chooses who may edit; and connected
members receive live item activity. The server implements the rules and API.
The React frontend implements list/calendar views, priority colors, access
management, activity history, and toast notifications. See [frontend setup](frontend.md).

## Rules

- Each item belongs to exactly one space and is a `task` or `event`.
- Title is required, up to 200 bytes; description is optional, up to 5,000 bytes.
- Deadline is required: either a calendar date or a timestamp with an offset.
- Priority is `low`, `medium`, or `high`. The frontend chooses colors and labels.
- Status is `open` or `done`; creation defaults to `open` if omitted. Updating to
  `open` reopens a completed item. The same simple rules apply to both types.
- Past deadlines are allowed. No automatic reminder or overdue state is stored.
- The creator is the admin, represented by the existing `owner` role. Owners
  manage permissions; owners and editors can edit any item in the space.
  Viewers can list/read items, history, and live updates but cannot write.
- Joining still requires an account, space key, and owner approval. No public
  timeline access is enabled. See [membership API](api.md#endpoints).
- Removal is a soft delete: the item disappears from active reads and snapshots,
  but its history and retry result remain. There is no restore endpoint yet.

## Create an item

`POST /api/spaces/{space}/items`

```json
{
  "operation_id": "unique-create-operation",
  "type": "task",
  "title": "Submit project report",
  "description": "Include the final budget",
  "deadline": {"date": "2026-10-02"},
  "priority": "high",
  "status": "open"
}
```

For a specific time, use `"deadline":{"at":"2026-10-02T17:30:00+05:30"}`.
Exactly one of `date` and `at` is permitted. Dates remain dates; they do not turn
into midnight UTC. Timestamp values retain the supplied offset. Timestamps without
an offset are rejected. Recurring local-time deadlines are not supported.

Write requests require the existing session cookie, exact configured Origin, and
`Content-Type: application/json`, including DELETE. Unknown JSON fields are rejected.
The server assigns the ID, space, version, authors, and timestamps; clients cannot
choose those fields.

Success is HTTP 201 with `{"item":{...},"activity":{...}}`. The item includes
the editable fields plus `id`, `space_id`, `version:1`, `created_by`, `updated_by`,
`created_at`, `updated_at`, and `deleted:false`.

Example after creating a space using the [account examples](api.md#try-the-access-flow):

```sh
curl -b /tmp/timeline-owner.cookies \
  -H 'Origin: http://localhost:8080' -H 'Content-Type: application/json' \
  -d '{"operation_id":"example-1","type":"task","title":"Submit report","deadline":{"date":"2026-10-02"},"priority":"high"}' \
  http://localhost:8080/api/spaces/SPACE_ID/items
```

## Read, edit, complete, and remove

| Method and path suffix | Behavior |
| --- | --- |
| `GET /items` | Active items, with optional `type`, `priority`, `status`, `limit`, and `after` |
| `GET /items/{id}` | One active item; missing/deleted items return 404 |
| `PUT /items/{id}` | Full replacement of editable fields plus `operation_id` and `base_version` |
| `DELETE /items/{id}` | JSON body with `operation_id` and `base_version` |
| `GET /activities` | Recent item history or history after a sequence cursor |

All suffixes are under `/api/spaces/{space}`. Use the item's current version as
`base_version`. PUT requires type, title, deadline, priority, and status; omitted
description becomes empty. To complete/reopen an item, submit its fields with
status `done`/`open`. Success is HTTP 200 with item and activity.

Deletion example:

```json
{"operation_id":"unique-remove-operation","base_version":3}
```

Each accepted change increments that item's version. Two edits to different
items can both succeed even if they use the same version number. A stale edit to
the same item returns 409. Fetch the latest item and resolve the conflict before
submitting a new operation. No automatic merge occurs.

Operation IDs are 1–128 bytes, unique per account within the space's item API.
Retrying identical input under the same ID returns the original item/activity
without another change or notification, including after a restart or deletion.
Changing the target, action, fields, or base version under an existing operation
ID returns 409. Permissions are checked again on retries. Create retries return
201 as originally; clients should deduplicate by item/activity ID.

## Listing and activity history

Item lists return `items`, `sequence`, `next_after`, and `has_more`. Results are
ordered by opaque item ID; `after` is the last returned ID. Default limit is 50,
maximum 200. Keep filters unchanged across pages. The frontend can arrange the
live snapshot by deadline/priority for timeline display.

Each page reflects one transaction. Pages are not a frozen snapshot across
requests: concurrent inserts before the cursor may be missed by pagination.
Use the live snapshot for a consistent initial timeline, then apply activities.

Every committed item change adds a durable activity with:

- An ID and increasing `sequence` within its space.
- Kind: `item.created`, `item.updated`, `item.completed`, `item.reopened`, or
  `item.deleted`.
- Actor ID and the actor's display name at the time of the change.
- The resulting `item`, the `previous` item for edits/removals, and `changed_fields`.
- A plain-text message and creation timestamp.

For example: `Ananya completed "Submit project report"`. Deadline or priority
changes use `item.updated`, with exact before/after values in the activity.
A PUT that repeats current field values under a new operation ID is still a
versioned update and may have an empty `changed_fields` list.

`GET /activities` returns the most recent 50 activities in ascending sequence.
Use `?after=0&limit=50` to read from the beginning; subsequent pages use
`next_after`. Responses contain `activities`, `sequence` (latest committed
sequence), `next_after` (last delivered sequence), and `has_more`. Resume from
`next_after`, not the latest sequence if more pages remain. Invalid/ahead cursors
are rejected. Both list endpoints are checked against current membership.

## Live notifications

Use the existing authenticated `/api/spaces/{space}/live` WebSocket. Its first
message is now:

```json
{
  "type": "snapshot",
  "space": {"id":"...","name":"Team","role":"editor","revision":0,"document":{}},
  "items": [],
  "sequence": 0,
  "activities": []
}
```

`items` contains all active items. `activities` contains the latest 50 activity
records, and `sequence` is the current item activity sequence. All three are read
in one transaction. Replace local item state with this snapshot; recent activities
populate the feed, rather than being treated as new toast notifications.

After a change, connected authorized members receive:

```json
{"type":"activity","activity":{"id":"...","space_id":"...","sequence":1,"kind":"item.created","actor":{"id":"...","display_name":"Ananya"},"item":{},"changed_fields":[],"message":"Ananya added ...","created_at":"..."}}
```

The example abbreviates the item. Real messages contain the complete item, making
it possible to insert/replace it locally or remove it when `deleted` is true.
Display messages and item text as text, never trusted HTML.

The server checks persisted activities every 250 ms and sends up to 32 per poll.
The activity is committed before HTTP acknowledgment or WebSocket delivery. It
may arrive before the HTTP response; clients should deduplicate notifications by
activity ID and process item changes in sequence order. Reconnect yields a fresh
snapshot; history can be fetched after a saved sequence to catch older activity.

Role changes resend a snapshot. Removed members stop receiving state and their
connections close on the next access check. Existing logout/session expiry rules
also apply. There is no browser push, email, read/unread tracking, or timed reminder.

Legacy document `change` messages can still appear independently. Their revision
is separate from the item activity sequence and must not be used for item conflicts.

## Persistence and growth

Items, activities, and operation results occupy separate bbolt records under
space-specific buckets. An edit atomically saves one item, one activity, its
sequence, and its retry result. Failed transactions leave no item or sequence gap.
Opening an existing database adds buckets without deleting accounts, memberships,
sessions, or legacy documents. Legacy JSON documents are not converted to tasks.

The domain package (`internal/timeline`) depends on a repository interface rather
than HTTP or bbolt, leaving room for a different database later. This remains a
single-server implementation: authentication metadata uses the old state record,
history and tombstones have no retention policy, filters scan a space's items,
and live snapshots include all active items. Large deployments will need indexes,
bounded synchronization, retention, and changes to delivery locking.
