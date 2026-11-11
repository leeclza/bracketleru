package web

import (
	"strconv"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Gambar bracket gugur di PDF sebagai bagan bergaris (kayak di web),
// satu bagan per halaman landscape, otomatis diperkecil biar muat.

// ukuran dasar (mm) sebelum diskalakan
const (
	bBoxW     = 50.0 // lebar kartu match
	bRowH     = 7.0  // tinggi satu baris peserta
	bBoxH     = 2 * bRowH
	bScoreW   = 8.0
	bColGap   = 12.0
	bVGap     = 5.0
	bTitleH   = 8.0 // judul ronde di atas kolom
	bSecGap   = 10.0
	bSecTitle = 8.0 // judul section (Upper / Lower)
)

type bPos struct {
	x, y float64
	m    matchView
	sec  int
}

type bLayout struct {
	pos       map[int]*bPos
	order     []int // urutan gambar
	w, h      float64
	secTitles []struct {
		y    float64
		text string
	}
	roundTitles []struct {
		x, y float64
		text string
	}
}

func parseFeeds(s string) (out []struct {
	id    int
	sideB bool
}) {
	for _, f := range strings.Fields(s) {
		if len(f) < 2 {
			continue
		}
		id, err := strconv.Atoi(f[:len(f)-1])
		if err != nil {
			continue
		}
		out = append(out, struct {
			id    int
			sideB bool
		}{id, strings.HasSuffix(f, "b")})
	}
	return out
}

// layoutBoard ngitung posisi tiap match. Posisi vertikal match = rata-rata
// match sumbernya (biar garisnya rapi), kolom pertama ditumpuk berurutan.
func layoutBoard(main []*sectionView, final *sectionView) *bLayout {
	L := &bLayout{pos: map[int]*bPos{}}
	colW := bBoxW + bColGap
	maxCols := 0
	y0 := 0.0

	place := func(sv *sectionView, sec int, firstCol int, top float64) float64 {
		bottom := top
		for c, r := range sv.Rounds {
			x := float64(firstCol+c) * colW
			if r.Spacer {
				continue
			}
			L.roundTitles = append(L.roundTitles, struct {
				x, y float64
				text string
			}{x, top, r.Title})
			cursor := top + bTitleH
			for _, m := range r.Matches {
				y := cursor
				var sum float64
				n := 0
				for _, f := range parseFeeds(m.Feeds) {
					if p, ok := L.pos[f.id]; ok && (p.sec == sec || sec == -1) {
						sum += p.y
						n++
					}
				}
				if n > 0 && sum/float64(n) > y {
					y = sum / float64(n)
				}
				L.pos[m.ID] = &bPos{x: x, y: y, m: m, sec: sec}
				L.order = append(L.order, m.ID)
				cursor = y + bBoxH + bVGap
			}
			if cursor > bottom {
				bottom = cursor
			}
		}
		return bottom
	}

	for i, sv := range main {
		if len(main) > 1 || final != nil {
			L.secTitles = append(L.secTitles, struct {
				y    float64
				text string
			}{y0, sv.Title})
			y0 += bSecTitle
		}
		y0 = place(sv, i, 0, y0) + bSecGap
		if len(sv.Rounds) > maxCols {
			maxCols = len(sv.Rounds)
		}
	}
	h := y0 - bSecGap
	cols := maxCols
	if final != nil {
		// final: kolom setelah semua kolom main, posisi ikut match sumbernya
		top := 0.0
		if len(main) > 1 {
			top = bSecTitle
		}
		if b := place(final, -1, maxCols, top); b > h {
			h = b
		}
		cols += len(final.Rounds)
	}
	L.w = float64(cols)*colW - bColGap
	L.h = h
	return L
}

// drawBoard gambar satu bagan di halaman landscape baru.
func drawBoard(pdf *fpdf.Fpdf, tr func(string) string, title string, main []*sectionView, final *sectionView) {
	L := layoutBoard(main, final)
	if len(L.order) == 0 {
		return
	}
	pdf.AddPageFormat("L", pdf.GetPageSizeStr("A4"))
	pageW, pageH := pdf.GetPageSize()
	left, top, _, bottomM := pdf.GetMargins()
	_ = bottomM

	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(left, top)
	pdf.CellFormat(0, 9, tr(title), "", 1, "L", false, 0, "")

	areaX, areaY := left, top+12
	areaW, areaH := pageW-2*left, pageH-areaY-18
	s := 1.0
	if L.w*s > areaW {
		s = areaW / L.w
	}
	if L.h*s > areaH {
		s = areaH / L.h
	}
	X := func(v float64) float64 { return areaX + v*s }
	Y := func(v float64) float64 { return areaY + v*s }
	font := func(style string, size float64) {
		sz := size * s
		if sz < 3.5 {
			sz = 3.5
		}
		pdf.SetFont("Helvetica", style, sz)
	}

	// judul section & ronde
	for _, t := range L.secTitles {
		font("B", 11)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetXY(X(0), Y(t.y))
		pdf.CellFormat(L.w*s, bSecTitle*s*0.8, tr(t.text), "", 0, "L", false, 0, "")
	}
	for _, t := range L.roundTitles {
		font("B", 8)
		pdf.SetTextColor(110, 110, 110)
		pdf.SetXY(X(t.x), Y(t.y))
		pdf.CellFormat(bBoxW*s, bTitleH*s*0.7, tr(t.text), "", 0, "C", false, 0, "")
	}

	// garis penghubung dulu, kartu digambar di atasnya
	pdf.SetLineWidth(0.3 * s)
	for _, id := range L.order {
		p := L.pos[id]
		for _, f := range parseFeeds(p.m.Feeds) {
			src, ok := L.pos[f.id]
			if !ok || !(src.sec == p.sec || p.sec == -1) {
				continue
			}
			x1, y1 := src.x+bBoxW, src.y+bBoxH/2
			x2, y2 := p.x, p.y+bRowH/2
			if f.sideB {
				y2 += bRowH
			}
			mx := x2 - bColGap/2
			if src.m.Done {
				pdf.SetDrawColor(0, 0, 0)
			} else {
				pdf.SetDrawColor(170, 170, 170)
			}
			pdf.Line(X(x1), Y(y1), X(mx), Y(y1))
			pdf.Line(X(mx), Y(y1), X(mx), Y(y2))
			pdf.Line(X(mx), Y(y2), X(x2), Y(y2))
		}
	}

	for _, id := range L.order {
		p := L.pos[id]
		if p.m.Ghost {
			continue
		}
		pdf.SetDrawColor(0, 0, 0)
		pdf.SetLineWidth(0.25 * s)
		pdf.SetFillColor(255, 255, 255)
		pdf.Rect(X(p.x), Y(p.y), bBoxW*s, bBoxH*s, "FD")
		for i, sd := range []sideView{p.m.A, p.m.B} {
			ry := p.y + float64(i)*bRowH
			if i == 1 {
				pdf.Line(X(p.x), Y(ry), X(p.x+bBoxW), Y(ry))
			}
			style := ""
			pdf.SetTextColor(20, 20, 20)
			if sd.Win {
				style = "B"
				pdf.SetTextColor(0, 0, 0)
			}
			if sd.Lose || sd.Placeholder {
				pdf.SetTextColor(150, 150, 150)
			}
			font(style, 8.5)
			pdf.SetXY(X(p.x+1.5), Y(ry))
			pdf.CellFormat((bBoxW-bScoreW-2)*s, bRowH*s, fit(pdf, tr(sd.Name), (bBoxW-bScoreW-2)*s), "", 0, "L", false, 0, "")
			// kotak skor: pemenang hitam, teks putih
			pdf.SetXY(X(p.x+bBoxW-bScoreW), Y(ry))
			if sd.Win {
				pdf.SetFillColor(0, 0, 0)
				pdf.SetTextColor(255, 255, 255)
				pdf.CellFormat(bScoreW*s, bRowH*s, sd.Score, "", 0, "C", true, 0, "")
			} else {
				pdf.SetFillColor(240, 240, 240)
				pdf.CellFormat(bScoreW*s, bRowH*s, sd.Score, "", 0, "C", true, 0, "")
			}
		}
		// nomor match kecil di kiri atas kartu
		font("", 5.5)
		pdf.SetTextColor(150, 150, 150)
		pdf.SetXY(X(p.x), Y(p.y-3))
		pdf.CellFormat(10*s, 3*s, "#"+strconv.Itoa(p.m.Num), "", 0, "L", false, 0, "")
	}
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.2)
}

// fit motong nama yang kepanjangan biar gak keluar kotak.
func fit(pdf *fpdf.Fpdf, s string, w float64) string {
	if pdf.GetStringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && pdf.GetStringWidth(string(r)+"...") > w {
		r = r[:len(r)-1]
	}
	return string(r) + "..."
}
