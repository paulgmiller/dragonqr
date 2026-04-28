package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"dragonqr/internal/game"
)

func TestStartCreatesPlayerAndSetsCookie(t *testing.T) {
	app := testServer(t, "")
	form := url.Values{
		"name":            {"Pat"},
		"adventurer_name": {"Star Pat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if len(rr.Result().Cookies()) == 0 {
		t.Fatal("expected player cookie")
	}
}

func TestOrganizerRequiresPasswordWhenConfigured(t *testing.T) {
	app := testServer(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/organizer", nil)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestPrintPageIncludesAllQuestCodes(t *testing.T) {
	app := testServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/organizer/print", nil)
	req.Host = "example.test"
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	count := strings.Count(rr.Body.String(), `<article class="qr-card">`)
	if count != len(app.quest.Codes) {
		t.Fatalf("qr-card count = %d, want %d", count, len(app.quest.Codes))
	}
	if strings.Contains(rr.Body.String(), "#ZgotmplZ") {
		t.Fatal("print page contains sanitized QR data URL placeholder #ZgotmplZ")
	}
	qrCount := strings.Count(rr.Body.String(), `src="data:image/png;base64,`)
	if qrCount != len(app.quest.Codes) {
		t.Fatalf("QR data URL count = %d, want %d", qrCount, len(app.quest.Codes))
	}
}

func TestPrintPageUsesReusableIDCards(t *testing.T) {
	app := testServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/organizer/print", nil)
	req.Host = "example.test"
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, "Sword") {
		t.Fatal("print page contains code title; reusable cards should show stable IDs only")
	}
	if strings.Contains(body, "http://example.test/q/sword") {
		t.Fatal("print page exposes raw code URL")
	}
	if !strings.Contains(body, ">sword<") {
		t.Fatal("print page does not show stable code ID")
	}
}

func TestGenerateImagesRequiresAPIKey(t *testing.T) {
	app := testServer(t, "")
	req := httptest.NewRequest(http.MethodPost, "/organizer/images/generate", nil)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "OPENAI_API_KEY is required") {
		t.Fatal("expected missing API key error")
	}
}

func TestPrintPageIncludesGeneratedStationImage(t *testing.T) {
	app := testServer(t, "")
	if err := os.MkdirAll(app.cfg.GeneratedImageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app.cfg.GeneratedImageDir+"/sword.webp", []byte("webp"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/organizer/print", nil)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if !strings.Contains(rr.Body.String(), `/static/generated/stations/sword.webp`) {
		t.Fatal("print page did not include generated station image")
	}
}

func testServer(t *testing.T, password string) *Server {
	t.Helper()
	q := game.Quest{
		Title:      "Test Quest",
		Intro:      "Intro",
		StartCode:  "start",
		DragonCode: "dragon",
		BaseHealth: 10,
		BaseAttack: 2,
		Codes: []game.Code{
			{ID: "start", Type: game.CodeStart, Title: "Start"},
			{ID: "sword", Type: game.CodeWeapon, Title: "Sword", Effects: game.Effects{Attack: 2}},
			{ID: "dragon", Type: game.CodeDragon, Title: "Dragon", Enemy: game.Enemy{Health: 5, Attack: 1}},
		},
	}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	store, err := game.NewStore(t.TempDir() + "/players.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(&q, store, Config{
		OrganizerPassword: password,
		GeneratedImageDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return app
}
