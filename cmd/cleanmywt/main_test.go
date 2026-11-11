package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/testrepo"
)

// syncBuffer is a bytes.Buffer safe to write from the server goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var linkPattern = regexp.MustCompile(`http://127\.0\.0\.1:\d+/#([0-9a-f]+)`)

func TestServesScanAndShutsDownCleanly(t *testing.T) {
	testrepo.RequireGit(t)
	home := testrepo.Isolate(t)
	testrepo.NewFixture(t, home)

	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr syncBuffer
	var opened string
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, nil, &stdout, &stderr, func(url string) error { opened = url; return nil })
	}()

	link := waitFor(t, func() []string { return linkPattern.FindStringSubmatch(stdout.String()) })
	base, token := strings.SplitN(link[0], "/#", 2)[0], link[1]

	// The default target is the (isolated) home folder, which holds the fixture.
	var snap struct {
		Progress struct{ Phase string }
		Rows     []struct{ Verdict string }
	}
	waitFor(t, func() []string {
		getJSON(t, base+"/api/state", token, &snap)
		if snap.Progress.Phase == "idle" && len(snap.Rows) == 3 {
			return []string{"ok"}
		}
		return nil
	})

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down")
	}
	if opened != link[0] {
		t.Errorf("browser opened %q, want %q", opened, link[0])
	}
}

func TestNoBrowserFlagSkipsOpening(t *testing.T) {
	testrepo.RequireGit(t)
	testrepo.Isolate(t)
	ctx, cancel := context.WithCancel(context.Background())
	var stdout syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"--no-browser"}, &stdout, io.Discard, func(string) error {
			t.Error("browser should not open")
			return nil
		})
	}()
	waitFor(t, func() []string { return linkPattern.FindStringSubmatch(stdout.String()) })
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestBrowserFailureIsReported(t *testing.T) {
	testrepo.RequireGit(t)
	testrepo.Isolate(t)
	ctx, cancel := context.WithCancel(context.Background())
	var stdout syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, nil, &stdout, io.Discard, func(string) error { return io.EOF })
	}()
	waitFor(t, func() []string {
		if strings.Contains(stdout.String(), "Couldn't open the browser") {
			return []string{"ok"}
		}
		return nil
	})
	cancel()
	<-done
}

func TestBadFlagExitsTwo(t *testing.T) {
	if code := run(context.Background(), []string{"--nope"}, io.Discard, io.Discard, nil); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if code := run(context.Background(), []string{"-h"}, io.Discard, io.Discard, nil); code != 0 {
		t.Fatalf("help exit %d", code)
	}
}

func TestBusyPortExitsOne(t *testing.T) {
	testrepo.RequireGit(t)
	testrepo.Isolate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"--port", port}, io.Discard, &stderr, nil); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "can't listen") {
		t.Errorf("stderr: %q", stderr.String())
	}
}

func getJSON(t *testing.T, url, token string, v any) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-CleanMyWT-Token", token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", url, res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls until check returns a non-nil result or 30 seconds pass.
func waitFor(t *testing.T, check func() []string) []string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if got := check(); got != nil {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out")
	return nil
}
