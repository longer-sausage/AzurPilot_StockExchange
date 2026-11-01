package exchange

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{Domain: "stock.azurpilot.com", AdminPassword: "IsolatedAdmin7!Password", SessionSecret: strings.Repeat("s", 64), BotAPIToken: strings.Repeat("b", 64), Frontend: "../../frontend/dist", SecureCookies: false}
}
func request(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie, bot bool) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		data = encode(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	if bot {
		r.Header.Set("Authorization", "Bearer "+testConfig().BotAPIToken)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func jsonResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	return out
}
func TestHTTPRegistrationLoginRememberAndBotAuthentication(t *testing.T) {
	f := newFixture(t)
	server := NewServer(f.e, testConfig())
	w := request(t, server, "GET", "/api/v2/public/market", nil, nil, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	r := httptest.NewRequest("GET", "/api/v2/public/market", nil)
	r.Header.Set("If-None-Match", etag)
	cached := httptest.NewRecorder()
	server.ServeHTTP(cached, r)
	if cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatal("market did not honor ETag")
	}
	w = request(t, server, "POST", "/api/v2/bot/register", map[string]any{"officialUserId": "app:http-user", "username": "HTTP用户", "requestId": "http-register-request"}, nil, false)
	if w.Code != 401 {
		t.Fatal("bot allowed without token")
	}
	w = request(t, server, "POST", "/api/v2/bot/register", map[string]any{"officialUserId": "app:http-user", "username": "HTTP用户", "requestId": "http-register-request"}, nil, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	initial := jsonResponse(t, w)["password"].(string)
	w = request(t, server, "POST", "/api/v2/auth/register", map[string]any{"username": "网页禁用"}, nil, false)
	if w.Code != 404 {
		t.Fatal("web registration still enabled")
	}
	w = request(t, server, "POST", "/api/v2/auth/login", map[string]any{"username": "HTTP用户", "password": initial}, nil, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].MaxAge != 0 {
		t.Fatal("register cookie must be session cookie")
	}
	w = request(t, server, "POST", "/api/v2/orders/preview", map[string]any{"stock": "1", "side": "buy", "quantity": 1, "price": 100000, "clientId": "http-order-test"}, cookies, false)
	if w.Code != 400 || jsonResponse(t, w)["error"].(map[string]any)["code"] != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatal("forced password gate missing")
	}
	w = request(t, server, "POST", "/api/v2/auth/password", map[string]any{"currentPassword": initial, "password": "NewStrong7!Password"}, cookies, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	oldCookies := cookies
	cookies = w.Result().Cookies()
	w = request(t, server, "GET", "/api/v2/auth/me", nil, oldCookies, false)
	if w.Code != 401 {
		t.Fatal("old session survived password change")
	}
	w = request(t, server, "POST", "/api/v2/auth/login", map[string]any{"username": "HTTP用户", "password": "NewStrong7!Password", "remember": true}, nil, false)
	if w.Code != 200 || w.Result().Cookies()[0].MaxAge != 30*86400 {
		t.Fatal("remember player session failed", w.Code, w.Body.String())
	}
	cookies = w.Result().Cookies()
	w = request(t, server, "POST", "/api/v2/bot/reset-password", map[string]any{"officialUserId": "app:http-user", "confirmed": false, "requestId": "reset-http-first"}, nil, true)
	if w.Code != 400 {
		t.Fatal("reset bypassed confirmation")
	}
	w = request(t, server, "POST", "/api/v2/bot/reset-password", map[string]any{"officialUserId": "app:http-user", "confirmed": true, "requestId": "reset-http-first"}, nil, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(t, server, "GET", "/api/v2/auth/me", nil, cookies, false)
	if w.Code != 401 {
		t.Fatal("reset did not revoke prior session")
	}
	w = request(t, server, "POST", "/api/v2/admin/login", map[string]any{"password": testConfig().AdminPassword, "remember": true}, nil, false)
	if w.Code != 200 || w.Result().Cookies()[0].MaxAge != 30*86400 || w.Result().Cookies()[0].Name != "mmex_admin" {
		t.Fatal("admin remember failed")
	}
	adminCookie := w.Result().Cookies()
	w = request(t, server, "GET", "/api/v2/admin/accounts", nil, adminCookie, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "passwordHash") || strings.Contains(w.Body.String(), "officialUserId") {
		t.Fatal("private credential leaked")
	}
	w = request(t, server, "GET", "/api/v2/auth/me", nil, adminCookie, false)
	if w.Code != 401 {
		t.Fatal("admin cookie authorized as player")
	}
}
func TestHTTPInvalidInputOriginsOldEndpointsAndCompression(t *testing.T) {
	f := newFixture(t)
	server := NewServer(f.e, testConfig())
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		w := request(t, server, method, "/api/market", nil, nil, false)
		if w.Code != 404 {
			t.Fatal("legacy endpoint survived", method, w.Code)
		}
	}
	w := request(t, server, "POST", "/api/v2/auth/login", map[string]any{"username": "nobody", "password": "bad", "captcha": "bypass"}, nil, false)
	if w.Code != 400 {
		t.Fatal("unknown fields not rejected")
	}
	r := httptest.NewRequest("GET", "/api/v2/public/market", nil)
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("untrusted origin allowed")
	}
	r = httptest.NewRequest("GET", "/api/v2/public/rules", nil)
	rules := f.e.Settings()
	rules.TaxBrackets = []TaxBracket{}
	for i := int64(1); i <= 20; i++ {
		rules.TaxBrackets = append(rules.TaxBrackets, TaxBracket{UpTo: i * 100000000, RatePPM: i * 1000})
	}
	rules.TaxBrackets = append(rules.TaxBrackets, TaxBracket{RatePPM: 320000})
	if err := f.e.SetSettings(rules); err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Accept-Encoding", "gzip")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("large rules not compressed")
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil || !strings.Contains(string(raw), "taxBrackets") {
		t.Fatal("invalid compressed json", err)
	}
	w = request(t, server, "POST", "/nonsense", nil, nil, false)
	if w.Code != 405 {
		t.Fatal("static handler accepted mutation")
	}
	w = request(t, server, "GET", "/assets/does-not-exist.js", nil, nil, false)
	if w.Code != 404 {
		t.Fatal("missing asset returned index")
	}
	r = httptest.NewRequest("GET", "/api/v2/public/market", nil)
	r.Header.Set("Origin", "https://stock.azurpilot.com")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("own origin rejected")
	}
}
