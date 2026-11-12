package web

import (
	"fmt"
	"html/template"
	"sort"
	"strings"

	"bracketleru/internal/bracket"
)

// View model: data yang udah "matang" buat template, supaya template
// tinggal nampilin tanpa logika ribet.

type sideView struct {
	Name        string
	Score       string
	Win, Lose   bool
	Placeholder bool // TBD / BYE
}

type matchView struct {
	ID, Num        int
	A, B           sideView
	Done, Ready    bool
	Ghost          bool // match BYE: tetap makan tempat biar bracket rapi, tapi gak kelihatan
	AllowDraw      bool
	ScoreA, ScoreB int
	Admin          bool
	Action         string
	Feeds          string // "12a 13b": match sumber buat garis penghubung
}

type roundView struct {
	Title   string
	Matches []matchView
	Spacer  bool // kolom kosong biar upper sejajar sama lower
}

type sectionView struct {
	Title   string
	Bracket bool
	Rounds  []roundView
}

type rowView struct {
	Rank                         int
	Name                         string
	Played, W, D, L, SF, SA, Pts int
	Diff                         string
	Buchholz                     int
	Qualify                      bool
}

type tableView struct {
	Title    string
	Rows     []rowView
	Buchholz bool
}

type blockView struct {
	ID      string
	Title   string
	Table   *tableView
	Section *sectionView
	Board   *boardView
}

// boardView: bracket gugur dalam satu kanvas (garis penghubung digambar JS).
// Main ditumpuk vertikal (upper di atas lower), Final di kanan.
type boardView struct {
	Main  []*sectionView
	Final *sectionView
}

type podiumView struct {
	Place int
	Names string
}

type pageView struct {
	Blocks       []blockView
	Podium       []podiumView
	Complete     bool
	Progress     string // misal "12/15 pertandingan"
	Remaining    int    // peserta yang belum gugur (khusus single/double elim, 0 = gak ditampilin)
	CanSwissNext bool
	SwissInfo    string
	CanPlayoff   bool
	WaitPlayoff  bool
}

