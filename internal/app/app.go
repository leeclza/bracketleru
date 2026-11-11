// Package app nyambungin semuanya: milih tempat simpan data dari
// environment variable, lalu bikin router web. Dipakai main.go (lokal)
// dan api/index.go (Vercel).
package app

import (
	"log"
	"net/http"
	"os"
	"strings"
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
		handler = restorePath(web.New(st))
	})
	return handler
}

// restorePath: rewrite di vercel.json ngirim semua request ke /api/index?__path=/asli.
// Di sini path aslinya dibalikin sebelum masuk router, jadi jalan baik lewat
// api/index.go maupun waktu Vercel jalanin main.go sebagai backend framework.
func restorePath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if p := q.Get("__path"); p != "" && strings.HasPrefix(p, "/") {
			q.Del("__path")
			r.URL.Path = p
			r.URL.RawPath = ""
			r.URL.RawQuery = q.Encode()
			r.RequestURI = r.URL.RequestURI()
		}
		next.ServeHTTP(w, r)
	})
}

// Kalau DATABASE_URL di-set -> pakai Postgres (Neon).
// Kalau enggak -> simpan file JSON di folder ./data (cocok buat lokal).
func newStore() (store.Store, error) {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		log.Println("penyimpanan: Postgres")
		return store.NewSQLStore(dsn)
	}
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		dir = "data"
		if os.Getenv("VERCEL") != "" {
			// di Vercel cuma /tmp yang bisa ditulis, dan isinya bisa hilang kapan aja
			dir = "/tmp/bracketleru"
			log.Println("PERINGATAN: DATABASE_URL belum di-set, data tidak akan awet")
		}
	}
	log.Println("penyimpanan: folder", dir)
	return store.NewFileStore(dir)
}
