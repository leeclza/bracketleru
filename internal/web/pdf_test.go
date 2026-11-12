package web

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"bracketleru/internal/bracket"
	"bracketleru/internal/store"
)

// PDF harus tetap di bawah 10 KB (16 peserta, semua format).
func TestPDFSize(t *testing.T) {
	st, _ := store.NewFileStore(t.TempDir())
	t.Setenv("GOOGLE_CLIENT_ID", "x")
	t.Setenv("GOOGLE_CLIENT_SECRET", "x")
	t.Setenv("SESSION_SECRET", strings.Repeat("k", 40))
	r := New(st)
	for fi, f := range []bracket.Format{bracket.Single, bracket.Double, bracket.RoundRobin, bracket.Groups} {
		var names []string
		for i := 0; i < 16; i++ {
			names = append(names, fmt.Sprintf("Pemain Nomor %d", i+1))
		}
		tr, _ := bracket.New("Turnamen Tes", "Valorant", f, names, bracket.Settings{SwissRounds: 3}, false)
		tr.ID, tr.OwnerID = fmt.Sprintf("pdft%04d", fi), "u1"
		for k := 0; k < 40; k++ {
			for _, m := range tr.Build().Matches {
				if !m.Done && m.A >= 0 && m.B >= 0 {
					tr.SetResult(m.ID, 2, 1)
				}
			}
		}
		st.Save(context.Background(), tr)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/t/"+tr.ID+"/pdf", nil))
		if w.Code != 200 || w.Body.Len() > 10*1024 {
			t.Errorf("format %d: status %d, ukuran %d byte (maks 10 KB)", fi, w.Code, w.Body.Len())
		}
	}
}
