package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

func testServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s := NewServer(NewState(MemorySettings()))
	s.port = 4567
	return s, s.Handler()
}

func do(h http.Handler, method, path, host, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = host
	if token != "" {
		req.Header.Set(TokenHeader, token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPIRequiresToken(t *testing.T) {
	_, h := testServer(t)
	if rec := do(h, "GET", "/api/state", "127.0.0.1:4567", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("no token: got %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/state", "127.0.0.1:4567", "wrong", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token: got %d", rec.Code)
	}
}

func TestRejectsForeignHost(t *testing.T) {
	s, h := testServer(t)
	if rec := do(h, "GET", "/api/state", "evil.example:4567", s.Token(), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("rebinding host: got %d", rec.Code)
	}
	if rec := do(h, "GET", "/", "evil.example:4567", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("static from foreign host: got %d", rec.Code)
	}
}

func TestStateWithToken(t *testing.T) {
	s, h := testServer(t)
	rec := do(h, "GET", "/api/state", "localhost:4567", s.Token(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	var snap Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Target.Kind != "home" || len(snap.Targets) < 2 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

func TestServesUI(t *testing.T) {
	_, h := testServer(t)
	rec := do(h, "GET", "/", "127.0.0.1:4567", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Clean My Worktree") {
		t.Fatalf("index: %d", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("missing CSP: %q", csp)
	}
}

func TestDeleteRejectsPathsOutsideScan(t *testing.T) {
	s, h := testServer(t)
	dir := t.TempDir()
	rec := do(h, "POST", "/api/delete", "127.0.0.1:4567", s.Token(), `{"paths":["`+jsonEscape(dir)+`"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if !wtkit.IsDirectory(dir) {
		t.Fatal("folder outside the scan was deleted")
	}
}

func TestOpenRejectsPathsOutsideScan(t *testing.T) {
	s, h := testServer(t)
	rec := do(h, "POST", "/api/open", "127.0.0.1:4567", s.Token(), `{"path":"/etc","action":"reveal"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestScanRejectsMissingFolder(t *testing.T) {
	s, h := testServer(t)
	rec := do(h, "POST", "/api/scan", "127.0.0.1:4567", s.Token(), `{"kind":"folder","path":"/definitely/not/here"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestSettingsRecentFolders(t *testing.T) {
	s := MemorySettings()
	for _, p := range []string{"/a", "/b", "/a", "/c", "/d", "/e", "/f"} {
		s.SetTarget(Target{Kind: "folder", Path: p})
	}
	got := strings.Join(s.Recent(), ",")
	if got != "/f,/e,/d,/c,/a" {
		t.Fatalf("recent = %s", got)
	}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
