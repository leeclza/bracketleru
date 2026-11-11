// Package web berisi route Gin, handler, template HTML, dan export PDF.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"bracketleru/internal/bracket"
	"bracketleru/internal/store"

	"github.com/gin-gonic/gin"
)

// embed masukin folder templates & static ke dalam binary Go,
// jadi pas deploy ke Vercel gak ada file yang ketinggalan.
//
//go:embed templates static
var assets embed.FS

type server struct {
	store store.Store
	auth  *authConfig // nil kalau env OAuth belum di-set
}

// New nyusun router Gin lengkap dengan semua route.
func New(st store.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(assets, "templates/*.html"))
	r.SetHTMLTemplate(tmpl)
	static, _ := fs.Sub(assets, "static")
	r.StaticFS("/static", http.FS(static))

	s := &server{store: st, auth: loadAuthConfig()}
	r.Use(s.requireLogin)
	r.GET("/login", s.loginPage)
	r.GET("/login/google", s.login)
	r.GET("/auth/callback", s.callback)
	r.POST("/logout", s.logout)
	r.GET("/", s.home)
	r.POST("/new", s.create)
	r.POST("/preview", s.preview)

	t := r.Group("/t/:id")
	t.GET("", s.show)
	t.GET("/pdf", s.pdf)
	t.POST("/match/:mid", s.score)
	t.POST("/match/:mid/clear", s.clearScore)
	t.POST("/swiss-next", s.swissNext)
	t.POST("/playoff", s.playoff)
	t.POST("/reset", s.reset)
	t.POST("/edit", s.edit)
	t.POST("/delete", s.remove)

	r.NoRoute(func(c *gin.Context) {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"Message": "Halaman tidak ditemukan"})
	})
	return r
}

// ---------- helper ----------

func randomID(n int) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// load ngambil turnamen dari URL dan ngecek apakah yang buka adalah
// pemiliknya (user Google yang login == OwnerID).
func (s *server) load(c *gin.Context) (*bracket.Tournament, bool, bool) {
	t, err := s.store.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		msg := "Gagal memuat turnamen"
		if errors.Is(err, store.ErrNotFound) {
			status, msg = http.StatusNotFound, "Turnamen tidak ditemukan"
		}
		c.HTML(status, "error.html", gin.H{"Message": msg})
		return nil, false, false
	}
	u := s.currentUser(c)
	admin := u != nil && t.OwnerID != "" && subtle.ConstantTimeCompare([]byte(u.ID), []byte(t.OwnerID)) == 1
	return t, admin, true
}

// loadAdmin sama kayak load tapi wajib admin (dipakai semua route POST).
func (s *server) loadAdmin(c *gin.Context) (*bracket.Tournament, bool) {
	t, admin, ok := s.load(c)
	if !ok {
		return nil, false
	}
	if !admin {
		c.HTML(http.StatusForbidden, "error.html", gin.H{"Message": "Cuma pembuat turnamen yang bisa mengubah ini. Login dulu dengan akun Google yang membuatnya."})
		return nil, false
	}
	return t, true
}

func adminURL(t *bracket.Tournament) string {
	return "/t/" + t.ID
}

// back redirect ke halaman turnamen, opsional bawa pesan error dan anchor.
func back(c *gin.Context, t *bracket.Tournament, errMsg, anchor string) {
	u := adminURL(t)
	if errMsg != "" {
		u += "?err=" + url.QueryEscape(errMsg)
	}
	if anchor != "" {
		u += "#" + anchor
	}
	c.Redirect(http.StatusSeeOther, u)
}

func (s *server) save(c *gin.Context, t *bracket.Tournament) bool {
	if err := s.store.Save(c.Request.Context(), t); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Gagal menyimpan: " + err.Error()})
		return false
	}
	return true
}

// ---------- handler ----------

type mineItem struct {
	Name, Format, URL string
	Complete          bool
}

func (s *server) home(c *gin.Context) {
	u := s.currentUser(c)
	var items []mineItem
	if u != nil {
		ts, err := s.store.ListByOwner(c.Request.Context(), u.ID, 30)
		if err != nil {
			log.Println("list turnamen:", err)
		}
		for _, t := range ts {
			_, done := t.Podium(t.Build())
			items = append(items, mineItem{Name: t.Name, Format: t.FormatName(), URL: adminURL(t), Complete: done})
		}
	}
	c.HTML(http.StatusOK, "home.html", gin.H{
		"User":      u,
		"AuthReady": s.auth != nil,
		"Mine":      items,
		"Formats":   formatOptions,
		"Error":     c.Query("err"),
	})
}

type formatOption struct {
	Value, Name, Desc string
}

