package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GODGIRII/timeline/internal/auth"
	"github.com/GODGIRII/timeline/internal/spaces"
	"github.com/GODGIRII/timeline/internal/storage"
	"github.com/gorilla/websocket"
)

const testPassword = "correct-horse-battery"

type fixture struct {
	t    *testing.T
	db   *storage.Store
	app  *Server
	http *httptest.Server
	path string
}

func start(t *testing.T, path string) *fixture {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewUnstartedServer(nil)
	app, err := New(db, Config{Origin: "http://" + h.Listener.Addr().String(), InsecureCookies: true})
	if err != nil {
		db.Close()
		h.Close()
		t.Fatal(err)
	}
	h.Config.Handler = app
	h.Start()
	f := &fixture{t: t, db: db, app: app, http: h, path: path}
	return f
}

func (f *fixture) close() {
	f.app.Close()
	f.http.Close()
	if err := f.db.Close(); err != nil {
		f.t.Error(err)
	}
}

type reply struct {
	status  int
	body    map[string]any
	raw     []byte
	cookies []*http.Cookie
}

func (f *fixture) request(method, path, token string, input any) reply {
	f.t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		f.t.Fatal(err)
	}
	r, err := http.NewRequest(method, f.http.URL+path, bytes.NewReader(data))
	if err != nil {
		f.t.Fatal(err)
	}
	r.Header.Set("Origin", f.http.URL)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	response, err := f.http.Client().Do(r)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return reply{response.StatusCode, body, raw, response.Cookies()}
}

func expect(t *testing.T, r reply, status int) reply {
	t.Helper()
	if r.status != status {
		t.Fatalf("status=%d want=%d body=%s", r.status, status, r.raw)
	}
	return r
}

func (f *fixture) register(name string) (string, string) {
	f.t.Helper()
	r := expect(f.t, f.request("POST", "/api/auth/register", "", map[string]string{"username": name, "display_name": name, "password": testPassword}), 201)
	if len(r.cookies) != 1 || !r.cookies[0].HttpOnly || r.cookies[0].SameSite != http.SameSiteStrictMode {
		f.t.Fatal("unsafe session cookie")
	}
	if bytes.Contains(r.raw, []byte("password")) {
		f.t.Fatal("password leaked")
	}
	return r.body["id"].(string), r.cookies[0].Value
}

func (f *fixture) login(name string) string {
	f.t.Helper()
	r := expect(f.t, f.request("POST", "/api/auth/login", "", map[string]string{"username": name, "password": testPassword}), 200)
	return r.cookies[0].Value
}

func (f *fixture) create(token string) (string, string) {
	f.t.Helper()
	r := expect(f.t, f.request("POST", "/api/spaces", token, map[string]string{"name": "Our timeline"}), 201)
	return r.body["id"].(string), r.body["key"].(string)
}

func (f *fixture) connect(token, id string) *websocket.Conn {
	f.t.Helper()
	h := http.Header{"Origin": []string{f.http.URL}, "Cookie": []string{cookieName + "=" + token}}
	c, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.http.URL, "http")+"/api/spaces/"+id+"/live", h)
	if err != nil {
		if res != nil {
			f.t.Log(res.Status)
		}
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { c.Close() })
	return c
}

func receive(t *testing.T, c *websocket.Conn, kind string) map[string]any {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var msg map[string]any
	if err := c.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != kind {
		t.Fatalf("message=%v want type=%s", msg, kind)
	}
	return msg
}

func change(id string, base int, value string) map[string]any {
	return map[string]any{"operation_id": id, "base_revision": base, "document": map[string]string{"title": value}}
}

