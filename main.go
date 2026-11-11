// Jalanin di laptop: go run .
package main

import (
	"log"
	"net/http"
	"os"

	"bracketleru/internal/app"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Bracket Leru jalan di http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, app.Handler()))
}