var formatOptions = []formatOption{
	{"single", "Single Elimination", "Kalah sekali langsung gugur"},
	{"double", "Double Elimination", "Gugur setelah kalah 2 kali"},
	{"roundrobin", "Round Robin", "Semua lawan semua, pakai klasemen"},
	{"swiss", "Swiss", "Lawan yang poinnya mirip tiap ronde"},
	{"groups", "Group Stage", "Fase grup, lalu playoff gugur"},
}

func newFromForm(c *gin.Context, shuffle bool) (*bracket.Tournament, error) {
	atoi := func(k string) int { n, _ := strconv.Atoi(c.PostForm(k)); return n }
	settings := bracket.Settings{
		ThirdPlace:      c.PostForm("thirdPlace") == "on",
		Legs:            atoi("legs"),
		SwissRounds:     atoi("swissRounds"),
		GroupCount:      atoi("groupCount"),
		AdvancePerGroup: atoi("advance"),
	}
	return bracket.New(c.PostForm("name"), c.PostForm("game"), bracket.Format(c.PostForm("format")),
		splitLines(c.PostForm("participants")), settings, shuffle)
}

func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
}

// preview dipanggil JS di halaman depan tiap form berubah dan balikin
// potongan HTML bracket (gak diacak, biar urutannya sama kayak yang diketik).
func (s *server) preview(c *gin.Context) {
	t, err := newFromForm(c, false)
	if err != nil {
		c.HTML(http.StatusOK, "preview.html", gin.H{"Error": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "preview.html", gin.H{"View": buildView(t, false)})
}

func (s *server) edit(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	if err := t.Edit(c.PostForm("name"), c.PostForm("game"), splitLines(c.PostForm("participants"))); err != nil {
		back(c, t, err.Error(), "")
		return
	}
	if s.save(c, t) {
		back(c, t, "", "")
	}
}

func (s *server) create(c *gin.Context) {
	u := s.currentUser(c)
	if u == nil {
		c.Redirect(http.StatusSeeOther, "/?err="+url.QueryEscape("Login dengan Google dulu untuk membuat turnamen"))
		return
	}
	t, err := newFromForm(c, c.PostForm("shuffle") == "on")
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/?err="+url.QueryEscape(err.Error()))
		return
	}
	t.ID = randomID(8)
	t.OwnerID = u.ID
	t.OwnerName = u.Name
	if !s.save(c, t) {
		return
	}
	c.Redirect(http.StatusSeeOther, adminURL(t))
}

func (s *server) show(c *gin.Context) {
	t, admin, ok := s.load(c)
	if !ok {
		return
	}
	c.HTML(http.StatusOK, "tournament.html", gin.H{
		"T":         t,
		"Admin":     admin,
		"User":      s.currentUser(c),
		"AuthReady": s.auth != nil,
		"View":      buildView(t, admin),
		"PublicURL": baseURL(c, s.auth) + "/t/" + t.ID,
		"Error":     c.Query("err"),
	})
}

func (s *server) score(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	mid, _ := strconv.Atoi(c.Param("mid"))
	a, errA := strconv.Atoi(c.PostForm("a"))
	b, errB := strconv.Atoi(c.PostForm("b"))
	if errA != nil || errB != nil {
		back(c, t, "Skor harus berupa angka", fmt.Sprintf("m%d", mid))
		return
	}
	if err := t.SetResult(mid, a, b); err != nil {
		back(c, t, err.Error(), fmt.Sprintf("m%d", mid))
		return
	}
	if s.save(c, t) {
		back(c, t, "", fmt.Sprintf("m%d", mid))
	}
}

func (s *server) clearScore(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	mid, _ := strconv.Atoi(c.Param("mid"))
	t.ClearResult(mid)
	if s.save(c, t) {
		back(c, t, "", fmt.Sprintf("m%d", mid))
	}
}

func (s *server) swissNext(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	if err := t.NextSwissRound(); err != nil {
		back(c, t, err.Error(), "")
		return
	}
	if s.save(c, t) {
		back(c, t, "", "")
	}
}

func (s *server) playoff(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	if err := t.StartPlayoff(); err != nil {
		back(c, t, err.Error(), "")
		return
	}
	if s.save(c, t) {
		back(c, t, "", "playoff")
	}
}

func (s *server) reset(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	t.Reset(c.PostForm("shuffle") == "on")
	if s.save(c, t) {
		back(c, t, "", "")
	}
}

func (s *server) remove(c *gin.Context) {
	t, ok := s.loadAdmin(c)
	if !ok {
		return
	}
	if err := s.store.Delete(c.Request.Context(), t.ID); err != nil {
		back(c, t, "Gagal menghapus: "+err.Error(), "")
		return
	}
	c.Redirect(http.StatusSeeOther, "/")
}
