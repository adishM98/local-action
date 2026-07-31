package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"

	"local-action/internal/app"
	"local-action/internal/db"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	dbPath := flag.String("db", "local-action.db", "path to sqlite database file")
	actBin := flag.String("act-bin", "act", "path to act executable")
	flag.Parse()

	keyPath, err := app.DefaultKeyPath()
	if err != nil {
		log.Fatalf("resolve key path: %v", err)
	}
	key, err := app.LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf("load encryption key: %v", err)
	}

	db, err := db.OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	hub := app.NewHub()
	engine := app.NewEngine(db, key, *actBin, hub.Broadcast, hub.Forget)

	mux := app.NewRouter(db, key, engine, hub, *actBin)

	staticFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("load embedded UI: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("local-action listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
