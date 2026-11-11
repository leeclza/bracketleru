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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookie = "bl_session"
	stateCookie   = "bl_oauth_state"
	sessionTTL    = 30 * 24 * time.Hour
)

type User struct {
	ID    string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Exp   int64  `json:"exp"`
}

type authConfig struct {
	clientID, clientSecret, baseURL string
	secret                          []byte
}

func loadAuthConfig() *authConfig {
	cfg := &authConfig{
		clientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		clientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		baseURL:      strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		secret:       []byte(os.Getenv("SESSION_SECRET")),
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
	if json.Unmarshal(payload, &u) != nil || u.ID == "" || time.Now().Unix() > u.Exp {
		return nil
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
	if public || s.currentUser(c) != nil {
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
	u.Exp = time.Now().Add(sessionTTL).Unix()
	payload, _ := json.Marshal(u)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, s.auth.sign(payload), int(sessionTTL.Seconds()), "/", "", secureCookie(c), true)
	c.Redirect(http.StatusSeeOther, safeNext(next))
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
	name := info.Name
	if name == "" {
		name = info.Email
	}
	return &User{ID: info.Sub, Email: info.Email, Name: name}, nil
}