func TestCollaborationAndRevocation(t *testing.T) {
	f := start(t, filepath.Join(t.TempDir(), "timeline.db"))
	defer f.close()
	ownerID, owner := f.register("owner")
	editorID, editor := f.register("editor")
	viewerID, viewer := f.register("viewer")
	secondDevice := f.login("editor")
	if secondDevice == editor {
		t.Fatal("devices share a session")
	}
	expect(t, f.request("GET", "/api/me", secondDevice, nil), 200)
	id, key := f.create(owner)
	base := "/api/spaces/" + id
	other, _ := f.create(viewer)
	expect(t, f.request("GET", base, "", nil), 401)
	expect(t, f.request("GET", base, editor, nil), 404)
	for _, token := range []string{editor, viewer} {
		for range 2 {
			r := expect(t, f.request("POST", "/api/join", token, map[string]string{"key": key}), 200)
			if r.body["status"] != "pending" || bytes.Contains(r.raw, []byte("document")) {
				t.Fatal("pending user received state")
			}
		}
	}
	expect(t, f.request("GET", base+"/members", editor, nil), 404)
	expect(t, f.request("PUT", base+"/document", editor, change("early", 0, "secret")), 404)
	for account, role := range map[string]string{editorID: "editor", viewerID: "viewer"} {
		expect(t, f.request("POST", base+"/members/"+account, owner, map[string]string{"role": role}), 200)
	}
	expect(t, f.request("POST", base+"/members/"+ownerID, owner, map[string]string{"role": "revoked"}), 409)
	expect(t, f.request("POST", base+"/members/"+viewerID, editor, map[string]string{"role": "editor"}), 403)
	expect(t, f.request("GET", base, secondDevice, nil), 200)
	expect(t, f.request("GET", "/api/spaces/"+other, editor, nil), 404)
	expect(t, f.request("PUT", base+"/document", viewer, change("denied", 0, "x")), 403)
	ownerConn := f.connect(owner, id)
	receive(t, ownerConn, "snapshot")
	editorConn := f.connect(secondDevice, id)
	receive(t, editorConn, "snapshot")
	view := expect(t, f.request("GET", base, viewer, nil), 200)
	if _, ok := view.body["key"]; ok {
		t.Fatal("join key leaked to non-owner")
	}
	write := expect(t, f.request("PUT", base+"/document", editor, change("first", 0, "shared")), 200)
	for _, c := range []*websocket.Conn{ownerConn, editorConn} {
		msg := receive(t, c, "change")
		event := msg["event"].(map[string]any)
		if event["revision"] != float64(1) || event["actor_id"] != editorID {
			t.Fatalf("wrong event: %v", event)
		}
	}
	retry := expect(t, f.request("PUT", base+"/document", editor, change("first", 0, "shared")), 200)
	if write.body["id"] != retry.body["id"] {
		t.Fatal("retry applied twice")
	}
	expect(t, f.request("PUT", base+"/document", editor, change("first", 0, "different")), 409)
	expect(t, f.request("PUT", base+"/document", owner, change("stale", 0, "old")), 409)
	editorConn.Close()
	expect(t, f.request("PUT", base+"/document", owner, change("offline", 1, "updated")), 200)
	reconnected := f.connect(secondDevice, id)
	snapshot := receive(t, reconnected, "snapshot")["space"].(map[string]any)
	if snapshot["revision"] != float64(2) {
		t.Fatalf("stale reconnect: %v", snapshot)
	}
	newKey := expect(t, f.request("POST", base+"/rotate-key", owner, map[string]any{}), 200).body["key"]
	if newKey == key {
		t.Fatal("key unchanged")
	}
	expect(t, f.request("POST", "/api/join", editor, map[string]string{"key": key}), 404)
	expect(t, f.request("GET", base, editor, nil), 200)
	expect(t, f.request("POST", base+"/members/"+editorID, owner, map[string]string{"role": "revoked"}), 200)
	expect(t, f.request("PUT", base+"/document", editor, change("removed", 2, "x")), 404)
	expect(t, f.request("GET", base, secondDevice, nil), 404)
	_ = reconnected.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := reconnected.ReadMessage(); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("revoked socket not closed: %v", err)
	}
	expect(t, f.request("POST", "/api/join", editor, map[string]any{"key": newKey}), 200)
	expect(t, f.request("GET", base, editor, nil), 404)
	expect(t, f.request("POST", base+"/members/"+editorID, owner, map[string]string{"role": "editor"}), 200)
	logoutConn := f.connect(editor, id)
	receive(t, logoutConn, "snapshot")
	expect(t, f.request("POST", "/api/auth/logout", editor, map[string]any{}), 200)
	expect(t, f.request("GET", "/api/me", editor, nil), 401)
	expect(t, f.request("GET", "/api/me", secondDevice, nil), 200)
	_ = logoutConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := logoutConn.ReadMessage(); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("logout socket not closed: %v", err)
	}
}

func TestRestartAndConcurrentEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeline.db")
	f := start(t, path)
	_, token := f.register("owner")
	id, key := f.create(token)
	endpoint := "/api/spaces/" + id
	var wg sync.WaitGroup
	results := make(chan reply, 2)
	for _, op := range []string{"one", "two"} {
		wg.Add(1)
		go func(op string) {
			defer wg.Done()
			results <- f.request("PUT", endpoint+"/document", token, change(op, 0, op))
		}(op)
	}
	wg.Wait()
	close(results)
	statuses := map[int]int{}
	var winning reply
	for result := range results {
		statuses[result.status]++
		if result.status == 200 {
			winning = result
		}
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent writes: %v", statuses)
	}
	f.close()
	f = start(t, path)
	defer f.close()
	r := expect(t, f.request("GET", endpoint, token, nil), 200)
	if r.body["revision"] != float64(1) || r.body["key"] != key {
		t.Fatalf("state lost on restart: %s", r.raw)
	}
	op := winning.body["document"].(map[string]any)["title"].(string)
	retry := expect(t, f.request("PUT", endpoint+"/document", token, change(op, 0, op)), 200)
	if retry.body["id"] != winning.body["id"] {
		t.Fatal("operation history lost")
	}
	c := f.connect(token, id)
	receive(t, c, "snapshot")
	expect(t, f.request("PUT", endpoint+"/document", token, change("next", 1, "next")), 200)
	receive(t, c, "change")
}

