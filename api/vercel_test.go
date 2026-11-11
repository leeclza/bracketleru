package handler

import (
	"net/http/httptest"
	"testing"
)

func TestRewritePath(t *testing.T) {
	cases := map[string]int{
		"/api/index?__path=/":                      303, // ke /login
		"/api/index?__path=/login":                 200,
		"/api/index?__path=/static/style.css":      200,
		"/api/index?__path=/login&next=%2Ft%2Fabc": 200,
	}
	for u, want := range cases {
		w := httptest.NewRecorder()
		Handler(w, httptest.NewRequest("GET", u, nil))
		if w.Code != want {
			t.Errorf("%s -> %d, mau %d (Location %q)", u, w.Code, want, w.Header().Get("Location"))
		}
	}
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/api/index?__path=/", nil))
	if loc := w.Header().Get("Location"); loc != "/login?next=%2F" {
		t.Errorf("redirect ke %q", loc)
	}
}
