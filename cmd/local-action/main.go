package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"time"

	"local-action/internal/db"
	"local-action/internal/httpapi"
	"local-action/internal/runs"
	"local-action/internal/secrets"
	"local-action/internal/terminal"
	"local-action/internal/ws"
	"local-action/web"
)

// version is set at build time via -ldflags "-X main.version=X.Y.Z" (see
// scripts/release-macos.sh) — "dev" for a plain `go build`/`go run`, which
// internal/update treats as "never has an update" rather than comparing
// against a meaningless placeholder.
var version = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	dbPath := flag.String("db", "local-action.db", "path to sqlite database file")
	actBin := flag.String("act-bin", "act", "path to act executable")
	flag.Parse()

	keyPath, err := secrets.DefaultKeyPath()
	if err != nil {
		log.Fatalf("resolve key path: %v", err)
	}
	key, err := secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf("load encryption key: %v", err)
	}

	db, err := db.OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if n, err := runs.ReconcileOrphanedRuns(db, time.Now().Unix()); err != nil {
		log.Printf("reconcile orphaned runs: %v", err)
	} else if n > 0 {
		log.Printf("marked %d orphaned run(s) from a previous session as failed", n)
	}

	hub := ws.NewHub()
	engine := runs.NewEngine(db, key, *actBin, hub.Broadcast, hub.Forget)
	term := terminal.NewManager()

	mux := httpapi.NewRouter(db, key, engine, hub, term, *actBin, version)

	staticFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatalf("load embedded UI: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("local-action listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
