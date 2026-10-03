package exchange

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

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
		if r.Method != "POST" || r.Form.Get("secret") != turnstileTestSecret || r.Form.Get("response") != turnstileTestToken {
			t.Error("Mock 必须向 Siteverify 提交官方测试密钥和测试 token")
		}
		_ = json.NewEncoder(w).Encode(turnstileResult{Success: success.Load(), Hostname: "example.com"})
	}))
	t.Cleanup(verify.Close)
	server.SiteverifyURL = verify.URL
	return calls, success, verify
}

func TestMockCaptchaUsesCloudflareSiteverify(t *testing.T) {
	f := setup(t)
	s := NewServer(f.e, Config{Mock: true, SecretKey: "must-not-use-production-secret"})
	calls, success, verify := stubTestCaptcha(t, s)
	requireCode(t, s.verify(t.Context(), "", "register"), "CAPTCHA_REQUIRED")
	requireCode(t, s.verify(t.Context(), "mock-pass", "login"), "CAPTCHA_FAILED")
	if calls.Load() != 0 {
		t.Fatal("缺失或非官方测试 token 不应访问 Cloudflare")
	}
	for _, action := range []string{"register", "login", "console-login"} {
		if err := s.verify(t.Context(), turnstileTestToken, action); err != nil {
			t.Fatal(action, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("每次认证都必须访问 Siteverify，不能在本地直接放行")
	}
	success.Store(false)
	requireCode(t, s.verify(t.Context(), turnstileTestToken, "login"), "CAPTCHA_FAILED")
	verify.Close()
	requireCode(t, s.verify(t.Context(), turnstileTestToken, "register"), "CAPTCHA_UNAVAILABLE")
}

func TestProductionCaptchaConfigAndMetadata(t *testing.T) {
	t.Setenv("MOCK_MODE", "false")
	t.Setenv("EXCHANGE_DOMAIN", "stock.nanoda.work")
	t.Setenv("ADMIN_PASSWORD", "test-admin-password")
	t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
	t.Setenv("TURNSTILE_SECRET_KEY", "test-production-secret")
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
	if _, exists := meta["turnstileSiteKey"]; exists || strings.Contains(w.Body.String(), c.SecretKey) {
		t.Fatal("服务端不得返回验证码密钥")
	}
	for _, secret := range []string{"", turnstileTestSecret, "2x0000000000000000000000000000000AA", "3x0000000000000000000000000000000AA"} {
		t.Setenv("TURNSTILE_SECRET_KEY", secret)
		if _, err = LoadConfig(); err == nil {
			t.Fatal("生产必须配置真实验证码私密密钥")
		}
	}
}
