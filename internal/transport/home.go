package transport

import (
	"io"
	"net/http"
)

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
    <p>Accounts, shared spaces, and live connections are available through the API.
    The timeline web interface is not built yet.</p>
    <p><a href="/healthz">Check server health</a></p>
    <p>For setup and API examples, see <code>docs/api.md</code> in the project.</p>
  </main>
</body>
</html>
`)
}
