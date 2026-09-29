# Sequence web app

The frontend uses React, TypeScript, Vite, Tailwind CSS, and Lucide icons. It
communicates with the existing API using Fetch and native WebSockets. Fonts use
the system stack; the app does not depend on external image/font services.

## Run the complete app

From the repository root:

```sh
npm ci --prefix web
npm run build --prefix web
go run ./cmd/server -dev -origin http://localhost:8080
```

Open **http://localhost:8080/**. If an older server is already using port 8080,
stop it before starting the updated server. The build is already generated after
development, but a fresh checkout needs the build command. Go serves `web/dist`;
`-web-dir` can point to an absolute build directory when starting elsewhere.
Unknown API paths remain errors, and source/configuration files are not served.

Register an account, create a space, and add a task or event. No example accounts
or tasks are inserted into your database. To test collaboration, open a private
browser window, register a second account, and use **Join a space** with the key
from **Invite people**. The admin approves requests and chooses viewing/editing
permissions. Open access controls poll for new requests every ten seconds;
approved spaces appear in the second user's sidebar on the same interval.

The default database remains `data/timeline.db` to preserve existing accounts
and spaces. Use `-db data/sequence-testing.db` for a separate test dataset.
Production uses HTTPS and Secure cookies as before.

## Frontend development

Terminal 1, from the repository root:

```sh
go run ./cmd/server -dev -origin http://localhost:5173
```

Terminal 2:

```sh
npm run dev --prefix web
```

Open **http://localhost:5173/**. Vite proxies `/api` (including WebSockets) to
Go on port 8080 and preserves the browser Origin. Use this exact hostname: the
backend checks Origin, and cookies belong to a hostname. Do not mix localhost
and 127.0.0.1 URLs. The development server uses a fixed port so origin checks
cannot silently break when a port is occupied. No broad CORS allowance is added.

## What is available

- Sign in, create an account, sign out, and restore the existing device session.
- Create a space, request to join by key, and switch between approved spaces.
- Admin membership approval, viewing/editing roles, removal, key copying/rotation.
- Create/read/edit/remove tasks or events; mark done and reopen with a checkbox.
- Required date-only deadlines or optional times in the device's local timezone.
- Low, medium, and high priorities, with colors plus readable labels.
- List and monthly calendar views; title/description search, status/type/priority
  filters, and deadline/priority sorting. The list groups items by deadline date.
- Open/overdue/completed summaries based on the selected space's real items.
- Live activity feed, notifications for teammates' updates, and earlier history.
- Mobile sidebar, responsive forms, keyboard-focusable controls, native modal
  dialogs, connection/error states, and reduced-motion support.

The **Upcoming** tab means all open items, including overdue ones. Click the
overdue summary to focus on late work. Events and tasks follow the same simple
open/done model. The calendar displays timed items in the device's timezone.
Email/push reminders, recurrence, password recovery, and offline editing remain
outside the MVP.

## Live state and save behavior

The WebSocket snapshot is the authority for items and the activity cursor.
Notifications are deduplicated by sequence; a gap reconnects for a fresh snapshot.
Connections retry with exponential delay and jitter. HTTP checks distinguish
expired sessions and removed memberships from temporary network failures.
Writes are disabled until a live snapshot arrives. Background membership changes
replace the role snapshot; revoked access clears the timeline.

Edits include the version loaded when opening the form. Conflicts preserve the
draft and offer **Reload latest version**, rather than overwriting a teammate's
work. The item editor does not silently refresh fields during editing.

Each item change gets one operation ID. A request whose result is uncertain is
kept in sessionStorage under the account ID, and the UI blocks further item
writes until **Confirm change** retries that exact request. This can reconcile
a saved change even after a reload. Pending drafts contain task data, not login
credentials, and remain in that browser tab until resolved. Session credentials
stay exclusively in the server-issued HttpOnly cookie.

## Layout

```text
web/
  src/
    App.tsx                 Session bootstrap and workspace composition
    api.ts                  Typed requests and errors
    types.ts                API models
    dates.ts                Date-only and local-time presentation
    hooks/useTimeline.ts    WebSocket state, gap detection, reconnection
    hooks/useWriter.ts      Pending operation persistence and retry
    components/             Auth, item forms, spaces, membership, calendar/feed
    styles.css              Tailwind setup and responsive visual system
  tests/workspace.spec.ts   Browser integration tests
  public/favicon.svg       Local brand mark
  dist/                    Generated build, ignored by Git
```

## Checks

```sh
npm run build --prefix web
cd web
npx playwright install chromium
npm test
```

Browser tests start a real Go server on port 4180 with a unique temporary database
under `/tmp`. They cover account creation, space creation, editing, completion,
filters, calendar, conflicts, removal, responsive layout, membership changes,
live collaboration between separate browser contexts, and lost-response retries.
They do not touch the normal development database. Tests write visual previews
to `/tmp/sequence-auth.png`, `/tmp/sequence-desktop.png`, and `/tmp/sequence-mobile.png`.

Continue running `go test -race ./...` and `go vet ./...` for the backend. The
frontend build checks TypeScript and produces a static deployment bundle.
