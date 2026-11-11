// Package app nyambungin semuanya: milih tempat simpan data dari
// environment variable, lalu bikin router web. Dipakai main.go (lokal)
// dan api/index.go (Vercel).
package app

import (
	"log"
	"net/http"
	"os"
	"sync"

	"bracketleru/internal/store"
	"bracketleru/internal/web"
)

var (
	once    sync.Once
	handler http.Handler
)

// Handler ngembaliin router yang sama terus (dibikin sekali aja).
func Handler() http.Handler {
	once.Do(func() {
		st, err := newStore()
		if err != nil {
			log.Fatalf("gagal menyiapkan penyimpanan: %v", err)
		}
		handler = web.New(st)
	})
	return handler
}

// Kalau TURSO_DATABASE_URL di-set -> pakai Turso.
// Kalau enggak -> simpan file JSON di folder ./data (cocok buat lokal).
func newStore() (store.Store, error) {
	if url := os.Getenv("TURSO_DATABASE_URL"); url != "" {
		log.Println("penyimpanan: Turso")
		return store.NewSQLStore(url, os.Getenv("TURSO_AUTH_TOKEN"))
	}
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "data"
		if os.Getenv("VERCEL") != "" {
			// di Vercel cuma /tmp yang bisa ditulis, dan isinya bisa hilang kapan aja
			dir = "/tmp/bracketleru"
			log.Println("PERINGATAN: TURSO_DATABASE_URL belum di-set, data tidak akan awet")
		}
	}
	log.Println("penyimpanan: folder", dir)
	return store.NewFileStore(dir)
}
