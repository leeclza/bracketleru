package web

// Login pakai Google OAuth 2.0 (authorization code flow), ditulis langsung
// pakai net/http biar gak nambah dependency.
//
// Sesi disimpan di cookie yang ditandatangani HMAC-SHA256 (SESSION_SECRET),
// jadi gak butuh tabel sesi dan tetap jalan di Vercel serverless.
//
// Env yang dibutuhkan:
//   GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET  -> dari Google Cloud Console
//   SESSION_SECRET                          -> string acak panjang (min 32 karakter)
//   BASE_URL (opsional)                     -> mis. https://bracketleru.vercel.app
//                                              kalau kosong diambil dari request

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"bracketleru/internal/store"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookie = "bl_session"
	stateCookie   = "bl_oauth_state"
	sessionTTL    = 7 * 24 * time.Hour // sesi hangus kalau 7 hari gak buka web
)

type User struct {
	ID       string `json:"sub"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"u,omitempty"` // kosong = belum isi username
	Exp      int64  `json:"exp"`
}

type authConfig struct {
	clientID, clientSecret, baseURL string
	secret                          []byte
	domains                         []string // domain email yang boleh login
}

// Default cuma email mahasiswa ITERA. Bisa diganti lewat env
// ALLOWED_EMAIL_DOMAINS (dipisah koma).
const defaultAllowedDomains = "student.itera.ac.id"

// emailAllowed: domain email harus PERSIS salah satu domain yang diizinkan
// (subdomain lain / itera.ac.id biasa ditolak).
func (a *authConfig) emailAllowed(email string) bool {
	_, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || domain == "" {
		return false
	}
	for _, d := range a.domains {
		if domain == d {
			return true
		}
	}
	return false
}

func loadAuthConfig() *authConfig {
	cfg := &authConfig{
		clientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		clientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		baseURL:      strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		secret:       []byte(os.Getenv("SESSION_SECRET")),
	}
	raw := os.Getenv("ALLOWED_EMAIL_DOMAINS")
	if raw == "" {
		raw = defaultAllowedDomains
	}
	for _, d := range strings.Split(raw, ",") {
		if d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@")); d != "" {
			cfg.domains = append(cfg.domains, d)
		}
	}
	if cfg.clientID == "" || cfg.clientSecret == "" || len(cfg.secret) < 32 {
		log.Println("PERINGATAN: GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET / SESSION_SECRET (min 32 karakter) belum di-set, login tidak akan jalan")
		return nil
	}
	return cfg
}

func baseURL(c *gin.Context, cfg *authConfig) string {
	if cfg != nil && cfg.baseURL != "" {
		return cfg.baseURL
	}
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + c.Request.Host
}

func secureCookie(c *gin.Context) bool {
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}

// ---------- cookie bertanda tangan ----------

func (a *authConfig) sign(payload []byte) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(mac.Sum(nil))
}

func (a *authConfig) verify(v string) ([]byte, bool) {
	p, s, ok := strings.Cut(v, ".")
	if !ok {
		return nil, false
	}
	enc := base64.RawURLEncoding
	payload, err1 := enc.DecodeString(p)
	sig, err2 := enc.DecodeString(s)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	return payload, hmac.Equal(sig, mac.Sum(nil))
}

// currentUser balikin user yang login, atau nil.
func (s *server) currentUser(c *gin.Context) *User {
	if s.auth == nil {
		return nil
	}
	v, err := c.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	payload, ok := s.auth.verify(v)
	if !ok {
		return nil
	}
	var u User
	if json.Unmarshal(payload, &u) != nil || u.ID == "" || time.Now().Unix() > u.Exp || !s.auth.emailAllowed(u.Email) {
		return nil
	}
	// sliding session: tiap buka web (maks sekali sehari) masa berlakunya diperpanjang lagi 7 hari
	if time.Until(time.Unix(u.Exp, 0)) < sessionTTL-24*time.Hour {
		s.setSession(c, &u)
	}
	return &u
}

// ---------- handler ----------

func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/login") {
		return "/"
	}
	return next
}

// requireLogin: semua halaman wajib login, kecuali halaman login itu sendiri,
// file statis, dan link tonton (GET /t/:id & PDF-nya) biar penonton gak perlu akun.
func (s *server) requireLogin(c *gin.Context) {
	p := c.Request.URL.Path
	public := p == "/login" || strings.HasPrefix(p, "/login/") || p == "/auth/callback" ||
		strings.HasPrefix(p, "/static/") || p == "/favicon.ico"
	if c.Request.Method == http.MethodGet && strings.HasPrefix(p, "/t/") {
		rest := strings.TrimPrefix(p, "/t/")
		if !strings.Contains(rest, "/") || strings.HasSuffix(rest, "/pdf") {
			public = true
		}
	}
	u := s.currentUser(c)
	if u != nil && u.Username == "" && !public && p != "/username" && p != "/logout" {
		// sudah login tapi belum isi username -> wajib isi dulu
		c.Redirect(http.StatusSeeOther, "/username?next="+url.QueryEscape(c.Request.URL.RequestURI()))
		c.Abort()
		return
	}
	if public || u != nil {
		c.Next()
		return
	}
	if c.Request.Method == http.MethodGet {
		c.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(c.Request.URL.RequestURI()))
	} else {
		c.Redirect(http.StatusSeeOther, "/login")
	}
	c.Abort()
}

