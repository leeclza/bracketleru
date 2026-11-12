package web

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"bracketleru/internal/bracket"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
)

// pdf bikin file PDF hasil turnamen: podium, klasemen, dan semua skor.
// Bisa diunduh siapa aja yang punya link turnamennya.
func (s *server) pdf(c *gin.Context) {
	t, _, ok := s.load(c)
	if !ok {
		return
	}
	v := buildView(t, false)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	tr := pdf.UnicodeTranslatorFromDescriptor("") // biar huruf kayak é tetap kebaca
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, 6, fmt.Sprintf("Bracket Leru  -  halaman %d", pdf.PageNo()), "", 0, "C", false, 0, "")
		pdf.SetY(-7)
		pdf.CellFormat(0, 4, tr("Made by leeclza · © 2026 · All rights reserved"), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()

	// judul
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetTextColor(0, 0, 0)
	pdf.MultiCell(0, 10, tr(t.Name), "", "C", false)
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(0, 0, 0)
	info := []string{t.FormatName(), fmt.Sprintf("%d peserta", len(t.Participants)), t.CreatedAt.Format("02 Jan 2006")}
	if t.Game != "" {
		info = append([]string{t.Game}, info...)
	}
	pdf.CellFormat(0, 7, tr(strings.Join(info, "  |  ")), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	// podium
	heading(pdf, "Hasil Akhir")
	if !v.Complete {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, 7, tr("Turnamen belum selesai - hasil di bawah adalah sementara ("+v.Progress+")."), "", 1, "L", false, 0, "")
		pdf.Ln(1)
	}
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.3)
	for _, p := range v.Podium {
		h := 9.0
		if p.Place <= 3 {
			// juara 1-3: label hitam teks putih
			pdf.SetFillColor(0, 0, 0)
			pdf.SetTextColor(255, 255, 255)
			pdf.SetFont("Helvetica", "B", 11)
			pdf.CellFormat(28, h, fmt.Sprintf("Juara %d", p.Place), "1", 0, "C", true, 0, "")
			pdf.SetFont("Helvetica", "B", 13)
		} else {
			// peringkat lain: label bergaris tepi, lebih kecil
			h = 7
			pdf.SetTextColor(0, 0, 0)
			pdf.SetFont("Helvetica", "B", 10)
			pdf.CellFormat(28, h, fmt.Sprintf("#%d", p.Place), "1", 0, "C", false, 0, "")
			pdf.SetFont("Helvetica", "", 11)
		}
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, h, "  "+tr(p.Names), "", 1, "L", false, 0, "")
		pdf.Ln(1.5)
	}
	if len(v.Podium) == 0 {
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, 7, "Belum ada juara.", "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	for _, b := range v.Blocks {
		if b.Table != nil {
			heading(pdf, b.Title+" - Klasemen")
			standingsTable(pdf, tr, b.Table)
		}
		if b.Section != nil {
			if b.Section.Bracket {
				// bracket gugur (mis. playoff): gambar bagan
				drawBoard(pdf, tr, b.Title, []*sectionView{b.Section}, nil)
			} else {
				// round robin / swiss / grup: daftar pertandingan
				heading(pdf, b.Title)
				matchList(pdf, tr, b.Section)
			}
		}
		if b.Board != nil {
			drawBoard(pdf, tr, b.Title, b.Board.Main, b.Board.Final)
		}
	}

	data := &strings.Builder{}
	if err := pdf.Output(data); err != nil {
		c.String(http.StatusInternalServerError, "gagal bikin PDF: %v", err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, fileName(t)))
	c.Data(http.StatusOK, "application/pdf", []byte(data.String()))
}

func heading(pdf *fpdf.Fpdf, text string) {
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, 8, text, "B", 1, "L", false, 0, "")
	pdf.Ln(2)
}

func standingsTable(pdf *fpdf.Fpdf, tr func(string) string, tv *tableView) {
	cols := []string{"#", "Nama", "M", "W", "D", "L", "Skor", "+/-", "Poin"}
	widths := []float64{10, 62, 13, 13, 13, 13, 20, 16, 20}
	if tv.Buchholz {
		cols = append(cols, "Buch")
		widths[1] -= 14
		widths = append(widths, 14)
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(0, 0, 0) // sama dgn warna teks: fpdf gak perlu nyetel warna ulang tiap sel
	pdf.SetTextColor(0, 0, 0)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.3)
	for i, h := range cols {
		align := "C"
		if i == 1 {
			align = "L"
		}
		pdf.CellFormat(widths[i], 7, h, "B", 0, align, false, 0, "")
	}
	pdf.Ln(-1)
	for _, r := range tv.Rows {
		// yang lolos ditebelin (tanpa warna latar biar tajam di printer)
		if r.Qualify {
			pdf.SetFont("Helvetica", "B", 9)
		} else {
			pdf.SetFont("Helvetica", "", 9)
		}
		vals := []string{fmt.Sprint(r.Rank), tr(r.Name), fmt.Sprint(r.Played), fmt.Sprint(r.W), fmt.Sprint(r.D),
			fmt.Sprint(r.L), fmt.Sprintf("%d-%d", r.SF, r.SA), r.Diff, fmt.Sprint(r.Pts)}
		if tv.Buchholz {
			vals = append(vals, fmt.Sprint(r.Buchholz))
		}
		for j, val := range vals {
			align := "C"
			if j == 1 {
				align = "L"
			}
			pdf.CellFormat(widths[j], 6.5, val, "", 0, align, false, 0, "")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(3)
}

func matchList(pdf *fpdf.Fpdf, tr func(string) string, sv *sectionView) {
	for _, r := range sv.Rounds {
		var rows []matchView
		for _, m := range r.Matches {
			if !m.Ghost {
				rows = append(rows, m)
			}
		}
		if len(rows) == 0 {
			continue
		}
		pdf.SetFillColor(0, 0, 0) // sama dgn warna teks: hemat ukuran file
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 7, tr(r.Title), "", 1, "L", false, 0, "")
		for _, m := range rows {
			score := "vs"
			if m.Done {
				score = fmt.Sprintf("%d - %d", m.ScoreA, m.ScoreB)
			}
			pdf.SetFont("Helvetica", "", 10)
			sideCell(pdf, tr, m.A, 76, "R")
			pdf.SetFont("Helvetica", "B", 10)
			pdf.CellFormat(28, 6.5, score, "", 0, "C", false, 0, "")
			sideCell(pdf, tr, m.B, 76, "L")
			pdf.Ln(-1)
		}
		pdf.Ln(2)
	}
	pdf.Ln(2)
}

func sideCell(pdf *fpdf.Fpdf, tr func(string) string, s sideView, w float64, align string) {
	// semua hitam pekat (abu-abu jadi buram di printer); pemenang tebal,
	// yang belum ketahuan (placeholder) miring
	style := ""
	if s.Win {
		style = "B"
	} else if s.Placeholder {
		style = "I"
	}
	pdf.SetFont("Helvetica", style, 10)
	pdf.CellFormat(w, 6.5, tr(s.Name), "", 0, align, false, 0, "")
}

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func fileName(t *bracket.Tournament) string {
	name := strings.Trim(unsafeChars.ReplaceAllString(strings.ToLower(t.Name), "-"), "-")
	if name == "" {
		name = "bracket"
	}
	return name + "-hasil"
}