func TestAuthenticationAndRequestGuards(t *testing.T) {
	f := start(t, filepath.Join(t.TempDir(), "timeline.db"))
	defer f.close()
	_, token := f.register("alice")
	second := f.login("alice")
	expect(t, f.request("POST", "/api/auth/login", "", map[string]string{"username": "alice", "password": "wrong"}), 401)
	expect(t, f.request("POST", "/api/auth/login", "", map[string]string{"username": "absent", "password": "wrong"}), 401)
	expect(t, f.request("POST", "/api/auth/register", "", map[string]string{"username": "short", "display_name": "s", "password": "short"}), 400)
	expect(t, f.request("POST", "/api/auth/register", "", map[string]string{"username": "ALICE", "display_name": "a", "password": testPassword}), 409)
	id, _ := f.create(token)
	for _, origin := range []string{"", "https://attacker.example"} {
		r, _ := http.NewRequest("POST", f.http.URL+"/api/spaces", strings.NewReader(`{"name":"bad"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		resp, err := f.http.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("cross-origin write allowed")
		}
		h := http.Header{"Origin": []string{origin}, "Cookie": []string{cookieName + "=" + token}}
		conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.http.URL, "http")+"/api/spaces/"+id+"/live", h)
		if conn != nil {
			conn.Close()
		}
		if err == nil || resp == nil || resp.StatusCode != 403 {
			t.Fatal("cross-origin live connection allowed")
		}
		resp.Body.Close()
	}
	h := http.Header{"Origin": []string{f.http.URL}}
	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.http.URL, "http")+"/api/spaces/"+id+"/live", h)
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatal("anonymous live connection allowed")
	}
	resp.Body.Close()
	expect(t, f.request("PUT", "/api/spaces/"+id+"/document", token, map[string]any{"operation_id": "no-base", "document": map[string]any{}}), 400)
	expect(t, f.request("PUT", "/api/spaces/"+id+"/document", token, map[string]any{"operation_id": "spoof", "base_revision": 0, "document": map[string]any{}, "actor_id": "someone"}), 400)
	c := f.connect(second, id)
	receive(t, c, "snapshot")
	expect(t, f.request("POST", "/api/auth/password", token, map[string]string{"current_password": testPassword, "new_password": "a-different-long-password"}), 200)
	for _, old := range []string{token, second} {
		expect(t, f.request("GET", "/api/me", old, nil), 401)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := c.ReadMessage(); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("password change left live session: %v", err)
	}
	login := expect(t, f.request("POST", "/api/auth/login", "", map[string]string{"username": "alice", "password": "a-different-long-password"}), 200)
	token = login.cookies[0].Value
	if err := f.db.Update(func(state *spaces.State) error {
		session := state.Sessions[auth.Digest(token)]
		session.ExpiresAt = time.Now().Add(-time.Second)
		state.Sessions[auth.Digest(token)] = session
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expect(t, f.request("GET", "/api/me", token, nil), 401)
	// Exhaust the attempt budget before performing expensive password work.
	for range 21 {
		f.request("POST", "/api/auth/login", "", map[string]any{})
	}
	expect(t, f.request("POST", "/api/auth/login", "", map[string]any{}), 429)
}

func TestProductionConfiguration(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, config := range []Config{{Origin: "http://example.com"}, {Origin: "http://example.com", InsecureCookies: true}, {Origin: "https://example.com/"}} {
		if s, err := New(db, config); err == nil {
			s.Close()
			t.Fatalf("unsafe config accepted: %+v", config)
		}
	}
	s, err := New(db, Config{Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := httptest.NewRecorder()
	s.cookie(w, "secret")
	c := w.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatal("production cookie not secure")
	}
}

func TestLiveReplayIsolationAndRoleChanges(t *testing.T) {
	f := start(t, filepath.Join(t.TempDir(), "timeline.db"))
	defer f.close()
	_, owner := f.register("owner")
	memberID, member := f.register("member")
	id, key := f.create(owner)
	other, _ := f.create(owner)
	base := "/api/spaces/" + id
	expect(t, f.request("POST", "/api/join", member, map[string]string{"key": key}), 200)
	h := http.Header{"Origin": []string{f.http.URL}, "Cookie": []string{cookieName + "=" + member}}
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.http.URL, "http")+base+"/live", h)
	if conn != nil {
		conn.Close()
	}
	if err == nil || response == nil || response.StatusCode != 404 {
		t.Fatal("pending member subscribed")
	}
	response.Body.Close()
	expect(t, f.request("POST", base+"/members/"+memberID, owner, map[string]string{"role": "rejected"}), 200)
	expect(t, f.request("GET", base, member, nil), 404)
	expect(t, f.request("POST", "/api/join", member, map[string]string{"key": key}), 200)
	expect(t, f.request("POST", base+"/members/"+memberID, owner, map[string]string{"role": "editor"}), 200)
	c := f.connect(member, id)
	// Writes can commit during the snapshot handoff. Every subsequent revision
	// must arrive, including enough events to cross the per-poll delivery limit.
	done := make(chan struct{})
	go func() {
		defer close(done)
		expect(t, f.request("PUT", "/api/spaces/"+other+"/document", owner, change("private", 0, "private")), 200)
		for i := 0; i < 40; i++ {
			expect(t, f.request("PUT", base+"/document", owner, change(fmt.Sprint(i), i, "shared")), 200)
		}
	}()
	initial := receive(t, c, "snapshot")["space"].(map[string]any)
	for rev := int(initial["revision"].(float64)) + 1; rev <= 40; rev++ {
		event := receive(t, c, "change")["event"].(map[string]any)
		if event["revision"] != float64(rev) || event["space_id"] != id {
			t.Fatalf("gap or cross-space delivery: %v", event)
		}
	}
	<-done
	expect(t, f.request("POST", base+"/members/"+memberID, owner, map[string]string{"role": "viewer"}), 200)
	updated := receive(t, c, "snapshot")["space"].(map[string]any)
	if updated["role"] != "viewer" || updated["revision"] != float64(40) {
		t.Fatalf("wrong role update: %v", updated)
	}
	expect(t, f.request("PUT", base+"/document", member, change("downgraded", 40, "denied")), 403)
}
