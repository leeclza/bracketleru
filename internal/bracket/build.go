package bracket

import "fmt"

// Match adalah satu pertandingan hasil Build(). Gak disimpan ke database.
type Match struct {
	ID      int
	Section *Section
	Round   int // index ronde di dalam section (mulai 0)

	A, B           int // index peserta, atau TBD / BYE
	ScoreA, ScoreB int
	Done           bool
	Draw           bool
	Bye            bool // salah satu sisi BYE, menang otomatis
	Hidden         bool // grand final reset yang gak kepake
	Winner, Loser  int

	inA, inB slot
	resetOf  *Match // khusus grand final reset
}

// slot menjelaskan dari mana pemain sebuah sisi match datang:
// pemain tetap, atau pemenang/yang kalah dari match lain.
type slot struct {
	player int
	from   *Match
	loser  bool
}

func fixed(p int) slot       { return slot{player: p} }
func winnerOf(m *Match) slot { return slot{from: m} }
func loserOf(m *Match) slot  { return slot{from: m, loser: true} }
func (s slot) value() int {
	if s.from == nil {
		return s.player
	}
	if !s.from.Done {
		return TBD
	}
	if s.loser {
		return s.from.Loser
	}
	return s.from.Winner
}

type Round struct {
	Title   string
	Matches []*Match
}

type Section struct {
	Key     string
	Title   string
	Elim    bool // babak gugur: gak boleh seri, ditampilin sebagai bracket
	Rounds  []*Round
	Members []int // peserta (khusus round robin / grup / swiss)
}

func (s *Section) Matches() []*Match {
	var out []*Match
	for _, r := range s.Rounds {
		out = append(out, r.Matches...)
	}
	return out
}

// Last ngembaliin match terakhir di ronde terakhir (biasanya final).
func (s *Section) Last() *Match {
	if s == nil || len(s.Rounds) == 0 {
		return nil
	}
	r := s.Rounds[len(s.Rounds)-1]
	return r.Matches[len(r.Matches)-1]
}

type Built struct {
	Matches  []*Match
	Sections []*Section
	ByID     map[int]*Match
}

func (b *Built) Section(key string) *Section {
	for _, s := range b.Sections {
		if s.Key == key {
			return s
		}
	}
	return nil
}

type builder struct {
	b      *Built
	nextID int
}

func (bd *builder) section(key, title string, elim bool) *Section {
	s := &Section{Key: key, Title: title, Elim: elim}
	bd.b.Sections = append(bd.b.Sections, s)
	return s
}

// add bikin match baru. ID-nya urut sesuai urutan pembuatan, jadi selama
// struktur turnamennya sama, ID tiap match juga selalu sama.
func (bd *builder) add(s *Section, round int, a, b slot) *Match {
	bd.nextID++
	m := &Match{ID: bd.nextID, Section: s, Round: round, inA: a, inB: b}
	for len(s.Rounds) <= round {
		s.Rounds = append(s.Rounds, &Round{})
	}
	s.Rounds[round].Matches = append(s.Rounds[round].Matches, m)
	bd.b.Matches = append(bd.b.Matches, m)
	bd.b.ByID[m.ID] = m
	return m
}

// Build nyusun semua match dari data turnamen lalu nerapin skor yang ada.
func (t *Tournament) Build() *Built {
	bd := &builder{b: &Built{ByID: map[int]*Match{}}}
	all := make([]int, len(t.Participants))
	for i := range all {
		all[i] = i
	}

	switch t.Format {
	case Single:
		bd.single("main", "Bracket", all, t.Settings.ThirdPlace)
	case Double:
		bd.double(all, t.Settings.GrandFinalReset)
	case RoundRobin:
		bd.roundRobin("rr", "Pertandingan", all, t.Settings.Legs)
	case Swiss:
		bd.swiss(t.SwissPairs, all)
	case Groups:
		for g, members := range t.Groups {
			bd.roundRobin(fmt.Sprintf("g%d", g), "Grup "+GroupLetter(g), members, t.Settings.Legs)
		}
		if t.PlayoffSeeds != nil {
			bd.single("po", "Playoff", t.PlayoffSeeds, t.Settings.ThirdPlace)
		}
	}

	bd.b.resolve(t.Results)
	return bd.b
}

