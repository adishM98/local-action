// Command local-action-gui is the double-clickable macOS app: same server
// as cmd/local-action, wrapped in a native window instead of being opened
// as a browser tab. Packaged into LocalAction.app by
// scripts/package-macos-app.sh; cmd/local-action (plain CLI, curl/Homebrew)
// is unaffected by any of this.
package main

import (
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	webview "github.com/webview/webview_go"

	"local-action/internal/db"
	"local-action/internal/httpapi"
	"local-action/internal/runs"
	"local-action/internal/secrets"
	"local-action/internal/terminal"
	"local-action/internal/ws"
	"local-action/web"
)

const addr = "127.0.0.1:8090"

// Cocoa requires all UI work happen on the process's original OS thread.
// Go's scheduler can otherwise migrate main() off it, silently breaking
// NSApplication (the process runs, the server answers, but no window or
// Dock icon ever appears) — must lock before any of that runs.
func init() {
	runtime.LockOSThread()
}

func main() {
	dataDir, err := appDataDir()
	if err != nil {
		log.Fatalf("resolve app data dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("create app data dir: %v", err)
	}

	// Finder-launched apps have no console, so send log output somewhere
	// findable instead of a stderr nobody will see.
	if logFile, err := os.OpenFile(filepath.Join(dataDir, "local-action.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	if !startServer(dataDir) {
		log.Printf("port %s already in use — assuming local-action is already running, opening its window instead of starting a second server", addr)
	}

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("local-action")
	w.SetSize(1280, 800, webview.HintNone)
	w.Navigate("http://" + addr)
	w.Run()
}

func appDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-action"), nil
}

// startServer binds addr and serves the app in the background. Returns
// false if the port's already taken instead of crashing — reopening the
// Dock icon while the app is already running should just show the window
// again, not error.
func startServer(dataDir string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}

	keyPath, err := secrets.DefaultKeyPath()
	if err != nil {
		log.Fatalf("resolve key path: %v", err)
	}
	key, err := secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf("load encryption key: %v", err)
	}

	database, err := db.OpenDB(filepath.Join(dataDir, "local-action.db"))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	hub := ws.NewHub()
	actBin := resolveActBin()
	engine := runs.NewEngine(database, key, actBin, hub.Broadcast, hub.Forget)
	term := terminal.NewManager()

	mux := httpapi.NewRouter(database, key, engine, hub, term, actBin)

	staticFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatalf("load embedded UI: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	go func() {
		log.Printf("local-action listening on %s", addr)
		log.Fatal(http.Serve(ln, mux))
	}()
	return true
}

// resolveActBin falls back to common Homebrew install locations. A
// Finder-launched app gets a minimal PATH (no .zshrc/.bash_profile
// sourced), so a Homebrew-installed act is often invisible to
// exec.LookPath here even though it works fine from a Terminal-launched CLI.
func resolveActBin() string {
	if _, err := exec.LookPath("act"); err == nil {
		return "act"
	}
	for _, candidate := range []string{"/opt/homebrew/bin/act", "/usr/local/bin/act"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "act"
}