func buildView(t *bracket.Tournament, admin bool) pageView {
	b := t.Build()
	var v pageView

	// nomor match berurutan, cuma untuk match yang beneran dimainkan
	nums := map[int]int{}
	total, done := 0, 0
	for _, m := range b.Matches {
		if isBye(m) || m.Hidden {
			continue
		}
		total++
		nums[m.ID] = total
		if m.Done {
			done++
		}
	}
	v.Progress = fmt.Sprintf("%d/%d pertandingan selesai", done, total)

	// sisa peserta yang belum gugur (single: kalah 1x gugur, double: 2x)
	if lives := map[bracket.Format]int{bracket.Single: 1, bracket.Double: 2}[t.Format]; lives > 0 {
		losses := map[int]int{}
		for _, m := range b.Matches {
			if m.Done && !m.Hidden && !isBye(m) && m.Loser >= 0 {
				losses[m.Loser]++
			}
		}
		out := 0
		for _, n := range losses {
			if n >= lives {
				out++
			}
		}
		v.Remaining = len(t.Participants) - out
	}

	sectionOf := func(s *bracket.Section) *sectionView {
		sv := &sectionView{Title: s.Title, Bracket: s.Elim}
		for _, r := range s.Rounds {
			rv := roundView{Title: r.Title}
			for _, m := range r.Matches {
				if m.Hidden || (isBye(m) && !s.Elim) {
					continue
				}
				rv.Matches = append(rv.Matches, matchView{
					ID:        m.ID,
					Num:       nums[m.ID],
					A:         side(t, m, m.A, m.ScoreA),
					B:         side(t, m, m.B, m.ScoreB),
					Done:      m.Done,
					Ready:     m.A >= 0 && m.B >= 0,
					Ghost:     isBye(m),
					AllowDraw: !s.Elim,
					ScoreA:    m.ScoreA,
					ScoreB:    m.ScoreB,
					Admin:     admin,
					Action:    fmt.Sprintf("/t/%s/match/%d", t.ID, m.ID),
					Feeds:     feeds(m),
				})
			}
			if len(rv.Matches) > 0 {
				sv.Rounds = append(sv.Rounds, rv)
			}
		}
		return sv
	}

	tableOf := func(title string, matches []*bracket.Match, members []int, qualify int) *tableView {
		tv := &tableView{Title: title, Buchholz: t.Format == bracket.Swiss}
		for i, s := range t.Standings(matches, members) {
			diff := fmt.Sprint(s.Diff())
			if s.Diff() > 0 {
				diff = "+" + diff
			}
			tv.Rows = append(tv.Rows, rowView{
				Rank: i + 1, Name: t.PlayerName(s.Player),
				Played: s.Played, W: s.W, D: s.D, L: s.L,
				SF: s.ScoreFor, SA: s.ScoreAgainst, Pts: s.Points,
				Diff: diff, Buchholz: s.Buchholz, Qualify: i < qualify,
			})
		}
		return tv
	}

	switch t.Format {
	case bracket.Single:
		v.Blocks = append(v.Blocks, boardBlock("bracket", "Bracket", b, sectionOf, []string{"main"}, ""))
		if s := b.Section("main3"); s != nil {
			v.Blocks = append(v.Blocks, blockView{ID: s.Key, Title: s.Title, Section: sectionOf(s)})
		}
	case bracket.Double:
		blk := boardBlock("bracket", "Double Elimination", b, sectionOf, []string{"wb", "lb"}, "gf")
		if len(blk.Board.Main) == 2 {
			alignUpper(blk.Board.Main[0])
		}
		v.Blocks = append(v.Blocks, blk)
	case bracket.RoundRobin, bracket.Swiss:
		s := b.Sections[0]
		v.Blocks = append(v.Blocks,
			blockView{ID: "klasemen", Title: "Klasemen", Table: tableOf("Klasemen", s.Matches(), s.Members, 0)},
			blockView{ID: s.Key, Title: "Pertandingan", Section: sectionOf(s)},
		)
	case bracket.Groups:
		allDone := true
		for g := range t.Groups {
			s := b.Section(fmt.Sprintf("g%d", g))
			for _, m := range s.Matches() {
				allDone = allDone && m.Done
			}
			v.Blocks = append(v.Blocks, blockView{
				ID:      s.Key,
				Title:   s.Title,
				Table:   tableOf(s.Title, s.Matches(), s.Members, t.Settings.AdvancePerGroup),
				Section: sectionOf(s),
			})
		}
		if t.PlayoffSeeds != nil {
			v.Blocks = append(v.Blocks, boardBlock("playoff", "Playoff", b, sectionOf, []string{"po"}, ""))
			if s := b.Section("po3"); s != nil {
				v.Blocks = append(v.Blocks, blockView{ID: s.Key, Title: s.Title, Section: sectionOf(s)})
			}
		} else {
			v.CanPlayoff = allDone
			v.WaitPlayoff = !allDone
		}
	}

	if t.Format == bracket.Swiss {
		roundDone := true
		for _, m := range b.Matches {
			roundDone = roundDone && m.Done
		}
		v.SwissInfo = fmt.Sprintf("Ronde %d dari %d", len(t.SwissPairs), t.Settings.SwissRounds)
		v.CanSwissNext = roundDone && len(t.SwissPairs) < t.Settings.SwissRounds
	}

	places, complete := t.Podium(b)
	v.Complete = complete
	for i, ps := range places {
		if len(ps) == 0 {
			continue
		}
		var names []string
		for _, p := range ps {
			names = append(names, t.PlayerName(p))
		}
		v.Podium = append(v.Podium, podiumView{Place: i + 1, Names: strings.Join(names, " & ")})
	}
	if complete {
		v.Podium = append(v.Podium, restPlaces(t, b, places)...)
	}
	return v
}

