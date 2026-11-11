// Command cleanmywt starts the Clean My Worktree app: a local web server that
// opens in your browser and lists every git worktree on this computer.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/app"
	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

func main() {
	port := flag.Int("port", 0, "port to listen on, on 127.0.0.1 (0 picks a free one)")
	noBrowser := flag.Bool("no-browser", false, "don't open the browser; just print the link")
	flag.Parse()

	if !wtkit.GitAvailable() {
		fmt.Fprintln(os.Stderr, "cleanmywt: git was not found. Install git (https://git-scm.com) and make sure it is on PATH.")
		os.Exit(1)
	}

	settings := app.LoadSettings()
	state := app.NewState(settings)
	server := app.NewServer(state)
	ln, err := server.Listen(*port)
	if err != nil {
		log.Fatalf("cleanmywt: can't listen: %v", err)
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}

	if err := state.StartScan(settings.Target()); err != nil {
		log.Printf("initial scan: %v", err)
	}

	url := server.URL()
	fmt.Printf("Clean My Worktree is running at\n\n    %s\n\nKeep this window open while you use it. Press Ctrl+C to quit.\n", url)
	if !*noBrowser {
		if err := app.OpenURL(url); err != nil {
			fmt.Println("Couldn't open the browser; open the link above yourself.")
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("cleanmywt: %v", err)
	}
}
