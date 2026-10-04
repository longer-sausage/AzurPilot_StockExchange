package exchange

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const recaptchaTestToken = "recaptcha-test-token"

// 单元测试不访问外网，但必须经过相同的 HTTP Siteverify 请求与响应处理。
func stubTestCaptcha(t *testing.T, server *Server) (*atomic.Int32, *atomic.Bool, *httptest.Server) {
	t.Helper()
	calls, success := &atomic.Int32{}, &atomic.Bool{}
	success.Store(true)
	verify := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.Form.Get("secret") != recaptchaTestSecret || r.Form.Get("response") != recaptchaTestToken {
			t.Error("Mock 必须向 Siteverify 提交官方测试密钥和测试 token")
		}
		_ = json.NewEncoder(w).Encode(recaptchaResult{Success: success.Load()})
	}))
	t.Cleanup(verify.Close)
	server.SiteverifyURL = verify.URL
	return calls, success, verify
}

func TestMockCaptchaUsesRecaptchaSiteverify(t *testing.T) {
	f := setup(t)
	s := NewServer(f.e, Config{Mock: true, SecretKey: "must-not-use-production-secret"})
	calls, success, verify := stubTestCaptcha(t, s)
	requireCode(t, s.verify(t.Context(), ""), "CAPTCHA_REQUIRED")
	requireCode(t, s.verify(t.Context(), " \t\n"), "CAPTCHA_REQUIRED")
	requireCode(t, s.verify(t.Context(), strings.Repeat("x", 16385)), "CAPTCHA_REQUIRED")
	if calls.Load() != 0 {
		t.Fatal("缺失或超长 token 不应访问 reCAPTCHA")
	}
	for _, action := range []string{"register", "login", "console-login"} {
		if err := s.verify(t.Context(), recaptchaTestToken); err != nil {
			t.Fatal(action, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("每次认证都必须访问 Siteverify，不能在本地直接放行")
	}
	success.Store(false)
	requireCode(t, s.verify(t.Context(), recaptchaTestToken), "CAPTCHA_FAILED")
	verify.Close()
	requireCode(t, s.verify(t.Context(), recaptchaTestToken), "CAPTCHA_UNAVAILABLE")
}

func TestProductionCaptchaConfigAndMetadata(t *testing.T) {
	t.Setenv("MOCK_MODE", "false")
	t.Setenv("EXCHANGE_DOMAIN", "stock.nanoda.work")
	t.Setenv("ADMIN_PASSWORD", "test-admin-password")
	t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
	t.Setenv("RECAPTCHA_SECRET_KEY", "test-production-secret")
	c, err := LoadConfig()
	if err != nil {
		t.Fatal("服务端不配置 site key 也必须能启动", err)
	}
	f := setup(t)
	s := NewServer(f.e, c)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/meta", nil))
	var meta map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &meta); err != nil || w.Code != 200 {
		t.Fatal("元数据响应无效", w.Code, err)
	}
	if _, exists := meta["recaptchaSiteKey"]; exists || strings.Contains(w.Body.String(), c.SecretKey) {
		t.Fatal("服务端不得返回验证码密钥")
	}
	if s.SiteverifyURL != "https://www.recaptcha.net/recaptcha/api/siteverify" {
		t.Fatal("服务端必须使用 recaptcha.net")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "https://www.recaptcha.net/recaptcha/") || !strings.Contains(csp, "https://www.gstatic.com/recaptcha/") || strings.Contains(csp, "google.com") || strings.Contains(csp, "challenges.cloudflare.com") {
		t.Fatal("CSP 必须允许 reCAPTCHA 国内入口及官方静态资源，不得使用不可达入口", csp)
	}
	for _, secret := range []string{"", " \t", recaptchaTestSecret, "replace-with-google-recaptcha-secret"} {
		t.Setenv("RECAPTCHA_SECRET_KEY", secret)
		if _, err = LoadConfig(); err == nil {
			t.Fatal("生产必须配置真实验证码私密密钥")
		}
	}
}

func TestRecaptchaUpstreamErrors(t *testing.T) {
	f := setup(t)
	s := NewServer(f.e, Config{SecretKey: "production-secret"})
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"used-token", `{"success":false,"error-codes":["timeout-or-duplicate"]}`, "CAPTCHA_FAILED", 200},
		{"invalid-token", `{"success":false,"error-codes":["invalid-input-response"]}`, "CAPTCHA_FAILED", 200},
		{"invalid-secret", `{"success":false,"error-codes":["invalid-input-secret"]}`, "CAPTCHA_UNAVAILABLE", 200},
		{"http-error", `{"success":true}`, "CAPTCHA_UNAVAILABLE", 503},
		{"bad-json", `<html>upstream error</html>`, "CAPTCHA_UNAVAILABLE", 200},
		{"missing-success", `{}`, "CAPTCHA_FAILED", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			s.SiteverifyURL = upstream.URL
			requireCode(t, s.verify(t.Context(), "token"), tc.code)
		})
	}
}

func TestRecaptchaRegisterThenLoginWithFreshTokens(t *testing.T) {
	f := setup(t)
	s := NewServer(f.e, Config{SecretKey: "production-secret", AdminPassword: "test-admin-password", SessionSecret: strings.Repeat("s", 32)})
	seen := map[string]bool{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.Form.Get("secret") != "production-secret" {
			t.Error("认证必须请求 Siteverify")
		}
		token := r.Form.Get("response")
		success := token != "" && !seen[token]
		seen[token] = true
		// 无论页面域名或旧 action 如何，生产 v2 只以 Google 验证结果为准。
		payload := map[string]any{"success": success, "hostname": "unlisted-pilot.example", "action": "register"}
		if !success {
			payload["error-codes"] = []string{"timeout-or-duplicate"}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer upstream.Close()
	s.SiteverifyURL = upstream.URL
	identity := newTestIdentity(t)
	request := func(path string, body map[string]any, status int) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("POST", path, bytes.NewReader(data)))
		if w.Code != status || strings.Contains(w.Body.String(), "验证码来源或用途不匹配") {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if status == 400 && (!strings.Contains(w.Body.String(), `"code":"CAPTCHA_FAILED"`) || strings.Contains(w.Body.String(), `"token":`)) {
			t.Fatal("复用验证码不得产生会话", w.Body.String())
		}
	}
	report := identity.report(f.now, 1000, f.now.Unix())
	request("/api/register", map[string]any{"username": "验证码回归猫", "password": "test-password-123", "acceptedNotice": "2026-10-03", "recaptchaToken": "register-token", "report": report}, 201)
	request("/api/login", map[string]any{"username": "验证码回归猫", "password": "test-password-123", "recaptchaToken": "login-token", "report": report}, 200)
	request("/api/console/login", map[string]any{"password": "test-admin-password", "recaptchaToken": "admin-token"}, 200)
	request("/api/login", map[string]any{"username": "验证码回归猫", "password": "test-password-123", "recaptchaToken": "register-token", "report": report}, 400)
}
