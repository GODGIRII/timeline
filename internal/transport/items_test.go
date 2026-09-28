package transport

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func itemInput(op, title string) map[string]any {
	return map[string]any{"operation_id": op, "type": "task", "title": title, "description": "Details", "deadline": map[string]string{"date": "2026-10-01"}, "priority": "high", "status": "open"}
}

func itemReply(t *testing.T, r reply, status int) map[string]any {
	t.Helper()
	return expect(t, r, status).body["item"].(map[string]any)
}

func TestItemsPermissionsNotificationsAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	f := start(t, path)
	defer func() { f.close() }()
	ownerID, owner := f.register("owner")
	editorID, editor := f.register("editor")
	_, viewer := f.register("viewer")
	id, key := f.create(owner)
	base := "/api/spaces/" + id
	other, _ := f.create(editor)
	// A valid key alone gives neither read nor write access.
	expect(t, f.request("POST", "/api/join", editor, map[string]string{"key": key}), 200)
	expect(t, f.request("POST", base+"/items", editor, itemInput("pending", "Denied")), 404)
	expect(t, f.request("GET", base+"/activities", editor, nil), 404)
	expect(t, f.request("POST", base+"/members/"+editorID, owner, map[string]string{"role": "editor"}), 200)
	viewerMe := expect(t, f.request("GET", "/api/me", viewer, nil), 200)
	viewerID := viewerMe.body["id"].(string)
	expect(t, f.request("POST", "/api/join", viewer, map[string]string{"key": key}), 200)
	expect(t, f.request("POST", base+"/members/"+viewerID, owner, map[string]string{"role": "viewer"}), 200)
	expect(t, f.request("POST", base+"/items", viewer, itemInput("viewer", "Denied")), 403)
	expect(t, f.request("POST", base+"/members/"+viewerID, editor, map[string]string{"role": "editor"}), 403)
	c := f.connect(viewer, id)
	initial := receive(t, c, "snapshot")
	if initial["sequence"] != float64(0) || len(initial["items"].([]any)) != 0 {
		t.Fatalf("unexpected initial state: %v", initial)
	}
	// An event in another space must not leak into this live subscription.
	expect(t, f.request("POST", "/api/spaces/"+other+"/items", editor, itemInput("private", "Private")), 201)
	input := itemInput("create-task", "Report")
	created := expect(t, f.request("POST", base+"/items", editor, input), 201)
	item := created.body["item"].(map[string]any)
	itemID := item["id"].(string)
	endpoint := base + "/items/" + itemID
	if item["version"] != float64(1) || item["created_by"] != editorID || item["deadline"].(map[string]any)["date"] != "2026-10-01" {
		t.Fatalf("bad item: %v", item)
	}
	activity := receive(t, c, "activity")["activity"].(map[string]any)
	if activity["kind"] != "item.created" || activity["space_id"] != id || activity["sequence"] != float64(1) || !strings.Contains(activity["message"].(string), "Report") {
		t.Fatalf("bad activity: %v", activity)
	}
	retry := expect(t, f.request("POST", base+"/items", editor, input), 201)
	if retry.body["activity"].(map[string]any)["id"] != activity["id"] {
		t.Fatal("duplicate create notification")
	}
	expect(t, f.request("GET", endpoint, viewer, nil), 200)
	expect(t, f.request("GET", "/api/spaces/"+other+"/items/"+itemID, editor, nil), 404)
	input["title"] = "Changed operation input"
	expect(t, f.request("POST", base+"/items", editor, input), 409)
	edit := itemInput("finish", "Final report")
	edit["base_version"] = 1
	edit["status"] = "done"
	edit["priority"] = "medium"
	expect(t, f.request("PUT", endpoint, viewer, edit), 403)
	updated := itemReply(t, f.request("PUT", endpoint, owner, edit), 200)
	if updated["version"] != float64(2) || updated["updated_by"] != ownerID || updated["created_by"] != editorID {
		t.Fatalf("bad update: %v", updated)
	}
	activity = receive(t, c, "activity")["activity"].(map[string]any)
	if activity["kind"] != "item.completed" || activity["previous"].(map[string]any)["title"] != "Report" {
		t.Fatalf("missing history: %v", activity)
	}
	expect(t, f.request("PUT", endpoint, owner, edit), 200)
	edit["operation_id"] = "stale"
	expect(t, f.request("PUT", endpoint, editor, edit), 409)
	edit["operation_id"] = "reopen"
	edit["base_version"] = 2
	edit["status"] = "open"
	expect(t, f.request("PUT", endpoint, editor, edit), 200)
	if receive(t, c, "activity")["activity"].(map[string]any)["kind"] != "item.reopened" {
		t.Fatal("missing reopen activity")
	}
	deletion := map[string]any{"operation_id": "delete", "base_version": 3}
	expect(t, f.request("DELETE", endpoint, viewer, deletion), 403)
	deleted := itemReply(t, f.request("DELETE", endpoint, editor, deletion), 200)
	if deleted["deleted"] != true || deleted["version"] != float64(4) {
		t.Fatal("item not tombstoned")
	}
	if receive(t, c, "activity")["activity"].(map[string]any)["kind"] != "item.deleted" {
		t.Fatal("missing removal activity")
	}
	expect(t, f.request("DELETE", endpoint, editor, deletion), 200)
	expect(t, f.request("GET", endpoint, owner, nil), 404)
	listed := expect(t, f.request("GET", base+"/items", viewer, nil), 200)
	if len(listed.body["items"].([]any)) != 0 {
		t.Fatal("removed item in active list")
	}
	feed := expect(t, f.request("GET", base+"/activities?after=0&limit=2", viewer, nil), 200)
	if feed.body["has_more"] != true || feed.body["next_after"] != float64(2) || feed.body["sequence"] != float64(4) {
		t.Fatalf("bad feed cursor: %s", feed.raw)
	}
	feed = expect(t, f.request("GET", base+"/activities?after=2", viewer, nil), 200)
	if len(feed.body["activities"].([]any)) != 2 {
		t.Fatal("bad activity pagination")
	}
	// A dated event survives restart; its timestamp's offset is retained.
	event := itemInput("event", "Review meeting")
	event["type"] = "event"
	event["deadline"] = map[string]string{"at": "2026-10-02T17:30:00+05:30"}
	eventItem := itemReply(t, f.request("POST", base+"/items", owner, event), 201)
	receive(t, c, "activity")
	c.Close()
	f.close()
	f = start(t, path)
	expect(t, f.request("DELETE", endpoint, editor, deletion), 200)
	stored := expect(t, f.request("GET", base+"/items/"+eventItem["id"].(string), viewer, nil), 200)
	if stored.body["deadline"].(map[string]any)["at"] != "2026-10-02T17:30:00+05:30" {
		t.Fatal("timed deadline changed")
	}
	reconnected := f.connect(editor, id)
	snapshot := receive(t, reconnected, "snapshot")
	if snapshot["sequence"] != float64(5) || len(snapshot["items"].([]any)) != 1 || len(snapshot["activities"].([]any)) != 5 {
		t.Fatalf("restart snapshot lost data: %v", snapshot)
	}
	// Admin downgrade affects both HTTP writes and the live role snapshot.
	expect(t, f.request("POST", base+"/members/"+editorID, owner, map[string]string{"role": "viewer"}), 200)
	if receive(t, reconnected, "snapshot")["space"].(map[string]any)["role"] != "viewer" {
		t.Fatal("role change missing")
	}
	expect(t, f.request("POST", base+"/items", editor, itemInput("denied", "Denied")), 403)
	expect(t, f.request("POST", base+"/members/"+editorID, owner, map[string]string{"role": "revoked"}), 200)
	expect(t, f.request("GET", base+"/activities", editor, nil), 404)
	_ = reconnected.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := reconnected.ReadMessage(); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("revoked stream remains open: %v", err)
	}
}

