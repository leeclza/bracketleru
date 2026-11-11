// Jalanin di laptop: go build -o bracketleru.exe . ; .\bracketleru.exe
package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"

	"bracketleru/internal/app"
)

func main() {
	loadDotEnv(".env")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Bracket Leru jalan di http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, app.Handler()))
}

// loadDotEnv baca file .env (KEY=VALUE per baris) buat development lokal.
// Env yang sudah di-set di terminal gak ditimpa. Di Vercel file ini gak dipakai.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set && v != "" {
			os.Setenv(k, v)
		}
	}
}
