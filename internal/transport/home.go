package transport

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	// Keep unknown API routes as errors. Only expose the public build artifacts,
	// never the source tree, configuration, directory listings or private files.
	if r.URL.Path == "/" {
		if s.config.WebDir != "" {
			index := filepath.Join(s.config.WebDir, "index.html")
			if info, err := os.Stat(index); err == nil && !info.IsDir() {
				http.ServeFile(w, r, index)
				return
			}
		}
		home(w, r)
		return
	}
	if s.config.WebDir != "" && (r.URL.Path == "/favicon.svg" || strings.HasPrefix(r.URL.Path, "/assets/")) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if !filepath.IsLocal(name) {
			http.NotFound(w, r)
			return
		}
		file := filepath.Join(s.config.WebDir, name)
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			http.ServeFile(w, r, file)
			return
		}
	}
	http.NotFound(w, r)
}

func home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Timeline</title>
</head>
<body>
  <main>
    <h1>Timeline</h1>
    <p>The backend is running.</p>
    <p>Accounts, shared tasks and events, deadlines, priorities, and live activity
    notifications are available through the API.
    Build the web interface with <code>cd web &amp;&amp; npm ci &amp;&amp; npm run build</code>,
    then refresh this page. Run the server from the project root or set
    <code>-web-dir</code> to the build directory.</p>
    <p><a href="/healthz">Check server health</a></p>
    <p>For setup and API examples, see <code>docs/api.md</code> in the project.</p>
  </main>
</body>
</html>
`)
}
