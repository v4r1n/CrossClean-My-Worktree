// Command cleanmywt starts the Clean My Worktree app: a local web server that
// opens in your browser and lists every git worktree on this computer.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/app"
	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

const shutdownTimeout = 3 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, app.OpenURL))
}

// run is the whole program: it serves until ctx is cancelled and returns the
// exit code. openURL opens the browser (replaced in tests).
func run(ctx context.Context, args []string, stdout, stderr io.Writer, openURL func(string) error) int {
	flags := flag.NewFlagSet("cleanmywt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	port := flags.Int("port", 0, "port to listen on, on 127.0.0.1 (0 picks a free one)")
	noBrowser := flags.Bool("no-browser", false, "don't open the browser; just print the link")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if !wtkit.GitAvailable() {
		fmt.Fprintln(stderr, "cleanmywt: git was not found. Install git (https://git-scm.com) and make sure it is on PATH.")
		return 1
	}

	settings := app.LoadSettings()
	state := app.NewState(settings)
	server := app.NewServer(state)
	ln, err := server.Listen(*port)
	if err != nil {
		fmt.Fprintf(stderr, "cleanmywt: can't listen: %v\n", err)
		return 1
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}

	if err := state.StartScan(settings.Target()); err != nil {
		fmt.Fprintf(stderr, "cleanmywt: initial scan: %v\n", err)
	}

	url := server.URL()
	fmt.Fprintf(stdout, "Clean My Worktree is running at\n\n    %s\n\nKeep this window open while you use it. Press Ctrl+C to quit.\n", url)
	if !*noBrowser {
		if err := openURL(url); err != nil {
			fmt.Fprintln(stdout, "Couldn't open the browser; open the link above yourself.")
		}
	}

	go func() {
		<-ctx.Done()
		state.StopScan()
		shutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "cleanmywt: %v\n", err)
		return 1
	}
	return 0
}
