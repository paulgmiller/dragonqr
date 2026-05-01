package server

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"dragonqr/internal/game"

	"github.com/skip2/go-qrcode"
)

func TestStartCreatesPlayerAndSetsCookie(t *testing.T) {
	app := testServer(t, "")
	form := url.Values{
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

func TestStartRequiresOnlyAdventurerName(t *testing.T) {
	app := testServer(t, "")
	form := url.Values{
		"name": {"Pat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "Adventurer name is required.") {
		t.Fatal("expected adventurer name validation error")
	}
}

func TestRestartClearsPlayerCookie(t *testing.T) {
	app := testServer(t, "")
	form := url.Values{"adventurer_name": {"Star Pat"}}
	startReq := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(form.Encode()))
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	startRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(startRR, startReq)
	cookies := startRR.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected player cookie from start")
	}

	req := httptest.NewRequest(http.MethodPost, "/restart", nil)
	req.AddCookie(cookies[0])
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	cleared := false
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == "dragonqr_player" && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("restart did not clear player cookie")
	}
	if got := rr.Header().Get("Location"); got != "/q/start" {
		t.Fatalf("Location = %q, want /q/start", got)
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

func TestAdminTestRequiresPasswordWhenConfigured(t *testing.T) {
	app := testServer(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestStatusRestartButtonSaysDifferentPlayer(t *testing.T) {
	app := testServer(t, "")
	form := url.Values{"adventurer_name": {"Star Pat"}}
	startReq := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(form.Encode()))
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	startRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(startRR, startReq)
	cookies := startRR.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected player cookie from start")
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.AddCookie(cookies[0])
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "Restart as Different Player") {
		t.Fatal("status page does not include restart-as-different-player button")
	}
}

func TestAdminTestStartCreatesPlayerAndListsCodeLinks(t *testing.T) {
	app := testServer(t, "")
	form := url.Values{"adventurer_name": {"Tester"}}
	startReq := httptest.NewRequest(http.MethodPost, "/admin/test/start", strings.NewReader(form.Encode()))
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	startRR := httptest.NewRecorder()

	app.Handler().ServeHTTP(startRR, startReq)

	if startRR.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", startRR.Code, http.StatusSeeOther)
	}
	if got := startRR.Header().Get("Location"); got != "/admin/test" {
		t.Fatalf("Location = %q, want /admin/test", got)
	}
	cookies := startRR.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected player cookie")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	req.AddCookie(cookies[0])
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, body)
	}
	if !strings.Contains(body, "Adventure Walkthrough") {
		t.Fatal("admin test page does not render walkthrough title")
	}
	if !strings.Contains(body, `href="/admin/test/q/sword"`) {
		t.Fatal("admin test page does not link directly to quest code page")
	}
}

func TestAdminTestWalksQuestCodeWithoutQRCode(t *testing.T) {
	app := testServer(t, "")
	cookie := createAdminTestPlayerCookie(t, app)
	req := httptest.NewRequest(http.MethodGet, "/admin/test/q/sword", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, body)
	}
	if !strings.Contains(body, "<h1>Sword</h1>") {
		t.Fatal("admin test code route did not render the code page")
	}
	if !strings.Contains(body, "Admin Test Walkthrough") {
		t.Fatal("admin test navigation was not included on code page")
	}
	player, ok := app.store.Get(cookie.Value)
	if !ok {
		t.Fatal("expected test player to be saved")
	}
	if player.Attack != 4 {
		t.Fatalf("player attack = %d, want 4 after sword scan", player.Attack)
	}
}

func TestAdminTestCombatUsesAdminRollRoute(t *testing.T) {
	app := testServer(t, "")
	cookie := createAdminTestPlayerCookie(t, app)
	req := httptest.NewRequest(http.MethodGet, "/admin/test/q/dragon", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()

	app.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, body)
	}
	if !strings.Contains(body, `action="/admin/test/combat/roll"`) {
		t.Fatal("admin test combat page does not post rolls to admin test route")
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

	if strings.Contains(rr.Body.String(), `/static/generated/stations/sword.webp`) {
		t.Fatal("print page included generated station image; QR cards must stay reusable")
	}
}

func TestQRDataURLUsesHighestRecoveryLevel(t *testing.T) {
	qr, err := qrDataURL("http://example.test/q/outdoor-damage")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "data:image/png;base64,"
	encoded := strings.TrimPrefix(string(qr), prefix)
	if encoded == string(qr) {
		t.Fatal("qr data URL is missing PNG base64 prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	expected, err := qrcode.New("http://example.test/q/outdoor-damage", qrcode.Highest)
	if err != nil {
		t.Fatal(err)
	}
	want := expected.Image(256)
	if !sameImage(got, want) {
		t.Fatal("qrDataURL did not match qrcode.Highest output")
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

func createAdminTestPlayerCookie(t *testing.T, app *Server) *http.Cookie {
	t.Helper()
	form := url.Values{"adventurer_name": {"Tester"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/test/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected player cookie")
	}
	return cookies[0]
}

func sameImage(a image.Image, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(a.At(x, y)) != color.NRGBAModel.Convert(b.At(x, y)) {
				return false
			}
		}
	}
	return true
}
