package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bracketleru/internal/bracket"
	"bracketleru/internal/store"
)

func TestSwissNextHTTP(t *testing.T) {
	st, _ := store.NewFileStore(t.TempDir())
	t.Setenv("GOOGLE_CLIENT_ID", "x")
	t.Setenv("GOOGLE_CLIENT_SECRET", "x")
	t.Setenv("SESSION_SECRET", strings.Repeat("k", 40))
	t.Setenv("ALLOWED_EMAIL_DOMAINS", "")
	r := New(st)
	cfg := loadAuthConfig()
	u := User{ID: "u1", Email: "a@student.itera.ac.id", Name: "A", Username: "leru", Exp: time.Now().Add(time.Hour).Unix()}
	p, _ := json.Marshal(u)
	cookie := &http.Cookie{Name: sessionCookie, Value: cfg.sign(p)}

	tr, _ := bracket.New("x", "", bracket.Swiss, []string{"Andi", "Budi", "Bolang", "Jamal", "Laper"}, bracket.Settings{SwissRounds: 3}, false)
	tr.ID, tr.OwnerID = "swis1234", "u1"
	for _, m := range tr.Build().Matches {
		if !m.Done && m.A >= 0 && m.B >= 0 {
			tr.SetResult(m.ID, 2, 1)
		}
	}
	st.Save(context.Background(), tr)

	req := httptest.NewRequest("POST", "/t/swis1234/swiss-next", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	t.Logf("status=%d location=%q body=%.200s", w.Code, w.Header().Get("Location"), w.Body.String())
	got, _ := st.Get(context.Background(), "swis1234")
	if len(got.SwissPairs) != 2 {
		t.Fatalf("ronde sekarang %d, mau 2", len(got.SwissPairs))
	}
}
