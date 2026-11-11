// Entry point buat Vercel. Vercel nyari fungsi Handler di folder api/,
// dan vercel.json ngarahin semua URL ke sini.
package handler

import (
	"net/http"

	"bracketleru/internal/app"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	app.Handler().ServeHTTP(w, r)
}
