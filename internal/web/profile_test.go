package web

import (
	"html/template"
	"io"
	"testing"
)

func TestProfileTemplate(t *testing.T) {
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(assets, "templates/*.html"))
	u := &User{ID: "1", Email: "leon.123@student.itera.ac.id", Name: "Leon", Username: "leon"}
	for _, mine := range [][]mineItem{nil, {{Name: "Leru Cup", URL: "/t/abcd", Complete: true}}} {
		if err := tmpl.ExecuteTemplate(io.Discard, "profile.html", map[string]any{"User": u, "Mine": mine}); err != nil {
			t.Fatal(err)
		}
	}
}