// restPlaces nyusun peringkat peserta sisanya (di luar podium). Round robin /
// swiss pakai klasemen; format gugur diurutkan dari jumlah kemenangan, yang
// sama jumlahnya berbagi peringkat.
func restPlaces(t *bracket.Tournament, b *bracket.Built, podium [3][]int) []podiumView {
	in := map[int]bool{}
	next := 1
	for _, ps := range podium {
		for _, p := range ps {
			in[p] = true
		}
		if len(ps) > 0 {
			next += len(ps)
		}
	}
	var rest []int
	if t.Format == bracket.RoundRobin || t.Format == bracket.Swiss {
		all := make([]int, len(t.Participants))
		for i := range all {
			all[i] = i
		}
		for _, s := range t.Standings(b.Matches, all) {
			if !in[s.Player] {
				rest = append(rest, s.Player)
			}
		}
		var out []podiumView
		for i, p := range rest {
			out = append(out, podiumView{Place: next + i, Names: t.PlayerName(p)})
		}
		return out
	}
	wins := map[int]int{}
	for _, m := range b.Matches {
		if m.Done && !isBye(m) && m.Winner >= 0 {
			wins[m.Winner]++
		}
	}
	for p := range t.Participants {
		if !in[p] {
			rest = append(rest, p)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return wins[rest[i]] > wins[rest[j]] })
	var out []podiumView
	for i := 0; i < len(rest); {
		j := i
		var names []string
		for j < len(rest) && wins[rest[j]] == wins[rest[i]] {
			names = append(names, t.PlayerName(rest[j]))
			j++
		}
		out = append(out, podiumView{Place: next + i, Names: strings.Join(names, " & ")})
		i = j
	}
	return out
}

func boardBlock(id, title string, b *bracket.Built, sectionOf func(*bracket.Section) *sectionView, main []string, final string) blockView {
	bv := &boardView{}
	for _, k := range main {
		if s := b.Section(k); s != nil {
			bv.Main = append(bv.Main, sectionOf(s))
		}
	}
	if s := b.Section(final); final != "" && s != nil {
		bv.Final = sectionOf(s)
	}
	return blockView{ID: id, Title: title, Board: bv}
}

// feeds nulis match sumber (yang pemenangnya maju ke sini) sebagai "12a 13b".
// Match BYE dilewati karena kartunya gak ditampilin.
func feeds(m *bracket.Match) string {
	var parts []string
	for _, f := range m.Feeders() {
		if isBye(f.From) || f.From.Hidden {
			continue
		}
		s := "a"
		if f.SideB {
			s = "b"
		}
		parts = append(parts, fmt.Sprintf("%d%s", f.From.ID, s))
	}
	return strings.Join(parts, " ")
}

func side(t *bracket.Tournament, m *bracket.Match, p, score int) sideView {
	sv := sideView{Name: t.PlayerName(p), Placeholder: p < 0}
	if m.Done && !m.Bye {
		sv.Score = fmt.Sprint(score)
	}
	if m.Done && !m.Draw && p >= 0 {
		sv.Win = m.Winner == p
		sv.Lose = m.Loser == p
	}
	return sv
}

var funcs = template.FuncMap{
	// dict bikin map dari pasangan key-value, buat ngoper beberapa nilai ke template lain
	"dict": func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	},
	"medal": func(place int) string {
		if place > 3 {
			return fmt.Sprintf("#%d", place)
		}
		return [...]string{"🥇", "🥈", "🥉"}[place-1]
	},
	"formatName": func(f bracket.Format) string { return bracket.FormatNames[f] },
}

// isBye: match yang salah satu sisinya BYE gak bakal dimainkan, walaupun
// lawannya masih TBD. Kartunya dikosongin dan gak dapat nomor.
func isBye(m *bracket.Match) bool {
	return m.Bye || m.A == bracket.BYE || m.B == bracket.BYE
}

// alignUpper nyisipin kolom kosong di upper bracket supaya tiap ronde upper
// sejajar sama ronde lower tempat yang kalah turun (kayak Liquipedia):
// upper ronde r (r >= 1) ada di kolom 2r-1, jadi Upper Final sejajar Lower Final.
func alignUpper(sv *sectionView) {
	var out []roundView
	for r, rv := range sv.Rounds {
		col := 0
		if r > 0 {
			col = 2*r - 1
		}
		for len(out) < col {
			out = append(out, roundView{Spacer: true})
		}
		out = append(out, rv)
	}
	sv.Rounds = out
}
