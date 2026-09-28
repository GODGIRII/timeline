// Package transport exposes the HTTP API and authenticated live subscriptions.
package transport

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GODGIRII/timeline/internal/auth"
	"github.com/GODGIRII/timeline/internal/spaces"
	"github.com/GODGIRII/timeline/internal/storage"
	"github.com/gorilla/websocket"
)

const cookieName = "timeline_session"
const sessionTTL = 7 * 24 * time.Hour

type Config struct {
	Origin          string
	InsecureCookies bool // Explicit localhost-only development mode.
}

type limitEntry struct {
	count int
	reset time.Time
}

type Server struct {
	store     *storage.Store
	config    Config
	mux       *http.ServeMux
	dummyHash string
	limitMu   sync.Mutex
	limits    map[string]limitEntry
	authSlots chan struct{}
	liveMu    sync.Mutex
	live      map[*websocket.Conn]struct{}
	closed    bool
}

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string           { return e.message }
func fail(status int, message string) error { return &apiError{status, message} }

func New(store *storage.Store, config Config) (*Server, error) {
	u, err := url.Parse(config.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("origin must be an exact http(s) origin without a trailing slash")
	}
	if config.InsecureCookies {
		if u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
			return nil, errors.New("insecure cookies require an HTTP localhost origin")
		}
	} else if u.Scheme != "https" {
		return nil, errors.New("HTTPS origin required outside local development")
	}
	dummy, err := auth.Hash(auth.Token())
	if err != nil {
		return nil, err
	}
	s := &Server{store: store, config: config, mux: http.NewServeMux(), dummyHash: dummy, limits: map[string]limitEntry{}, authSlots: make(chan struct{}, 4), live: map[*websocket.Conn]struct{}{}}
	s.mux.HandleFunc("GET /{$}", home)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	s.mux.HandleFunc("POST /api/auth/register", s.register)
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)
	s.mux.HandleFunc("POST /api/auth/password", s.password)
	s.mux.HandleFunc("GET /api/me", s.me)
	s.mux.HandleFunc("GET /api/spaces", s.listSpaces)
	s.mux.HandleFunc("POST /api/spaces", s.createSpace)
	s.mux.HandleFunc("POST /api/join", s.join)
	s.mux.HandleFunc("GET /api/spaces/{space}", s.snapshot)
	s.mux.HandleFunc("GET /api/spaces/{space}/members", s.members)
	s.mux.HandleFunc("POST /api/spaces/{space}/members/{account}", s.setMember)
	s.mux.HandleFunc("POST /api/spaces/{space}/rotate-key", s.rotateKey)
	s.mux.HandleFunc("PUT /api/spaces/{space}/document", s.writeDocument)
	s.mux.HandleFunc("GET /api/spaces/{space}/live", s.websocket)
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" && r.Method != "HEAD" {
		// Require Origin even on login to prevent login CSRF. No wildcard CORS.
		if r.Header.Get("Origin") != s.config.Origin {
			report(w, fail(403, "origin denied"))
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			report(w, fail(415, "application/json required"))
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	s.mux.ServeHTTP(w, r)
}

func decode(r *http.Request, target any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fail(400, "invalid JSON body")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fail(400, "expected one JSON object")
	}
	return nil
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func report(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		respond(w, api.status, map[string]string{"error": api.message})
		return
	}
	log.Printf("API storage error: %v", err)
	respond(w, 500, map[string]string{"error": "internal server error"})
}

func sessionID(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return auth.Digest(c.Value)
}

func identity(state *spaces.State, token string) (spaces.Account, error) {
	session, ok := state.Sessions[token]
	if !ok || !time.Now().Before(session.ExpiresAt) {
		return spaces.Account{}, fail(401, "authentication required")
	}
	account, ok := state.Accounts[session.AccountID]
	if !ok {
		return spaces.Account{}, fail(401, "authentication required")
	}
	return account, nil
}

func (s *Server) access(r *http.Request, write bool, fn func(*spaces.State, spaces.Account) error) error {
	work := func(state *spaces.State) error {
		account, err := identity(state, sessionID(r))
		if err != nil {
			return err
		}
		return fn(state, account)
	}
	if write {
		return s.store.Update(work)
	}
	return s.store.Read(work)
}

func permitted(state *spaces.State, id, account string, roles ...string) (*spaces.Space, error) {
	space, ok := state.Spaces[id]
	if !ok || space.Members[account] == "" {
		return nil, fail(404, "space unavailable")
	}
	for _, role := range roles {
		if role == space.Members[account] {
			return space, nil
		}
	}
	return nil, fail(403, "permission denied")
}

func publicAccount(a spaces.Account) any {
	return map[string]string{"id": a.ID, "username": a.Username, "display_name": a.DisplayName}
}

func publicSpace(space *spaces.Space, account string) map[string]any {
	v := map[string]any{"id": space.ID, "name": space.Name, "role": space.Members[account], "revision": space.Revision, "document": space.Document}
	if space.Members[account] == "owner" {
		v["key"] = space.Key
	}
	return v
}

