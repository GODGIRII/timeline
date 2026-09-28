package transport

import (
	"net/http"
	"time"

	"github.com/GODGIRII/timeline/internal/spaces"
	"github.com/GODGIRII/timeline/internal/storage"
	"github.com/gorilla/websocket"
)

// Close terminates hijacked WebSocket connections during server shutdown.
func (s *Server) Close() {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.closed = true
	for conn := range s.live {
		_ = conn.Close()
	}
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != s.config.Origin {
		report(w, fail(403, "origin denied"))
		return
	}
	err := s.access(r, false, func(state *spaces.State, a spaces.Account) error {
		_, err := permitted(state, r.PathValue("space"), a.ID, "owner", "editor", "viewer")
		return err
	})
	if err != nil {
		report(w, err)
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == s.config.Origin }}
	s.liveMu.Lock()
	if s.closed || len(s.live) >= 256 {
		s.liveMu.Unlock()
		report(w, fail(503, "live connections unavailable"))
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.liveMu.Unlock()
		return
	}
	s.live[conn] = struct{}{}
	s.liveMu.Unlock()
	defer func() { _ = conn.Close(); s.liveMu.Lock(); delete(s.live, conn); s.liveMu.Unlock() }()
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "use HTTP for writes"), time.Now().Add(time.Second))
			return
		}
	}()
	var revision uint64
	var sequence uint64
	first := true
	lastRole := ""
	deliver := func() error {
		return s.store.ReadTimeline(func(state *spaces.State, tx *storage.TimelineTx) error {
			a, err := identity(state, sessionID(r))
			if err != nil {
				return err
			}
			space, err := permitted(state, r.PathValue("space"), a.ID, "owner", "editor", "viewer")
			if err != nil {
				return err
			}
			records := tx.Space(space.ID)
			if first || lastRole != space.Members[a.ID] {
				items, err := records.Items(storage.ItemFilter{})
				if err != nil {
					return err
				}
				activities, err := records.Activities(0, 50, true)
				if err != nil {
					return err
				}
				_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if err := conn.WriteJSON(map[string]any{"type": "snapshot", "space": publicSpace(space, a.ID), "items": items.Items, "sequence": items.Sequence, "activities": activities.Activities}); err != nil {
					return err
				}
				first = false
				lastRole = space.Members[a.ID]
				revision = space.Revision
				sequence = items.Sequence
				return nil
			}
			// Events and the snapshot revision share the same durable transaction.
			// Any commit after a snapshot is found on the next poll, without a
			// subscription-registration gap or dependence on in-memory broadcasts.
			end := space.Revision
			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if end > revision+32 {
				end = revision + 32
			}
			for revision < end {
				event := space.Events[revision]
				if err := conn.WriteJSON(map[string]any{"type": "change", "event": event}); err != nil {
					return err
				}
				revision = event.Revision
			}
			activities, err := records.Activities(sequence, 32, false)
			if err != nil {
				return err
			}
			for _, activity := range activities.Activities {
				if err := conn.WriteJSON(map[string]any{"type": "activity", "activity": activity}); err != nil {
					return err
				}
				sequence = activity.Sequence
			}
			return nil
		})
	}
	if err := deliver(); err != nil {
		return
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := deliver(); err != nil {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "subscription ended"), time.Now().Add(time.Second))
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(2*time.Second)); err != nil {
				return
			}
		}
	}
}