// loginPage nampilin halaman login (tombol "Login dengan Google").
func (s *server) loginPage(c *gin.Context) {
	next := safeNext(c.Query("next"))
	if s.currentUser(c) != nil {
		c.Redirect(http.StatusSeeOther, next)
		return
	}
	c.HTML(http.StatusOK, "login.html", gin.H{
		"AuthReady": s.auth != nil,
		"Next":      next,
		"Error":     c.Query("err"),
	})
}

func (s *server) login(c *gin.Context) {
	if s.auth == nil {
		c.HTML(http.StatusServiceUnavailable, "error.html", gin.H{"Message": "Login belum dikonfigurasi di server"})
		return
	}
	state := randomID(32)
	next := safeNext(c.Query("next"))
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(stateCookie, state+"|"+next, 600, "/", "", secureCookie(c), true)

	q := url.Values{
		"client_id":     {s.auth.clientID},
		"redirect_uri":  {baseURL(c, s.auth) + "/auth/callback"},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"hd":            {s.auth.domains[0]}, // hint ke Google: tampilkan akun domain ini
		"state":         {state},
		"prompt":        {"select_account"},
	}
	c.Redirect(http.StatusFound, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode())
}

func (s *server) callback(c *gin.Context) {
	if s.auth == nil {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	fail := func(msg string) {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"Message": "Login gagal: " + msg})
	}
	raw, _ := c.Cookie(stateCookie)
	c.SetCookie(stateCookie, "", -1, "/", "", secureCookie(c), true)
	want, next, _ := strings.Cut(raw, "|")
	got := c.Query("state")
	if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		fail("sesi login kedaluwarsa, coba lagi")
		return
	}
	if e := c.Query("error"); e != "" {
		fail(e)
		return
	}
	u, err := s.auth.exchange(c.Request.Context(), c.Query("code"), baseURL(c, s.auth)+"/auth/callback")
	if err != nil {
		log.Println("oauth:", err)
		fail("tidak bisa verifikasi akun Google")
		return
	}
	if !s.auth.emailAllowed(u.Email) {
		log.Println("login ditolak, domain tidak diizinkan:", u.Email)
		c.Redirect(http.StatusSeeOther, "/login?err="+url.QueryEscape("Hanya email @student.itera.ac.id yang bisa login. Akun "+u.Email+" ditolak."))
		return
	}
	name, err := s.store.GetUsername(c.Request.Context(), u.ID)
	if err != nil {
		log.Println("ambil username:", err)
		fail("gagal memuat akun, coba lagi")
		return
	}
	u.Username = name
	s.setSession(c, u)
	c.Redirect(http.StatusSeeOther, safeNext(next)) // requireLogin yang ngarahin ke /username kalau kosong
}

func (s *server) setSession(c *gin.Context, u *User) {
	u.Exp = time.Now().Add(sessionTTL).Unix()
	payload, _ := json.Marshal(u)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, s.auth.sign(payload), int(sessionTTL.Seconds()), "/", "", secureCookie(c), true)
}

var validUsername = regexp.MustCompile(`^[a-z0-9_.]{3,20}$`)

// usernamePage: form wajib isi username setelah login pertama.
func (s *server) usernamePage(c *gin.Context) {
	u := s.currentUser(c)
	if u == nil {
		c.Redirect(http.StatusSeeOther, "/login")
		return
	}
	next := safeNext(c.Query("next"))
	if c.Request.Method == http.MethodGet {
		if u.Username != "" {
			c.Redirect(http.StatusSeeOther, next)
			return
		}
		c.HTML(http.StatusOK, "username.html", gin.H{"User": u, "Next": next})
		return
	}
	next = safeNext(c.PostForm("next"))
	name := strings.ToLower(strings.TrimSpace(c.PostForm("username")))
	show := func(msg string) {
		c.HTML(http.StatusOK, "username.html", gin.H{"User": u, "Next": next, "Value": name, "Error": msg})
	}
	if !validUsername.MatchString(name) {
		show("Username 3-20 karakter, hanya huruf kecil, angka, titik, dan underscore.")
		return
	}
	err := s.store.SetUsername(c.Request.Context(), u.ID, u.Email, name)
	if errors.Is(err, store.ErrUsernameTaken) {
		show("Username @" + name + " sudah dipakai, coba yang lain.")
		return
	}
	if err != nil {
		log.Println("simpan username:", err)
		show("Gagal menyimpan, coba lagi.")
		return
	}
	u.Username = name
	s.setSession(c, u)
	c.Redirect(http.StatusSeeOther, next)
}

func (s *server) logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", secureCookie(c), true)
	c.Redirect(http.StatusSeeOther, "/")
}

// exchange nuker authorization code jadi access token, lalu ambil profil user.
func (a *authConfig) exchange(ctx context.Context, code, redirect string) (*User, error) {
	if code == "" {
		return nil, errors.New("code kosong")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	form := url.Values{
		"code":          {code},
		"client_id":     {a.clientID},
		"client_secret": {a.clientSecret},
		"redirect_uri":  {redirect},
		"grant_type":    {"authorization_code"},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint: status %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		return nil, errors.New("access token tidak valid")
	}

	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	res, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo: status %d", res.StatusCode)
	}
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil || info.Sub == "" {
		return nil, errors.New("profil tidak valid")
	}
	if !info.EmailVerified {
		return nil, errors.New("email Google belum terverifikasi")
	}
	name := info.Name
	if name == "" {
		name = info.Email
	}
	return &User{ID: info.Sub, Email: info.Email, Name: name}, nil
}