// resolve jalan dari match pertama sampai terakhir. Karena match sumber
// selalu dibuat lebih dulu, pemenangnya udah ketahuan pas match berikutnya diproses.
func (b *Built) resolve(results map[int]Result) {
	for _, m := range b.Matches {
		m.Done, m.Draw, m.Bye, m.Hidden = false, false, false, false
		m.ScoreA, m.ScoreB = 0, 0
		m.Winner, m.Loser = TBD, TBD

		if g := m.resetOf; g != nil {
			// reset cuma dimainkan kalau juara lower bracket menang grand final pertama
			if !g.Done || g.Winner == g.A {
				m.A, m.B, m.Hidden = TBD, TBD, true
				continue
			}
			m.A, m.B = g.A, g.B
		} else {
			m.A, m.B = m.inA.value(), m.inB.value()
		}

		switch {
		case m.A == BYE && m.B == BYE:
			m.Done, m.Bye, m.Winner, m.Loser = true, true, BYE, BYE
		case m.A == BYE && m.B >= 0:
			m.Done, m.Bye, m.Winner, m.Loser = true, true, m.B, BYE
		case m.B == BYE && m.A >= 0:
			m.Done, m.Bye, m.Winner, m.Loser = true, true, m.A, BYE
		case m.A >= 0 && m.B >= 0:
			r, ok := results[m.ID]
			if !ok || r.A != m.A || r.B != m.B || (m.Section.Elim && r.ScoreA == r.ScoreB) {
				continue
			}
			m.Done, m.ScoreA, m.ScoreB = true, r.ScoreA, r.ScoreB
			switch {
			case r.ScoreA > r.ScoreB:
				m.Winner, m.Loser = m.A, m.B
			case r.ScoreB > r.ScoreA:
				m.Winner, m.Loser = m.B, m.A
			default:
				m.Draw = true
			}
		}
	}
}

// seedOrder ngasih urutan seed standar di ronde pertama, misal untuk 8:
// 1,8,4,5,2,7,3,6 -> seed 1 vs 8, 4 vs 5, 2 vs 7, 3 vs 6.
// Jadi seed 1 dan 2 baru bisa ketemu di final.
func seedOrder(size int) []int {
	order := []int{1, 2}
	for len(order) < size {
		sum := len(order)*2 + 1
		next := make([]int, 0, len(order)*2)
		for _, s := range order {
			next = append(next, s, sum-s)
		}
		order = next
	}
	return order
}

// elim bikin bracket gugur dan ngembaliin match per ronde.
// Jumlah slot dibulatkan ke pangkat 2; sisanya diisi BYE.
func (bd *builder) elim(s *Section, players []int) [][]*Match {
	size := 2
	for size < len(players) {
		size *= 2
	}
	seat := func(seed int) int {
		if seed <= len(players) {
			return players[seed-1]
		}
		return BYE
	}
	order := seedOrder(size)

	var rounds [][]*Match
	var cur []*Match
	for i := 0; i < size/2; i++ {
		cur = append(cur, bd.add(s, 0, fixed(seat(order[2*i])), fixed(seat(order[2*i+1]))))
	}
	rounds = append(rounds, cur)
	for r := 1; len(cur) > 1; r++ {
		var next []*Match
		for i := 0; i < len(cur)/2; i++ {
			next = append(next, bd.add(s, r, winnerOf(cur[2*i]), winnerOf(cur[2*i+1])))
		}
		rounds = append(rounds, next)
		cur = next
	}
	return rounds
}

func elimTitles(s *Section, prefix string) {
	n := len(s.Rounds)
	for i, r := range s.Rounds {
		switch n - 1 - i {
		case 0:
			r.Title = prefix + "Final"
		case 1:
			r.Title = prefix + "Semifinal"
		case 2:
			r.Title = prefix + "Perempat Final"
		default:
			r.Title = fmt.Sprintf("%sRonde %d", prefix, i+1)
		}
	}
}

func (bd *builder) single(key, title string, players []int, thirdPlace bool) {
	s := bd.section(key, title, true)
	rounds := bd.elim(s, players)
	elimTitles(s, "")
	if thirdPlace && len(rounds) >= 2 {
		semis := rounds[len(rounds)-2]
		t := bd.section(key+"3", "Perebutan Juara 3", true)
		bd.add(t, 0, loserOf(semis[0]), loserOf(semis[1]))
		t.Rounds[0].Title = "Juara 3"
	}
}

