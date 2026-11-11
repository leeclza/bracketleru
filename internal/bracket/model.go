// Package bracket berisi semua logika turnamen: bikin struktur bracket,
// nerapin skor, majuin pemenang, klasemen, sama pairing Swiss.
// Package ini gak tau apa-apa soal web atau database.
package bracket

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// Nilai khusus buat slot pemain di sebuah match.
const (
	TBD = -1 // belum ketahuan (nunggu match sebelumnya)
	BYE = -2 // slot kosong, lawannya otomatis menang
)

type Format string

const (
	Single     Format = "single"
	Double     Format = "double"
	RoundRobin Format = "roundrobin"
	Swiss      Format = "swiss"
	Groups     Format = "groups"
)

var FormatNames = map[Format]string{
	Single:     "Single Elimination",
	Double:     "Double Elimination",
	RoundRobin: "Round Robin",
	Swiss:      "Swiss",
	Groups:     "Group Stage + Playoff",
}

const (
	PointsWin  = 3
	PointsDraw = 1
)

type Settings struct {
	ThirdPlace      bool `json:"thirdPlace"`      // single elim / playoff: ada perebutan juara 3
	GrandFinalReset bool `json:"gfReset"`         // double elim: grand final bisa reset
	Legs            int  `json:"legs"`            // round robin / grup: 1 = sekali ketemu, 2 = kandang-tandang
	SwissRounds     int  `json:"swissRounds"`     // jumlah ronde Swiss
	GroupCount      int  `json:"groupCount"`      // jumlah grup
	AdvancePerGroup int  `json:"advancePerGroup"` // yang lolos per grup
}

// Result itu skor yang diinput admin. A dan B dicatat supaya kalau isi
// match berubah (misal skor ronde sebelumnya diedit) hasil lama gak kepake.
type Result struct {
	A      int `json:"a"`
	B      int `json:"b"`
	ScoreA int `json:"sa"`
	ScoreB int `json:"sb"`
}

// Tournament adalah data yang disimpan ke database (sebagai JSON).
// Struktur bracket gak disimpan: selalu dihitung ulang dari data ini lewat Build().
type Tournament struct {
	ID           string         `json:"id"`
	OwnerID      string         `json:"ownerId,omitempty"`
	OwnerName    string         `json:"ownerName,omitempty"`
	AdminKey     string         `json:"adminKey,omitempty"` // lama, sudah tidak dipakai
	Name         string         `json:"name"`
	Game         string         `json:"game"`
	Format       Format         `json:"format"`
	Settings     Settings       `json:"settings"`
	Participants []string       `json:"participants"` // urutan = seed
	CreatedAt    time.Time      `json:"createdAt"`
	Results      map[int]Result `json:"results"`
	SwissPairs   [][][2]int     `json:"swissPairs,omitempty"`   // pairing tiap ronde Swiss
	Groups       [][]int        `json:"groups,omitempty"`       // index peserta per grup
	PlayoffSeeds []int          `json:"playoffSeeds,omitempty"` // diisi pas playoff dimulai
}

func (t *Tournament) PlayerName(i int) string {
	switch {
	case i == BYE:
		return "BYE"
	case i < 0 || i >= len(t.Participants):
		return "TBD"
	}
	return t.Participants[i]
}

func (t *Tournament) FormatName() string { return FormatNames[t.Format] }

// New bikin turnamen baru dan langsung nyiapin ronde pertamanya.
func New(name, game string, format Format, participants []string, s Settings, shuffle bool) (*Tournament, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Turnamen Leru"
	}
	var names []string
	for _, p := range participants {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	if len(names) < 2 {
		return nil, errors.New("minimal 2 peserta")
	}
	if len(names) > 256 {
		return nil, errors.New("maksimal 256 peserta")
	}
	if _, ok := FormatNames[format]; !ok {
		return nil, errors.New("format turnamen tidak dikenal")
	}

	if s.Legs != 2 {
		s.Legs = 1
	}
	n := len(names)
	if format == Swiss {
		if s.SwissRounds <= 0 {
			s.SwissRounds = ceilLog2(n)
		}
		s.SwissRounds = clamp(s.SwissRounds, 1, max(1, n-1))
	}
	if format == Groups {
		if s.GroupCount <= 0 {
			s.GroupCount = max(2, n/4)
		}
		if n < 4 {
			return nil, errors.New("group stage butuh minimal 4 peserta")
		}
		s.GroupCount = clamp(s.GroupCount, 2, n/2)
		if s.AdvancePerGroup <= 0 {
			s.AdvancePerGroup = 2
		}
		s.AdvancePerGroup = clamp(s.AdvancePerGroup, 1, n/s.GroupCount)
	}

	t := &Tournament{
		Name:         name,
		Game:         strings.TrimSpace(game),
		Format:       format,
		Settings:     s,
		Participants: names,
		CreatedAt:    time.Now(),
	}
	t.Reset(shuffle)
	return t, nil
}

