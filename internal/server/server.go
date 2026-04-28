package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"dragonqr/internal/game"

	"github.com/skip2/go-qrcode"
)

type Config struct {
	Addr              string
	BaseURL           string
	OrganizerPassword string
}

type Server struct {
	quest *game.Quest
	store *game.Store
	cfg   Config
	tpl   *template.Template
}

func New(q *game.Quest, store *game.Store, cfg Config) (*Server, error) {
	tpl, err := template.ParseGlob(templateGlob())
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Server{quest: q, store: store, cfg: cfg, tpl: tpl}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /static/", s.handleStatic)
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /q/{id}", s.handleCode)
	mux.HandleFunc("POST /start", s.handleStart)
	mux.HandleFunc("GET /organizer", s.withOrganizerAuth(s.handleOrganizer))
	mux.HandleFunc("GET /organizer/print", s.withOrganizerAuth(s.handlePrint))
	return mux
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir()))).ServeHTTP(w, r)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.currentPlayer(r); ok {
		s.render(w, "status.html", s.statusView(r, p, ""))
		return
	}
	http.Redirect(w, r, "/q/"+url.PathEscape(s.quest.StartCode), http.StatusSeeOther)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := s.currentPlayer(r)
	if !ok {
		http.Redirect(w, r, "/q/"+url.PathEscape(s.quest.StartCode), http.StatusSeeOther)
		return
	}
	s.render(w, "status.html", s.statusView(r, p, ""))
}

func (s *Server) handleCode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	code, ok := s.quest.Code(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if code.Type == game.CodeStart {
		if p, ok := s.currentPlayer(r); ok {
			s.render(w, "status.html", s.statusView(r, p, "Your adventure is already started."))
			return
		}
		s.render(w, "start.html", map[string]any{
			"Quest": s.quest,
			"Code":  code,
		})
		return
	}

	p, ok := s.currentPlayer(r)
	if !ok {
		http.Redirect(w, r, "/q/"+url.PathEscape(s.quest.StartCode), http.StatusSeeOther)
		return
	}
	var result game.ScanResult
	var err error
	p, err = s.store.Update(p.ID, func(p *game.Player) error {
		result = game.ApplyScan(s.quest, p, code)
		return nil
	})
	if err != nil {
		http.Error(w, "Could not save quest progress.", http.StatusInternalServerError)
		return
	}
	result.Player = p
	s.render(w, "scan.html", s.scanView(r, result))
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	adventurerName := strings.TrimSpace(r.FormValue("adventurer_name"))
	if name == "" || adventurerName == "" {
		s.render(w, "start.html", map[string]any{
			"Quest": s.quest,
			"Code":  s.quest.Start(),
			"Error": "Both names are required.",
			"Name":  name,
			"Adv":   adventurerName,
		})
		return
	}
	p, err := s.store.Create(s.quest, name, adventurerName)
	if err != nil {
		http.Error(w, "Could not create player.", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dragonqr_player",
		Value:    p.ID,
		Path:     "/",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/status", http.StatusSeeOther)
}

func (s *Server) handleOrganizer(w http.ResponseWriter, r *http.Request) {
	s.render(w, "organizer.html", map[string]any{
		"Quest":   s.quest,
		"BaseURL": s.baseURL(r),
	})
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	type printableCode struct {
		game.Code
		URL string
		QR  template.URL
	}
	var codes []printableCode
	for _, code := range s.quest.Codes {
		u := s.codeURL(r, code.ID)
		qr, err := qrDataURL(u)
		if err != nil {
			http.Error(w, "Could not generate QR code.", http.StatusInternalServerError)
			return
		}
		codes = append(codes, printableCode{Code: code, URL: u, QR: qr})
	}
	s.render(w, "print.html", map[string]any{
		"Quest": s.quest,
		"Codes": codes,
	})
}

func (s *Server) currentPlayer(r *http.Request) (*game.Player, bool) {
	cookie, err := r.Cookie("dragonqr_player")
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	return s.store.Get(cookie.Value)
}

func (s *Server) statusView(r *http.Request, p *game.Player, message string) map[string]any {
	ready, missing := game.DragonReady(s.quest, p)
	return map[string]any{
		"Quest":        s.quest,
		"Player":       p,
		"Message":      message,
		"Ready":        ready,
		"Missing":      missing,
		"DragonURL":    s.codeURL(r, s.quest.DragonCode),
		"OrganizerURL": "/organizer",
	}
}

func (s *Server) scanView(r *http.Request, result game.ScanResult) map[string]any {
	ready, missing := game.DragonReady(s.quest, result.Player)
	return map[string]any{
		"Quest":   s.quest,
		"Result":  result,
		"Player":  result.Player,
		"Ready":   ready,
		"Missing": missing,
	}
}

func (s *Server) withOrganizerAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.OrganizerPassword == "" {
			next(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "organizer" || pass != s.cfg.OrganizerPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="Dragon QR Organizer"`)
			http.Error(w, "Organizer password required.", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Template error.", http.StatusInternalServerError)
	}
}

func (s *Server) codeURL(r *http.Request, id string) string {
	base := s.baseURL(r)
	u, err := url.Parse(base)
	if err != nil {
		return path.Join(base, "q", id)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/q/" + url.PathEscape(id)
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.BaseURL != "" {
		return strings.TrimRight(s.cfg.BaseURL, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host
}

func qrDataURL(value string) (template.URL, error) {
	img, err := qrcode.New(value, qrcode.Medium)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img.Image(256)); err != nil {
		return "", err
	}
	return template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())), nil
}

func IsServerClosed(err error) bool {
	return errors.Is(err, http.ErrServerClosed)
}

func templateGlob() string {
	for _, candidate := range []string{
		"templates/*.html",
		"../../templates/*.html",
	} {
		matches, _ := filepath.Glob(candidate)
		if len(matches) > 0 {
			return candidate
		}
	}
	return "templates/*.html"
}

func staticDir() string {
	for _, candidate := range []string{"static", "../../static"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return "static"
}
