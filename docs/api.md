# Network layer API

## Start and configure

```sh
go run ./cmd/server -dev -origin http://localhost:8080
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | Listen address |
| `-db` | `data/timeline.db` | Embedded database file |
| `-origin` | `https://localhost:8080` | Exact browser origin, without trailing slash |
| `-dev` | `false` | Allow HTTP cookies for a localhost origin |
| `-tls-cert`, `-tls-key` | empty | Certificate and key for direct HTTPS |

Outside development, serve HTTPS directly or use an HTTPS reverse proxy that
preserves Origin and supports WebSocket upgrades. Bind the upstream to a private
interface. Cookies remain Secure when TLS terminates at the proxy. Serve the
eventual frontend and API through the same public origin; cross-origin frontends
are not supported.

The database contains password hashes and session digests. Keep it private;
back it up while the server is stopped. Only one process may open it.

## Authentication and request rules

Every write request requires `Content-Type: application/json` and an `Origin`
exactly matching configuration, including login and registration. This is the
CSRF defense; missing Origin is rejected even for command-line clients.
WebSocket handshakes also require this Origin. No wildcard CORS is enabled.

Registration and login issue an opaque `timeline_session` HttpOnly,
SameSite=Strict cookie, Secure outside dev. Sessions expire after seven days,
persist across restarts, and are separate per device. Only SHA-256 token digests
are stored. Passwords use salted PBKDF2-HMAC-SHA256 with 600,000 iterations and
must be 12–256 bytes.

Usernames are case-insensitive, normalized to lowercase, and contain 3–32 ASCII
letters, digits or underscores. Display names are 1–100 bytes. Registration is
open. Email verification and forgotten-password recovery are not implemented.
Authenticated password changes revoke every session for the account.

Credential endpoints share a 20-attempt/minute limit per direct peer IP and at
most four simultaneous password operations. Joining is limited to 30 attempts
per minute per peer. Forwarded IP headers are not trusted; behind a proxy these
limits are shared by clients using that proxy. Limits reset on server restart.
Production edge rate limits need deployment-specific configuration.

## Endpoints

All paths are relative to the server origin. Successful responses use JSON;
errors have `{"error":"message"}`. IDs are opaque strings issued by the server.

| Method and path | Input / behavior |
| --- | --- |
| `GET /` | HTML backend landing page; no authentication |
| `GET /healthz` | Liveness response; no authentication |
| `POST /api/auth/register` | `username`, `display_name`, `password`; creates account and session, returns account, HTTP 201 |
| `POST /api/auth/login` | `username`, `password`; returns account and new device session |
| `POST /api/auth/logout` | `{}`; revokes current session and clears cookie |
| `POST /api/auth/password` | `current_password`, `new_password`; revokes all account sessions |
| `GET /api/me` | Current account ID, username and display name |
| `GET /api/spaces` | Array of accessible spaces |
| `POST /api/spaces` | `name`; creates space and owner membership, HTTP 201 |
| `POST /api/join` | `key`; returns pending status, or approved status with space for an existing member |
| `GET /api/spaces/{space}` | Document, revision, ID, name, role; key included only for owner |
| `GET /api/spaces/{space}/members` | Owner-only membership and join-request lists |
| `POST /api/spaces/{space}/members/{account}` | Owner-only `role`: editor, viewer, revoked, or rejected |
| `POST /api/spaces/{space}/rotate-key` | Owner-only `{}`; returns new key, preserves memberships and pending requests |
| `PUT /api/spaces/{space}/document` | Versioned document write, described below |
| `GET /api/spaces/{space}/live` | Authenticated WebSocket upgrade |

Each named input field belongs in a JSON object. Role assignment requires an
existing member or pending request. Owner access cannot be assigned, removed,
or transferred through this endpoint. Editors can write documents; viewers can
read. Repeated pending requests are combined. Rejected or revoked users can
request approval again but cannot regain access automatically. Pending users can
poll `/api/spaces` or retry `/api/join` to discover approval. Membership management
uses HTTP; no owner notification stream exists yet.

Errors: 400 invalid input, 401 unauthenticated/expired session, 403 insufficient
role or denied origin, 404 inaccessible space or unknown key, 409 revision or
operation conflict, 415 wrong content type, 429 rate limit, 503 live connection
limit. Inaccessible timeline data is never included in these responses.

## Try the access flow

Separate cookie files represent separate users. These passwords are local
demonstration values.

