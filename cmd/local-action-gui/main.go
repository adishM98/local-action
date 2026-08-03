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
	"path/filepath"
	"runtime"
	"strings"

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

// version is set at build time via -ldflags "-X main.version=X.Y.Z" (see
// scripts/package-macos-app.sh) — "dev" for a plain `go build`, which
// internal/update treats as "never has an update" rather than comparing
// against a meaningless placeholder.
var version = "dev"

// Cocoa requires all UI work happen on the process's original OS thread.
// Go's scheduler can otherwise migrate main() off it, silently breaking
// NSApplication (the process runs, the server answers, but no window or
// Dock icon ever appears) — must lock before any of that runs.
func init() {
	runtime.LockOSThread()
}

func main() {
	fixPATH()

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
	installEditMenu()
	// Bind's callback already runs synchronously on the main thread (per
	// webview_go's own Cocoa backend — the JS message handler that invokes
	// it is called directly, no dispatch involved), so pickFolder can just
	// call NSOpenPanel's runModal directly here. Do NOT route this through
	// w.Dispatch(): dispatching to the main queue and then blocking this
	// same main thread waiting for that dispatched job is a deadlock — the
	// queued job can never run because the thread that would run it is the
	// one stuck waiting on it.
	if err := w.Bind("pickRepoFolder", pickFolder); err != nil {
		log.Printf("bind pickRepoFolder: %v", err)
	}
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

	mux := httpapi.NewRouter(database, key, engine, hub, term, actBin, version)

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

// resolveActBin prefers the act binary bundled in the app itself
// (Contents/Resources/act-bin/act, put there by scripts/package-macos-app.sh)
// over whatever's on PATH — a real user hit "act: executable file not
// found in $PATH" because they'd never installed act separately, which
// the DMG app shouldn't require at all when it can just ship its own copy.
// Falls back to plain "act" (PATH lookup, same as before) when running
// outside a real .app bundle, e.g. via `go run` in development.
func resolveActBin() string {
	exe, err := os.Executable()
	if err != nil {
		return "act"
	}
	bundled := filepath.Join(filepath.Dir(exe), "..", "Resources", "act-bin", "act")
	if info, err := os.Stat(bundled); err == nil && !info.IsDir() {
		return bundled
	}
	return "act"
}

// fixPATH prepends common Homebrew/Docker install locations to PATH. A
// Finder-launched app gets a minimal PATH (no .zshrc/.bash_profile
// sourced) that's missing wherever Docker etc. actually live — they work
// fine from a Terminal-launched CLI but are invisible here otherwise.
// act itself no longer depends on this (see resolveActBin above), but the
// Docker health check and git both still shell out by bare name.
func fixPATH() {
	path := os.Getenv("PATH")
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if !strings.Contains(path, dir) {
			path = dir + ":" + path
		}
	}
	os.Setenv("PATH", path)
}
