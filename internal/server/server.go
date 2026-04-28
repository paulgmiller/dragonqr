package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"image/png"
	"log/slog"
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
	OpenAIAPIKey      string
	GeneratedImageDir string
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
	if cfg.GeneratedImageDir == "" {
		cfg.GeneratedImageDir = filepath.Join(staticDir(), "generated", "stations")
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
	mux.HandleFunc("POST /restart", s.handleRestart)
	mux.HandleFunc("POST /combat/roll", s.handleCombatRoll)
	mux.HandleFunc("GET /organizer", s.withOrganizerAuth(s.handleOrganizer))
	mux.HandleFunc("GET /organizer/print", s.withOrganizerAuth(s.handlePrint))
	mux.HandleFunc("POST /organizer/images/generate", s.withOrganizerAuth(s.handleGenerateMissingImages))
	mux.HandleFunc("POST /organizer/images/generate/{id}", s.withOrganizerAuth(s.handleGenerateCodeImage))
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
		slog.Warn("unknown code scanned", "code_id", id)
		http.NotFound(w, r)
		return
	}
	if code.Type == game.CodeStart {
		if p, ok := s.currentPlayer(r); ok {
			slog.Info("start code scanned by existing player", "player_id", p.ID, "code_id", code.ID)
			s.render(w, "status.html", s.statusView(r, p, "Your adventure is already started."))
			return
		}
		slog.Info("start code scanned", "code_id", code.ID)
		s.render(w, "start.html", map[string]any{
			"Quest": s.quest,
			"Code":  code,
		})
		return
	}

	p, ok := s.currentPlayer(r)
	if !ok {
		slog.Info("code scan without player", "code_id", code.ID, "code_type", code.Type)
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
		slog.Error("failed to save scan progress", "error", err, "player_id", p.ID, "code_id", code.ID)
		http.Error(w, "Could not save quest progress.", http.StatusInternalServerError)
		return
	}
	result.Player = p
	slog.Info("code scanned",
		"player_id", p.ID,
		"code_id", code.ID,
		"code_type", code.Type,
		"already_seen", result.AlreadySeen,
		"blocked", result.Blocked,
		"combat_active", result.Combat != nil,
		"victory", result.Victory,
		"health", p.Health,
		"max_health", p.MaxHealth,
		"attack", p.Attack,
		"armor", p.Armor,
	)
	if result.Combat != nil {
		result.Combat = p.Combat
		s.render(w, "combat.html", s.combatView(r, result))
		return
	}
	s.render(w, "scan.html", s.scanView(r, result))
}

func (s *Server) handleCombatRoll(w http.ResponseWriter, r *http.Request) {
	p, ok := s.currentPlayer(r)
	if !ok {
		slog.Info("combat roll without player")
		http.Redirect(w, r, "/q/"+url.PathEscape(s.quest.StartCode), http.StatusSeeOther)
		return
	}
	var result game.ScanResult
	var err error
	p, err = s.store.Update(p.ID, func(p *game.Player) error {
		var rollErr error
		result, rollErr = game.RollCombat(s.quest, p)
		return rollErr
	})
	if errors.Is(err, game.ErrNoActiveCombat) {
		slog.Info("combat roll without active combat", "player_id", p.ID)
		http.Redirect(w, r, "/status", http.StatusSeeOther)
		return
	}
	if err != nil {
		slog.Error("failed to save combat progress", "error", err, "player_id", p.ID)
		http.Error(w, "Could not save combat progress.", http.StatusInternalServerError)
		return
	}
	result.Player = p
	attrs := []any{
		"player_id", p.ID,
		"code_id", result.Code.ID,
		"code_type", result.Code.Type,
		"combat_active", result.Combat != nil,
		"blocked", result.Blocked,
		"defeated", result.Defeated,
		"victory", result.Victory,
		"health", p.Health,
		"max_health", p.MaxHealth,
	}
	if result.Combat != nil {
		attrs = append(attrs, "enemy_health", result.Combat.EnemyHealth, "rounds", len(result.Combat.Rounds))
	}
	if result.LastRound != nil {
		attrs = append(attrs,
			"player_roll", result.LastRound.PlayerRoll,
			"enemy_roll", result.LastRound.EnemyRoll,
			"player_damage", result.LastRound.PlayerDamage,
			"enemy_damage", result.LastRound.EnemyDamage,
		)
	}
	slog.Info("combat rolled", attrs...)
	if result.Combat != nil {
		result.Combat = p.Combat
		s.render(w, "combat.html", s.combatView(r, result))
		return
	}
	s.render(w, "scan.html", s.scanView(r, result))
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Warn("bad start form", "error", err)
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	adventurerName := strings.TrimSpace(r.FormValue("adventurer_name"))
	if adventurerName == "" {
		slog.Info("start form rejected", "missing_adventurer_name", true)
		s.render(w, "start.html", map[string]any{
			"Quest": s.quest,
			"Code":  s.quest.Start(),
			"Error": "Adventurer name is required.",
			"Name":  name,
			"Adv":   adventurerName,
		})
		return
	}
	if name == "" {
		name = adventurerName
	}
	p, err := s.store.Create(s.quest, name, adventurerName)
	if err != nil {
		slog.Error("failed to create player", "error", err)
		http.Error(w, "Could not create player.", http.StatusInternalServerError)
		return
	}
	slog.Info("player created",
		"player_id", p.ID,
		"max_health", p.MaxHealth,
		"health", p.Health,
		"attack", p.Attack,
		"armor", p.Armor,
	)
	http.SetCookie(w, &http.Cookie{
		Name:     "dragonqr_player",
		Value:    p.ID,
		Path:     "/",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/status", http.StatusSeeOther)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.currentPlayer(r); ok {
		slog.Info("player restart requested", "player_id", p.ID)
	} else {
		slog.Info("player restart requested without active player")
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dragonqr_player",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/q/"+url.PathEscape(s.quest.StartCode), http.StatusSeeOther)
}

func (s *Server) handleOrganizer(w http.ResponseWriter, r *http.Request) {
	s.render(w, "organizer.html", s.organizerView(r, "", ""))
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
			slog.Error("failed to generate qr code", "error", err, "code_id", code.ID)
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
		"Quest":           s.quest,
		"Player":          p,
		"Message":         message,
		"Ready":           ready,
		"Missing":         missing,
		"DragonURL":       s.codeURL(r, s.quest.DragonCode),
		"OrganizerURL":    "/organizer",
		"GearNames":       p.GearNames(s.quest),
		"CompanionNames":  p.CompanionNames(s.quest),
		"ActiveCombatURL": s.activeCombatURL(r, p),
	}
}

func (s *Server) scanView(r *http.Request, result game.ScanResult) map[string]any {
	ready, missing := game.DragonReady(s.quest, result.Player)
	return map[string]any{
		"Quest":    s.quest,
		"Result":   result,
		"Player":   result.Player,
		"Ready":    ready,
		"Missing":  missing,
		"ImageURL": s.codeImageURL(result.Code),
	}
}

func (s *Server) combatView(r *http.Request, result game.ScanResult) map[string]any {
	return map[string]any{
		"Quest":    s.quest,
		"Result":   result,
		"Player":   result.Player,
		"Combat":   result.Combat,
		"ImageURL": s.codeImageURL(result.Code),
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
			slog.Warn("organizer auth failed", "path", r.URL.Path, "has_credentials", ok)
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
		slog.Error("template render failed", "error", err, "template", name)
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
	img, err := qrcode.New(value, qrcode.Highest)
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
