// Entry point buat Vercel. Vercel nyari fungsi Handler di folder api/,
// dan vercel.json ngarahin semua URL ke sini.
package handler

import (
	"net/http"

	"bracketleru/internal/app"
)

// Karena rewrite di vercel.json, path yang sampai ke sini jadi "/api/index".
// Path aslinya dititipin lewat query __path, jadi dibalikin dulu sebelum
// diterusin ke router Gin.
func Handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if p := q.Get("__path"); p != "" {
		q.Del("__path")
		r.URL.Path = p
		r.URL.RawPath = ""
		r.URL.RawQuery = q.Encode()
		r.RequestURI = r.URL.RequestURI()
	}
	app.Handler().ServeHTTP(w, r)
}