```sh
curl -c /tmp/timeline-owner.cookies \
  -H 'Origin: http://localhost:8080' -H 'Content-Type: application/json' \
  -d '{"username":"owner","display_name":"Owner","password":"local-demo-password"}' \
  http://localhost:8080/api/auth/register

curl -b /tmp/timeline-owner.cookies \
  -H 'Origin: http://localhost:8080' -H 'Content-Type: application/json' \
  -d '{"name":"Our timeline"}' http://localhost:8080/api/spaces

curl -c /tmp/timeline-member.cookies \
  -H 'Origin: http://localhost:8080' -H 'Content-Type: application/json' \
  -d '{"username":"member","display_name":"Member","password":"local-demo-password"}' \
  http://localhost:8080/api/auth/register
```

With the member cookie, POST `{"key":"RETURNED_KEY"}` to `/api/join`. With the
owner cookie, GET the space's `/members` endpoint and POST `{"role":"editor"}`
to `/api/spaces/{space}/members/{member-account-id}` to approve access.

## Document writes and retries

The initial document is `{}` at revision 0. This transport-level object is a
placeholder for the future task model; it does not validate task semantics.

```json
{
  "operation_id": "client-generated-unique-id",
  "base_revision": 0,
  "document": {"title": "Team activities"}
}
```

PUT this to `/api/spaces/{space}/document`. The document replaces the previous
object and must be at most 64 KiB; total requests are limited to 128 KiB.
Operation IDs must be 1–128 bytes and unique per actor within a space.

Success returns `id`, `space_id`, `actor_id`, `revision`, `document`, and
`created_at`. Actor identity comes from the session. Document, revision, durable
event, and deduplication record commit atomically before acknowledgment or delivery.

Retry the same operation ID with the same base revision and document after an
uncertain network result. It returns the original event, even after restart.
Reusing an ID with different input returns 409. A new operation using a stale
revision also returns 409; fetch current state and resolve the edit before
submitting with a new operation ID. Permissions are checked even on retries.
Application-level idempotency applies to document writes; space creation and key
rotation are not retry-deduplicated.

## Live protocol

Connect from a signed-in browser at the configured origin:

```js
const url = new URL(`/api/spaces/${spaceId}/live`, location.origin);
url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
const socket = new WebSocket(url);
socket.onmessage = ({ data }) => console.log(JSON.parse(data));
```

The first message is a fresh snapshot:

```json
{"type":"snapshot","space":{"id":"...","name":"Our timeline","role":"editor","revision":0,"document":{}}}
```

Subsequent changes are ordered by revision:

```json
{"type":"change","event":{"id":"...","space_id":"...","actor_id":"...","revision":1,"document":{"title":"Team activities"},"created_at":"..."}}
```

The server polls durable events every 250 ms, delivering at most 32 events per
connection per poll. Session and membership checks precede every delivery and
are serialized with access changes. Revocation and logout close affected sockets
on their next poll; no later state is sent after revocation commits. Data sent
earlier may already be buffered by the network. Role changes send a replacement
snapshot. Key rotation is visible via HTTP.

Reconnects start with a fresh snapshot. Durable events after that revision are
replayed, eliminating a subscription gap or dependency on in-memory broadcasts.
Clients should replace their snapshot, ignore duplicate revisions, and reconnect
if a revision skips. Implement retry delays with jitter and check HTTP session
and membership after denied access. Reconcile pending writes by retrying the
operation ID. Do not accept new offline edits.

Sockets are receive-only for application data: submit writes through HTTP.
The server pings every 20 seconds, waits up to 60 seconds for responsive peers,
sets a two-second write deadline, and permits 256 concurrent live connections.
Browsers answer protocol pings automatically.

## Scope and limits

No frontend, task scheduling rules, account recovery, ownership transfer, or
offline merging is included yet. Admin changes do not have timeline audit events.
Document events contain actor and timestamp, but there is no history endpoint.

Storage uses bbolt transactions with a single versioned JSON state record.
Whole-state reads/writes, unbounded event history, and serialized socket delivery
are intended for small deployments. A slow socket can hold the storage lock
until its write deadline. Before scaling, split records/indexes and event
retention into dedicated structures and decouple delivery while preserving access
checks. Multiple server instances are not supported.

Library references: [bbolt transactions](https://pkg.go.dev/go.etcd.io/bbolt),
[Gorilla WebSocket](https://pkg.go.dev/github.com/gorilla/websocket), and
[Go PBKDF2](https://pkg.go.dev/crypto/pbkdf2).