// double bikin upper bracket, lower bracket, dan grand final.
//
// Lower bracket selang-seling antara ronde "drop" (pemenang lower lawan
// yang baru kalah di upper) dan ronde "konsolidasi" (sesama pemenang lower).
func (bd *builder) double(players []int, reset bool) {
	wbSec := bd.section("wb", "Upper Bracket", true)
	wb := bd.elim(wbSec, players)
	elimTitles(wbSec, "Upper ")
	k := len(wb)

	lbChampion := loserOf(wb[0][0]) // kalau cuma 2 pemain, gak ada lower bracket
	if k >= 2 {
		lbSec := bd.section("lb", "Lower Bracket", true)
		round := 0
		var prev []*Match
		r1 := wb[0]
		for i := 0; i < len(r1)/2; i++ {
			prev = append(prev, bd.add(lbSec, round, loserOf(r1[2*i]), loserOf(r1[2*i+1])))
		}
		for j := 1; j < k; j++ {
			// ronde drop: yang kalah di upper ronde j+1 turun ke sini.
			// Urutannya dibalik tiap ronde genap supaya gak cepat rematch.
			round++
			drops := wb[j]
			var cur []*Match
			for i := range prev {
				d := drops[i]
				if j%2 == 0 {
					d = drops[len(drops)-1-i]
				}
				cur = append(cur, bd.add(lbSec, round, winnerOf(prev[i]), loserOf(d)))
			}
			prev = cur
			if j < k-1 {
				round++
				var next []*Match
				for i := 0; i < len(prev)/2; i++ {
					next = append(next, bd.add(lbSec, round, winnerOf(prev[2*i]), winnerOf(prev[2*i+1])))
				}
				prev = next
			}
		}
		for i, r := range lbSec.Rounds {
			r.Title = fmt.Sprintf("Lower Ronde %d", i+1)
		}
		lbSec.Rounds[len(lbSec.Rounds)-1].Title = "Lower Final"
		lbChampion = winnerOf(prev[0])
	}

	gfSec := bd.section("gf", "Grand Final", true)
	gf := bd.add(gfSec, 0, winnerOf(wb[k-1][0]), lbChampion)
	gfSec.Rounds[0].Title = "Grand Final"
	if reset {
		gf2 := bd.add(gfSec, 1, slot{}, slot{})
		gf2.resetOf = gf
		gfSec.Rounds[1].Title = "Grand Final Reset"
	}
}

// roundRobin pakai metode lingkaran: satu pemain diam, sisanya berputar,
// jadi tiap ronde semua orang main sekali.
func (bd *builder) roundRobin(key, title string, players []int, legs int) {
	s := bd.section(key, title, false)
	s.Members = players
	ps := append([]int(nil), players...)
	if len(ps)%2 == 1 {
		ps = append(ps, BYE)
	}
	n := len(ps)
	var rounds [][][2]int
	for r := 0; r < n-1; r++ {
		var pairs [][2]int
		for i := 0; i < n/2; i++ {
			a, b := ps[i], ps[n-1-i]
			if a == BYE || b == BYE {
				continue
			}
			if i == 0 && r%2 == 1 {
				a, b = b, a
			}
			pairs = append(pairs, [2]int{a, b})
		}
		rounds = append(rounds, pairs)
		// putar semua kecuali index 0
		ps = append([]int{ps[0], ps[n-1]}, ps[1:n-1]...)
	}
	if legs == 2 {
		first := len(rounds)
		for r := 0; r < first; r++ {
			var pairs [][2]int
			for _, p := range rounds[r] {
				pairs = append(pairs, [2]int{p[1], p[0]})
			}
			rounds = append(rounds, pairs)
		}
	}
	for r, pairs := range rounds {
		for _, p := range pairs {
			bd.add(s, r, fixed(p[0]), fixed(p[1]))
		}
		s.Rounds[r].Title = fmt.Sprintf("Ronde %d", r+1)
	}
}

func (bd *builder) swiss(pairs [][][2]int, players []int) {
	s := bd.section("swiss", "Swiss", false)
	s.Members = players
	for r, round := range pairs {
		for _, p := range round {
			bd.add(s, r, fixed(p[0]), fixed(p[1]))
		}
		s.Rounds[r].Title = fmt.Sprintf("Ronde %d", r+1)
	}
}