// Reset ngapus semua skor dan nyusun ulang bracket dari awal.
func (t *Tournament) Reset(shuffle bool) {
	if shuffle {
		rand.Shuffle(len(t.Participants), func(i, j int) {
			t.Participants[i], t.Participants[j] = t.Participants[j], t.Participants[i]
		})
	}
	t.Results = map[int]Result{}
	t.SwissPairs = nil
	t.PlayoffSeeds = nil
	t.Groups = nil

	switch t.Format {
	case Swiss:
		_ = t.NextSwissRound()
	case Groups:
		// distribusi "ular": seed 1,2,3,4 ke grup A,B,C,D lalu balik D,C,B,A
		g := t.Settings.GroupCount
		t.Groups = make([][]int, g)
		for i := range t.Participants {
			row, col := i/g, i%g
			if row%2 == 1 {
				col = g - 1 - col
			}
			t.Groups[col] = append(t.Groups[col], i)
		}
	}
}

// SetResult nyimpen skor sebuah match.
func (t *Tournament) SetResult(matchID, scoreA, scoreB int) error {
	b := t.Build()
	m, ok := b.ByID[matchID]
	if !ok || m.Hidden {
		return errors.New("pertandingan tidak ditemukan")
	}
	if m.A < 0 || m.B < 0 {
		return errors.New("pemain pertandingan ini belum lengkap")
	}
	if scoreA < 0 || scoreB < 0 {
		return errors.New("skor tidak boleh negatif")
	}
	if m.Section.Elim && scoreA == scoreB {
		return errors.New("babak gugur tidak boleh seri")
	}
	t.Results[matchID] = Result{A: m.A, B: m.B, ScoreA: scoreA, ScoreB: scoreB}
	return nil
}

func (t *Tournament) ClearResult(matchID int) {
	delete(t.Results, matchID)
}

// StartPlayoff ngambil peringkat atas tiap grup lalu bikin bracket playoff.
func (t *Tournament) StartPlayoff() error {
	if t.Format != Groups {
		return errors.New("bukan turnamen group stage")
	}
	if t.PlayoffSeeds != nil {
		return errors.New("playoff sudah dimulai")
	}
	b := t.Build()
	for _, m := range b.Matches {
		if !m.Done && !m.Hidden {
			return errors.New("selesaikan dulu semua pertandingan grup")
		}
	}
	tables := make([][]Standing, len(t.Groups))
	for g, members := range t.Groups {
		tables[g] = t.Standings(b.Section(fmt.Sprintf("g%d", g)).Matches(), members)
	}
	// urutan seed: semua juara grup dulu, lalu semua runner-up, dst.
	var seeds []int
	for rank := 0; rank < t.Settings.AdvancePerGroup; rank++ {
		for _, table := range tables {
			if rank < len(table) {
				seeds = append(seeds, table[rank].Player)
			}
		}
	}
	if len(seeds) < 2 {
		return errors.New("peserta yang lolos kurang dari 2")
	}
	t.PlayoffSeeds = seeds
	return nil
}

// GroupLetter: 0 -> "A", 1 -> "B", ...
func GroupLetter(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return fmt.Sprint(i + 1)
}

func ceilLog2(n int) int {
	r := 0
	for 1<<r < n {
		r++
	}
	return max(r, 1)
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// Feeder nunjukin match sumber yang pemenangnya masuk ke sisi A atau B
// match ini. Dipakai buat gambar garis penghubung di bracket.
type Feeder struct {
	From  *Match
	SideB bool
}

func (m *Match) Feeders() []Feeder {
	if m.resetOf != nil {
		return []Feeder{{From: m.resetOf}}
	}
	var out []Feeder
	if m.inA.from != nil && !m.inA.loser {
		out = append(out, Feeder{From: m.inA.from})
	}
	if m.inB.from != nil && !m.inB.loser {
		out = append(out, Feeder{From: m.inB.from, SideB: true})
	}
	return out
}

// Edit ganti nama turnamen, game, dan nama peserta. Jumlah peserta harus
// tetap supaya struktur bracket dan skor yang udah masuk gak berubah.
func (t *Tournament) Edit(name, game string, participants []string) error {
	if name = strings.TrimSpace(name); name == "" {
		return errors.New("nama turnamen tidak boleh kosong")
	}
	var names []string
	for _, p := range participants {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	if len(names) != len(t.Participants) {
		return fmt.Errorf("jumlah peserta harus tetap %d (cuma bisa ganti nama)", len(t.Participants))
	}
	t.Name, t.Game, t.Participants = name, strings.TrimSpace(game), names
	return nil
}
