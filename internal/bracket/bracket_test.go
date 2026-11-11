package bracket

import (
	"fmt"
	"testing"
)

func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("P%d", i+1)
	}
	return out
}

// playAll ngisi semua match yang siap dimainkan; seed lebih kecil selalu menang
// (kecuali flip), sampai gak ada lagi match yang bisa diisi.
func playAll(t *testing.T, tr *Tournament, flip func(*Match) bool) {
	for guard := 0; guard < 1000; guard++ {
		b := tr.Build()
		progressed := false
		for _, m := range b.Matches {
			if m.Done || m.Hidden || m.A < 0 || m.B < 0 {
				continue
			}
			a, bb := 1, 0
			if m.A > m.B {
				a, bb = 0, 1
			}
			if flip != nil && flip(m) {
				a, bb = bb, a
			}
			if err := tr.SetResult(m.ID, a, bb); err != nil {
				t.Fatal(err)
			}
			progressed = true
		}
		if !progressed {
			if tr.Format == Swiss && len(tr.SwissPairs) < tr.Settings.SwissRounds {
				if err := tr.NextSwissRound(); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if tr.Format == Groups && tr.PlayoffSeeds == nil {
				if err := tr.StartPlayoff(); err != nil {
					t.Fatal(err)
				}
				continue
			}
			return
		}
	}
	t.Fatal("tidak selesai")
}

func TestAllFormatsFinish(t *testing.T) {
	for _, f := range []Format{Single, Double, RoundRobin, Swiss, Groups} {
		for _, n := range []int{2, 3, 5, 8, 13, 16} {
			if f == Groups && n < 4 {
				continue
			}
			s := Settings{ThirdPlace: true, GrandFinalReset: true}
			tr, err := New("x", "", f, names(n), s, false)
			if err != nil {
				t.Fatal(f, n, err)
			}
			playAll(t, tr, nil)
			places, done := tr.Podium(tr.Build())
			if !done {
				t.Fatalf("%s n=%d: belum selesai", f, n)
			}
			if f != Groups && places[0][0] != 0 {
				t.Errorf("%s n=%d: juara %v, harusnya seed 1", f, n, places[0])
			}
		}
	}
}

func TestDoubleElimEveryoneLosesTwice(t *testing.T) {
	tr, _ := New("x", "", Double, names(8), Settings{GrandFinalReset: true}, false)
	// juara lower bracket menang grand final pertama -> reset dimainkan
	playAll(t, tr, func(m *Match) bool { return m.Section.Key == "gf" && m.Round == 0 })
	b := tr.Build()
	losses := map[int]int{}
	for _, m := range b.Matches {
		if m.Done && !m.Bye && !m.Hidden {
			losses[m.Loser]++
		}
	}
	for p := 1; p < 8; p++ {
		if losses[p] != 2 {
			t.Errorf("P%d kalah %d kali, harusnya 2", p+1, losses[p])
		}
	}
	places, done := tr.Podium(b)
	if !done || places[0][0] != 0 {
		t.Fatalf("podium salah: %v %v", places, done)
	}
}

func TestSwissNoRematch(t *testing.T) {
	tr, _ := New("x", "", Swiss, names(8), Settings{SwissRounds: 3}, false)
	playAll(t, tr, nil)
	seen := map[[2]int]bool{}
	for _, r := range tr.SwissPairs {
		for _, p := range r {
			k := pairKey(p[0], p[1])
			if seen[k] {
				t.Errorf("rematch %v", p)
			}
			seen[k] = true
		}
	}
}