func TestItemConcurrencyValidationAndPagination(t *testing.T) {
	f := start(t, filepath.Join(t.TempDir(), "db"))
	defer f.close()
	_, owner := f.register("owner")
	id, _ := f.create(owner)
	base := "/api/spaces/" + id
	for _, invalid := range []map[string]any{
		{"title": " "}, {"priority": "urgent"}, {"type": "unknown"}, {"status": "cancelled"},
		{"deadline": map[string]string{"date": "2026-02-30"}}, {"deadline": map[string]string{"at": "2026-09-28T12:00:00"}},
		{"deadline": map[string]string{"date": "2026-09-28", "at": "2026-09-28T12:00:00Z"}},
		{"deadline": nil}, {"created_by": "spoofed"}, {"description": strings.Repeat("x", 5001)},
	} {
		input := itemInput("invalid", "Valid")
		for key, value := range invalid {
			input[key] = value
		}
		expect(t, f.request("POST", base+"/items", owner, input), 400)
	}
	for _, query := range []string{"limit=0", "limit=201", "priority=urgent"} {
		expect(t, f.request("GET", base+"/items?"+query, owner, nil), 400)
	}
	for _, query := range []string{"after=-1", "after=bad", "after=1"} {
		expect(t, f.request("GET", base+"/activities?"+query, owner, nil), 400)
	}
	a := itemReply(t, f.request("POST", base+"/items", owner, itemInput("a", "Task A")), 201)
	bInput := itemInput("b", "Event B")
	bInput["type"] = "event"
	bInput["priority"] = "low"
	delete(bInput, "status")
	b := itemReply(t, f.request("POST", base+"/items", owner, bInput), 201)
	if b["status"] != "open" {
		t.Fatal("missing default status")
	}
	page := expect(t, f.request("GET", base+"/items?limit=1", owner, nil), 200)
	if page.body["has_more"] != true {
		t.Fatal("missing next page")
	}
	next := expect(t, f.request("GET", base+"/items?limit=1&after="+page.body["next_after"].(string), owner, nil), 200)
	if next.body["has_more"] != false || next.body["items"].([]any)[0].(map[string]any)["id"] == page.body["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("invalid item pagination")
	}
	filtered := expect(t, f.request("GET", base+"/items?priority=low&type=event&status=open", owner, nil), 200)
	if len(filtered.body["items"].([]any)) != 1 {
		t.Fatal("filter mismatch")
	}
	// Independent item versions let unrelated edits succeed concurrently.
	results := make(chan reply, 2)
	var wg sync.WaitGroup
	for i, item := range []map[string]any{a, b} {
		wg.Add(1)
		go func(i int, item map[string]any) {
			defer wg.Done()
			input := itemInput(fmt.Sprintf("edit-%d", i), "Updated")
			input["base_version"] = 1
			results <- f.request("PUT", base+"/items/"+item["id"].(string), owner, input)
		}(i, item)
	}
	wg.Wait()
	for range 2 {
		expect(t, <-results, 200)
	}
	// Two new edits of the same version cannot both commit.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := itemInput(fmt.Sprintf("race-%d", i), "Race")
			input["base_version"] = 2
			results <- f.request("PUT", base+"/items/"+a["id"].(string), owner, input)
		}(i)
	}
	wg.Wait()
	statuses := map[int]int{}
	for range 2 {
		statuses[(<-results).status]++
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("lost update: %v", statuses)
	}
	feed := expect(t, f.request("GET", base+"/activities", owner, nil), 200)
	if feed.body["sequence"] != float64(5) {
		t.Fatalf("rejected writes created events: %s", feed.raw)
	}
}
