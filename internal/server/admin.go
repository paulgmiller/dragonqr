package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dragonqr/internal/game"
)

type adminTestCode struct {
	game.Code
	VisitURL     string
	Scanned      bool
	Current      bool
	ActiveCombat bool
}

type adminTestNav struct {
	HomeURL         string
	StartURL        string
	RestartURL      string
	StatusURL       string
	CombatRollURL   string
	ActiveCombatURL string
	PrevURL         string
	NextURL         string
	CurrentID       string
	HasPlayer       bool
	Codes           []adminTestCode
}

func (s *Server) handleAdminTest(w http.ResponseWriter, r *http.Request) {
	p, ok := s.currentPlayer(r)
	data := map[string]any{
		"Quest":     s.quest,
		"Player":    p,
		"HasPlayer": ok,
		"AdminTest": s.adminTestNav(r, p, ""),
	}
	s.render(w, "admin_test.html", data)
}

func (s *Server) handleAdminTestStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Warn("bad admin test start form", "error", err)
		http.Error(w, "Bad form.", http.StatusBadRequest)
		return
	}
	adventurerName := strings.TrimSpace(r.FormValue("adventurer_name"))
	if adventurerName == "" {
		adventurerName = "Test Adventurer"
	}
	p, err := s.store.Create(s.quest, adventurerName, adventurerName)
	if err != nil {
		slog.Error("failed to create admin test player", "error", err)
		http.Error(w, "Could not create test player.", http.StatusInternalServerError)
		return
	}
	slog.Info("admin test player created", "player_id", p.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "dragonqr_player",
		Value:    p.ID,
		Path:     "/",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin/test", http.StatusSeeOther)
}

func (s *Server) handleAdminTestRestart(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.currentPlayer(r); ok {
		slog.Info("admin test restart requested", "player_id", p.ID)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dragonqr_player",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin/test", http.StatusSeeOther)
}

func (s *Server) handleAdminTestCode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	code, ok := s.quest.Code(id)
	if !ok {
		slog.Warn("unknown admin test code opened", "code_id", id)
		http.NotFound(w, r)
		return
	}
	p, ok := s.currentPlayer(r)
	if !ok {
		http.Redirect(w, r, "/admin/test", http.StatusSeeOther)
		return
	}
	if code.Type == game.CodeStart {
		var result game.ScanResult
		playerID := p.ID
		var err error
		p, err = s.store.Update(playerID, func(p *game.Player) error {
			result = game.ApplyScan(s.quest, p, code)
			return nil
		})
		if err != nil {
			slog.Error("failed to save admin test start progress", "error", err, "player_id", playerID, "code_id", code.ID)
			http.Error(w, "Could not save quest progress.", http.StatusInternalServerError)
			return
		}
		data := s.statusView(r, p, result.Body)
		s.addAdminTestView(data, r, p, code.ID)
		s.render(w, "status.html", data)
		return
	}

	playerID := p.ID
	var result game.ScanResult
	var err error
	p, err = s.store.Update(playerID, func(p *game.Player) error {
		result = game.ApplyScan(s.quest, p, code)
		return nil
	})
	if err != nil {
		slog.Error("failed to save admin test scan progress", "error", err, "player_id", playerID, "code_id", code.ID)
		http.Error(w, "Could not save quest progress.", http.StatusInternalServerError)
		return
	}
	result.Player = p
	slog.Info("admin test code opened",
		"player_id", p.ID,
		"code_id", code.ID,
		"code_type", code.Type,
		"already_seen", result.AlreadySeen,
		"blocked", result.Blocked,
		"combat_active", result.Combat != nil,
		"victory", result.Victory,
	)
	if result.Combat != nil {
		result.Combat = p.Combat
		data := s.combatView(r, result)
		s.addAdminTestView(data, r, p, code.ID)
		s.render(w, "combat.html", data)
		return
	}
	data := s.scanView(r, result)
	s.addAdminTestView(data, r, p, code.ID)
	s.render(w, "scan.html", data)
}

func (s *Server) handleAdminTestCombatRoll(w http.ResponseWriter, r *http.Request) {
	p, ok := s.currentPlayer(r)
	if !ok {
		http.Redirect(w, r, "/admin/test", http.StatusSeeOther)
		return
	}
	playerID := p.ID
	currentID := ""
	if p.Combat != nil {
		currentID = p.Combat.CodeID
	}
	var result game.ScanResult
	var err error
	p, err = s.store.Update(playerID, func(p *game.Player) error {
		var rollErr error
		result, rollErr = game.RollCombat(s.quest, p)
		return rollErr
	})
	if err != nil {
		slog.Error("failed to save admin test combat progress", "error", err, "player_id", playerID)
		http.Redirect(w, r, "/admin/test", http.StatusSeeOther)
		return
	}
	result.Player = p
	if result.Code.ID != "" {
		currentID = result.Code.ID
	}
	slog.Info("admin test combat rolled",
		"player_id", p.ID,
		"code_id", result.Code.ID,
		"combat_active", result.Combat != nil,
		"defeated", result.Defeated,
		"victory", result.Victory,
	)
	if result.Combat != nil {
		result.Combat = p.Combat
		data := s.combatView(r, result)
		s.addAdminTestView(data, r, p, currentID)
		s.render(w, "combat.html", data)
		return
	}
	data := s.scanView(r, result)
	s.addAdminTestView(data, r, p, currentID)
	s.render(w, "scan.html", data)
}

func (s *Server) addAdminTestView(data map[string]any, r *http.Request, p *game.Player, currentID string) {
	nav := s.adminTestNav(r, p, currentID)
	data["AdminTest"] = nav
	data["StatusURL"] = nav.StatusURL
	data["CombatRollURL"] = nav.CombatRollURL
	data["RestartURL"] = nav.RestartURL
	data["RestartLabel"] = "Start Different Test Adventurer"
	if p != nil && p.Combat != nil {
		data["ActiveCombatURL"] = nav.ActiveCombatURL
	}
}

func (s *Server) adminTestNav(r *http.Request, p *game.Player, currentID string) adminTestNav {
	nav := adminTestNav{
		HomeURL:       "/admin/test",
		StartURL:      "/admin/test/start",
		RestartURL:    "/admin/test/restart",
		StatusURL:     "/admin/test/q/" + url.PathEscape(s.quest.StartCode),
		CombatRollURL: "/admin/test/combat/roll",
		CurrentID:     currentID,
		HasPlayer:     p != nil,
	}
	activeCombatID := ""
	if p != nil && p.Combat != nil {
		activeCombatID = p.Combat.CodeID
		nav.ActiveCombatURL = adminTestCodeURL(activeCombatID)
	}
	currentIndex := -1
	for i, code := range s.quest.Codes {
		if code.ID == currentID {
			currentIndex = i
		}
		visitURL := adminTestCodeURL(code.ID)
		nav.Codes = append(nav.Codes, adminTestCode{
			Code:         code,
			VisitURL:     visitURL,
			Scanned:      p != nil && p.HasScanned(code.ID),
			Current:      code.ID == currentID,
			ActiveCombat: code.ID == activeCombatID,
		})
	}
	if currentIndex > 0 {
		nav.PrevURL = nav.Codes[currentIndex-1].VisitURL
	}
	if currentIndex >= 0 && currentIndex < len(nav.Codes)-1 {
		nav.NextURL = nav.Codes[currentIndex+1].VisitURL
	}
	return nav
}

func adminTestCodeURL(id string) string {
	return "/admin/test/q/" + url.PathEscape(id)
}
