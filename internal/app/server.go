package app

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

//go:embed web
var webFiles embed.FS

// TokenHeader carries the per-launch secret on every API call.
const TokenHeader = "X-CleanMyWT-Token"

const maxBodyBytes = 1 << 20

// Server serves the UI and its JSON API on a loopback address.
//
// Anything on the machine, including web pages in a browser, can reach a
// loopback port, and this API deletes folders. So every API call must carry a
// random token that only the launched browser tab knows (custom headers can't
// be sent cross-origin without a CORS preflight, which is never granted), and
// the Host header must be the loopback address (blocks DNS rebinding).
type Server struct {
	state    *State
	token    string
	port     int
	platform string
}

// NewServer creates a server for state; Handler is ready once port is set.
func NewServer(state *State) *Server {
	return &Server{state: state, token: newToken(), platform: runtime.GOOS}
}

func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Token is the secret the UI must send.
func (s *Server) Token() string { return s.token }

// Listen binds a loopback port (0 picks a free one).
func (s *Server) Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	return ln, nil
}

// URL is the address to open, with the token in the fragment (never sent to the server).
func (s *Server) URL() string {
	return "http://127.0.0.1:" + strconv.Itoa(s.port) + "/#" + s.token
}

// Handler routes requests.
func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(webFiles, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.FS(static)))
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/stop", s.handleStop)
	mux.HandleFunc("POST /api/delete", s.handleDelete)
	mux.HandleFunc("POST /api/open", s.handleOpen)
	return s.guard(mux)
}

func (s *Server) guard(next http.Handler) http.Handler {
	port := strconv.Itoa(s.port)
	allowedHosts := map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			got := r.Header.Get(TokenHeader)
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				http.Error(w, "missing or wrong token", http.StatusForbidden)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.state.Snapshot(s.platform))
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var target Target
	if !readJSON(w, r, &target) {
		return
	}
	if target.Kind == "folder" {
		target.Path = wtkit.Real(wtkit.ExpandTilde(target.Path))
		if !wtkit.IsDirectory(target.Path) {
			writeError(w, http.StatusBadRequest, errors.New("that folder doesn't exist"))
			return
		}
	}
	if !validTarget(target) {
		writeError(w, http.StatusBadRequest, errors.New("unknown scan target"))
		return
	}
	if err := s.state.StartScan(target); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, _ *http.Request) {
	s.state.StopScan()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths          []string `json:"paths"`
		DeleteBranches bool     `json:"deleteBranches"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("nothing to delete"))
		return
	}
	id, err := s.state.StartDelete(req.Paths, req.DeleteBranches)
	switch {
	case errors.Is(err, errUnknown):
		writeError(w, http.StatusBadRequest, err)
	case err != nil:
		writeError(w, http.StatusConflict, err)
	default:
		writeJSON(w, http.StatusOK, map[string]int{"job": id})
	}
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string `json:"path"`
		Action string `json:"action"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if _, err := s.state.Lookup([]string{req.Path}); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var err error
	switch req.Action {
	case "reveal":
		err = Reveal(filepath.Clean(req.Path))
	case "terminal":
		err = OpenTerminal(filepath.Clean(req.Path))
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
