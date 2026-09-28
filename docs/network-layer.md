# Connection and access layer

Status: first backend implementation complete. See [API usage](api.md) for exact
behavior and limits. Task-specific logic and the browser UI remain future work.

## Purpose and scope

Allow multiple people on different devices to connect to the same shared timeline
space using a shared space key and their own account and password. This layer owns connection, access,
sessions, and delivery of updates. Task scheduling and timeline editing rules
will be designed separately.

## Connection model

Browsers connect to a central Go server. They do not connect directly to each
other. The server checks access and is the authority for shared state.

Use HTTPS requests to create or join spaces, submit changes, fetch state, and
leave a session. Use an authenticated WebSocket connection to receive live
updates. A failed live connection must not erase saved state or imply that a
pending edit succeeded.

```text
Browser A ---- HTTPS / WebSocket ----+
                                    Go server ---- Persistent storage
Browser B ---- HTTPS / WebSocket ----+
```

A space key identifies the space; it does not grant access by itself. Successful
authentication creates a session, so the browser does not send the password with
every edit or live update.

## Selected identity model

Each person signs in with their own account and password. The shared key
identifies a space. Membership connects an account to a space, giving the same
person a stable identity across devices. Each device has its own revocable
session. A space has no shared password.

Implemented joining rule: a signed-in user enters the space key to request access.
The owner approves the request and assigns editor or viewer access. Existing
active members enter directly. Knowing the key alone does not grant access.
Owner approval follows the design adopted for this implementation.

## Logical records

- Space: internal ID, public join key, creation time, and lifecycle status.
- Account: internal ID, unique login identifier, password hash, and display name.
- Membership: space ID, account ID, role, and active/revoked status.
- Join request: space ID, account ID, and pending/approved/rejected status.
- Session: authenticated identity, expiration, and revocation status.
- Connection: temporary live socket attached to a session and authorized space.
- Space revision: increasing number identifying the latest committed change.

Passwords and session secrets must never appear in broadcast events or logs.
Store password hashes rather than plaintext passwords. Keep sessions in secure,
HttpOnly cookies; do not put passwords or session tokens in URLs. Check the
browser origin for live connections and protect state-changing HTTP requests
against cross-site request forgery. Rate-limit credential attempts.

## Proposed access rules

The signed-in space creator receives owner access. Owners manage the space key and
membership. Editors can maintain timeline content. Viewers can only read it.
Role assignment is server-controlled; a joining client cannot choose owner access.

Every request and live subscription must be checked against active membership.
An open socket is not permanent permission. Revoking access invalidates relevant
sessions or memberships and disconnects affected live subscriptions.

Space creation assigns an unpredictable join key and atomically creates the
owner membership. Repeated pending requests from the same account are combined.
A revoked member cannot rejoin without fresh owner approval. Changing the key
does not revoke existing memberships; removing a member does.

Registration uses a unique username and personal password. Authenticated password
changes revoke all sessions; forgotten-password recovery remains future work.
A space key must never confer ownership.

## Join and connection flow

1. The browser submits the user's account credentials over HTTPS. The server
   validates them and establishes a device session.
2. The signed-in browser submits the space key. The server checks space
   availability and membership. Invalid keys receive a generic unavailable
   response without disclosing private space details.
3. Active members receive their permitted space and role. Other users submit a
   join request and remain pending without timeline access until owner approval.
   Rejected requests do not grant access. The remaining steps require an active
   membership.
4. The browser opens an authenticated live connection and requests synchronization.
5. The server registers the subscription and provides a consistent snapshot plus
   its revision, followed by changes after that revision. Subscription and snapshot
   handoff must prevent changes from being lost between the two operations.
6. Only then does the browser mark the timeline as live.

Proposed browser connection states:

```text
Disconnected -> Authenticating -> Synchronizing -> Live
                       |                |           |
                       v                +-----------+-> Reconnecting
                  Access denied                          |
                                                        v
                                                   Synchronizing
```

Pending membership is a separate access state: the browser waits for approval
before synchronizing. An expired or revoked session returns to authentication. A temporary network
failure triggers retries with increasing delays and jitter. The UI distinguishes
connecting, live, reconnecting, and access denied.

## Delivery and simultaneous edits

Persist an accepted change before acknowledging or broadcasting it. Broadcast
only within the authorized space. Each committed change carries an event ID,
space ID, revision, and actor ID. The server derives actor identity from the
session, never from an untrusted client field.

Clients submit a unique operation ID with each document write. Retrying the same operation
must return its original result rather than apply it twice. The operation record
and the change must commit together. Writes also identify the revision they are
based on; stale writes return a conflict so one participant cannot silently
overwrite another's changes. Detailed merge behavior belongs to the timeline
logic design.

For the first version, reconnecting fetches a fresh snapshot through the same
consistent subscription handoff. Clients ignore duplicate revisions and
resynchronize if revisions skip. A failed broadcast after a successful commit
must eventually cause resynchronization; the implementation needs durable event
delivery or periodic revision checks, not just an in-memory broadcast.

While disconnected, the first version does not accept new edits. An edit whose
acknowledgment was lost remains pending until its operation ID is checked or
retried. Do not present an uncertain write as saved or failed without reconciling
with the server. Offline editing is outside the initial scope.

## Session and credential lifecycle

Logging out revokes the current session and closes its connections. It does not
delete the space or its timeline. Temporary disconnection does not remove a
membership.

Membership removal revokes that account's access to the space across devices,
without removing access to other spaces. Password reset should revoke existing
account sessions. Sessions expire after seven days. Forgotten-password recovery
remains to be designed.

## Acceptance scenarios for implementation

- Two browsers with valid access join the same space and receive its state.
- The same account on two devices has the same memberships and separate sessions.
- A valid key without approved membership cannot expose timeline data.
- Owner approval grants only the selected role to the requesting account.
- Invalid credentials cannot read state, submit edits, or subscribe to updates.
- Participants in one space never receive another space's data.
- A viewer's write is rejected even when sent directly to the server.
- An accepted edit reaches the other connected browser with its new revision.
- Retrying a write after losing its acknowledgment does not duplicate it.
- Concurrent stale edits produce an explicit conflict.
- Reconnection converges to persisted state, including changes made while offline.
- Revoked access stops HTTP writes and live delivery.
- Restarting the server preserves spaces, memberships, and committed state.

## Implementation structure

Directories are added as their first files are implemented:

```text
cmd/server/             Go server entry point
internal/auth/          Credentials and sessions
internal/spaces/        Spaces, membership, and permission rules
internal/transport/     HTTP endpoints and WebSocket connections
internal/storage/       Persistence implementation
```

Go tests live beside their packages. Transport integration tests exercise HTTP,
WebSockets, permissions, concurrent writes, and restarts. Add `web/` when browser
implementation begins.