func (s *Server) cookie(w http.ResponseWriter, token string) {
	c := &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: !s.config.InsecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())}
	if token == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func addSession(state *spaces.State, account, token string) {
	for key, value := range state.Sessions {
		if !time.Now().Before(value.ExpiresAt) {
			delete(state.Sessions, key)
		}
	}
	state.Sessions[auth.Digest(token)] = spaces.Session{AccountID: account, ExpiresAt: time.Now().Add(sessionTTL)}
}

// Limits are bounded and use the direct peer address, never untrusted proxy headers.
func (s *Server) allow(r *http.Request, category string, max int) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	now := time.Now()
	for key, entry := range s.limits {
		if now.After(entry.reset) {
			delete(s.limits, key)
		}
	}
	key := category + ":" + ip
	entry, ok := s.limits[key]
	if !ok {
		if len(s.limits) >= 10000 {
			return false
		}
		entry.reset = now.Add(time.Minute)
	}
	entry.count++
	s.limits[key] = entry
	return entry.count <= max
}

func (s *Server) credentialSlot(w http.ResponseWriter, r *http.Request) bool {
	if !s.allow(r, "credentials", 20) {
		report(w, fail(429, "too many attempts; retry in one minute"))
		return false
	}
	select {
	case s.authSlots <- struct{}{}:
		return true
	default:
		report(w, fail(429, "authentication busy; retry later"))
		return false
	}
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.credentialSlot(w, r) {
		return
	}
	defer func() { <-s.authSlots }()
	var input struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(input.Username) || input.DisplayName == "" || len(input.DisplayName) > 100 {
		report(w, fail(400, "username must be 3–32 lowercase letters, digits or underscores; display_name must be 1–100 bytes"))
		return
	}
	hash, err := auth.Hash(input.Password)
	if err != nil {
		report(w, fail(400, err.Error()))
		return
	}
	a := spaces.Account{ID: auth.Token(), Username: input.Username, DisplayName: input.DisplayName, PasswordHash: hash}
	token := auth.Token()
	err = s.store.Update(func(state *spaces.State) error {
		for _, old := range state.Accounts {
			if old.Username == a.Username {
				return fail(409, "username unavailable")
			}
		}
		state.Accounts[a.ID] = a
		addSession(state, a.ID, token)
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	s.cookie(w, token)
	respond(w, 201, publicAccount(a))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.credentialSlot(w, r) {
		return
	}
	defer func() { <-s.authSlots }()
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	var a spaces.Account
	err := s.store.Read(func(state *spaces.State) error {
		for _, candidate := range state.Accounts {
			if candidate.Username == strings.ToLower(strings.TrimSpace(input.Username)) {
				a = candidate
				break
			}
		}
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	hash := a.PasswordHash
	if hash == "" {
		hash = s.dummyHash
	}
	if !auth.Verify(hash, input.Password) || a.ID == "" {
		report(w, fail(401, "invalid credentials"))
		return
	}
	token := auth.Token()
	err = s.store.Update(func(state *spaces.State) error {
		if state.Accounts[a.ID].PasswordHash != hash {
			return fail(401, "credentials changed; sign in again")
		}
		addSession(state, a.ID, token)
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	s.cookie(w, token)
	respond(w, 200, publicAccount(a))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	err := s.store.Update(func(state *spaces.State) error { delete(state.Sessions, sessionID(r)); return nil })
	if err != nil {
		report(w, err)
		return
	}
	s.cookie(w, "")
	respond(w, 200, map[string]bool{"logged_out": true})
}

func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	if !s.credentialSlot(w, r) {
		return
	}
	defer func() { <-s.authSlots }()
	var input struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &input); err != nil {
		report(w, err)
		return
	}
	var a spaces.Account
	err := s.access(r, false, func(_ *spaces.State, account spaces.Account) error { a = account; return nil })
	if err != nil {
		report(w, err)
		return
	}
	if !auth.Verify(a.PasswordHash, input.Current) {
		report(w, fail(401, "invalid credentials"))
		return
	}
	hash, err := auth.Hash(input.New)
	if err != nil {
		report(w, fail(400, err.Error()))
		return
	}
	err = s.access(r, true, func(state *spaces.State, current spaces.Account) error {
		if current.PasswordHash != a.PasswordHash {
			return fail(409, "credentials changed; sign in again")
		}
		current.PasswordHash = hash
		state.Accounts[current.ID] = current
		for key, session := range state.Sessions {
			if session.AccountID == current.ID {
				delete(state.Sessions, key)
			}
		}
		return nil
	})
	if err != nil {
		report(w, err)
		return
	}
	s.cookie(w, "")
	respond(w, 200, map[string]bool{"sign_in_required": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	var result any
	err := s.access(r, false, func(_ *spaces.State, a spaces.Account) error { result = publicAccount(a); return nil })
	if err != nil {
		report(w, err)
		return
	}
	respond(w, 200, result)
}
