package bracket

import (
	"errors"
	"sort"
)

type Standing struct {
	Player                 int
	Played, W, D, L        int
	ScoreFor, ScoreAgainst int
	Points                 int
	Buchholz               int // total poin lawan-lawan (tie-breaker Swiss)
}

func (s Standing) Diff() int { return s.ScoreFor - s.ScoreAgainst }

// Standings ngitung klasemen dari match yang udah selesai.
// Urutan: poin, selisih skor, skor masuk, Buchholz, lalu seed.
func (t *Tournament) Standings(matches []*Match, players []int) []Standing {
	rows := map[int]*Standing{}
	for _, p := range players {
		rows[p] = &Standing{Player: p}
	}
	opponents := map[int][]int{}

	for _, m := range matches {
		if !m.Done || m.Hidden {
			continue
		}
		if m.Bye {
			// BYE dihitung menang tanpa skor
			if r, ok := rows[m.Winner]; ok {
				r.Played++
				r.W++
				r.Points += PointsWin
			}
			continue
		}
		a, okA := rows[m.A]
		b, okB := rows[m.B]
		if !okA || !okB {
			continue
		}
		a.Played++
		b.Played++
		a.ScoreFor += m.ScoreA
		a.ScoreAgainst += m.ScoreB
		b.ScoreFor += m.ScoreB
		b.ScoreAgainst += m.ScoreA
		opponents[m.A] = append(opponents[m.A], m.B)
		opponents[m.B] = append(opponents[m.B], m.A)
		switch {
		case m.Draw:
			a.D++
			b.D++
			a.Points += PointsDraw
			b.Points += PointsDraw
		case m.Winner == m.A:
			a.W++
			b.L++
			a.Points += PointsWin
		default:
			b.W++
			a.L++
			b.Points += PointsWin
		}
	}

	out := make([]Standing, 0, len(players))
	for _, p := range players {
		r := rows[p]
		for _, o := range opponents[p] {
			r.Buchholz += rows[o].Points
		}
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if a.Diff() != b.Diff() {
			return a.Diff() > b.Diff()
		}
		if a.ScoreFor != b.ScoreFor {
			return a.ScoreFor > b.ScoreFor
		}
		if a.Buchholz != b.Buchholz {
			return a.Buchholz > b.Buchholz
		}
		return a.Player < b.Player
	})
	return out
}

// NextSwissRound bikin pairing ronde Swiss berikutnya: pemain dengan poin
// mirip diadu, dan sebisa mungkin gak ketemu lawan yang sama dua kali.
func (t *Tournament) NextSwissRound() error {
	if t.Format != Swiss {
		return errors.New("bukan turnamen Swiss")
	}
	if len(t.SwissPairs) >= t.Settings.SwissRounds {
		return errors.New("semua ronde Swiss sudah dimainkan")
	}
	b := t.Build()
	for _, m := range b.Matches {
		if !m.Done {
			return errors.New("selesaikan dulu semua pertandingan ronde ini")
		}
	}

	played := map[[2]int]bool{}
	hadBye := map[int]bool{}
	for _, round := range t.SwissPairs {
		for _, p := range round {
			if p[1] == BYE {
				hadBye[p[0]] = true
			} else {
				played[pairKey(p[0], p[1])] = true
			}
		}
	}

	all := make([]int, len(t.Participants))
	for i := range all {
		all[i] = i
	}
	var order []int
	for _, s := range t.Standings(b.Matches, all) {
		order = append(order, s.Player)
	}

	// ganjil: peringkat terbawah yang belum pernah BYE dapet BYE
	byePlayer := TBD
	if len(order)%2 == 1 {
		idx := len(order) - 1
		for i := len(order) - 1; i >= 0; i-- {
			if !hadBye[order[i]] {
				idx = i
				break
			}
		}
		byePlayer = order[idx]
		order = append(order[:idx:idx], order[idx+1:]...)
	}

	budget := 200000
	pairs, ok := pairUp(order, played, &budget)
	if !ok {
		// gak ada susunan tanpa rematch: pasangkan berurutan aja
		pairs = nil
		for i := 0; i+1 < len(order); i += 2 {
			pairs = append(pairs, [2]int{order[i], order[i+1]})
		}
	}
	if byePlayer != TBD {
		pairs = append(pairs, [2]int{byePlayer, BYE})
	}
	t.SwissPairs = append(t.SwissPairs, pairs)
	return nil
}

// pairUp nyoba masangin pemain teratas dengan lawan terdekat yang belum
// pernah dilawan (backtracking). budget ngebatesin biar gak kelamaan.
func pairUp(rem []int, played map[[2]int]bool, budget *int) ([][2]int, bool) {
	if len(rem) == 0 {
		return nil, true
	}
	a := rem[0]
	for i := 1; i < len(rem); i++ {
		if *budget--; *budget <= 0 {
			return nil, false
		}
		b := rem[i]
		if played[pairKey(a, b)] {
			continue
		}
		rest := make([]int, 0, len(rem)-2)
		rest = append(rest, rem[1:i]...)
		rest = append(rest, rem[i+1:]...)
		if p, ok := pairUp(rest, played, budget); ok {
			return append([][2]int{{a, b}}, p...), true
		}
	}
	return nil, false
}

func pairKey(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

// Podium ngembaliin juara 1, 2, dan 3 (juara 3 bisa lebih dari satu kalau
// gak ada perebutan juara 3). complete = turnamen sudah selesai.
func (t *Tournament) Podium(b *Built) (places [3][]int, complete bool) {
	all := make([]int, len(t.Participants))
	for i := range all {
		all[i] = i
	}

	switch t.Format {
	case Single:
		return elimPodium(b.Section("main"), b.Section("main3"))
	case Groups:
		if t.PlayoffSeeds == nil {
			return places, false
		}
		return elimPodium(b.Section("po"), b.Section("po3"))
	case Double:
		gfSec := b.Section("gf")
		gf := gfSec.Rounds[0].Matches[0]
		final := gf
		if len(gfSec.Rounds) > 1 {
			if gf2 := gfSec.Rounds[1].Matches[0]; !gf2.Hidden {
				final = gf2
			}
		}
		if !final.Done {
			return places, false
		}
		places[0] = []int{final.Winner}
		places[1] = []int{final.Loser}
		if lbFinal := b.Section("lb").Last(); lbFinal != nil && lbFinal.Done && lbFinal.Loser >= 0 {
			places[2] = []int{lbFinal.Loser}
		}
		return places, true
	case RoundRobin, Swiss:
		for _, m := range b.Matches {
			if !m.Done {
				return places, false
			}
		}
		if t.Format == Swiss && len(t.SwissPairs) < t.Settings.SwissRounds {
			return places, false
		}
		for i, s := range t.Standings(b.Matches, all) {
			if i > 2 {
				break
			}
			places[i] = []int{s.Player}
		}
		return places, true
	}
	return places, false
}

func elimPodium(main, third *Section) (places [3][]int, complete bool) {
	final := main.Last()
	if final == nil || !final.Done {
		return places, false
	}
	places[0] = []int{final.Winner}
	if final.Loser >= 0 {
		places[1] = []int{final.Loser}
	}
	if m := third.Last(); m != nil {
		if !m.Done {
			return places, false
		}
		if m.Winner >= 0 {
			places[2] = []int{m.Winner}
		}
	} else if len(main.Rounds) >= 2 {
		for _, sf := range main.Rounds[len(main.Rounds)-2].Matches {
			if sf.Loser >= 0 {
				places[2] = append(places[2], sf.Loser)
			}
		}
	}
	return places, true
}
