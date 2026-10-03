package exchange

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type testIdentity struct {
	id  string
	key ed25519.PrivateKey
}

func newTestIdentity(t *testing.T) testIdentity {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(pub)
	s := hex.EncodeToString(hash[:])
	return testIdentity{s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32], key}
}
func (i testIdentity) report(now time.Time, ap, observed int64) InstanceReport {
	p := InstanceReport{InstanceID: i.id, PublicKey: base64.StdEncoding.EncodeToString(i.key.Public().(ed25519.PublicKey)), ActionPoints: ap, ObservedAt: observed, IssuedAt: now.Unix()}
	p.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(i.key, []byte(reportCanonical(p))))
	return p
}
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || publicError(err).Code != code {
		t.Fatalf("期望 %s，实际 %v", code, err)
	}
}

func TestInstanceSignatureDoesNotAuthenticateGameData(t *testing.T) {
	identity := newTestIdentity(t)
	now := time.Now()
	report := identity.report(now, 987654, now.Unix())
	// 直接接受客户端自报的行动力，不读取截图，不执行真实性核验。
	binding, err := verifyInstance(report, now)
	if err != nil || binding.InstanceID != identity.id {
		t.Fatal(err)
	}
	report.ActionPoints++
	_, err = verifyInstance(report, now)
	requireCode(t, err, "INVALID_INSTANCE")
	_, err = verifyInstance(identity.report(now.Add(-121*time.Second), 123, now.Unix()), now)
	requireCode(t, err, "STALE_INSTANCE")
	_, err = verifyInstance(identity.report(now, 0, 0), now)
	if err != nil {
		t.Fatal("没有行动力记录也可以认证实例", err)
	}
}

func TestPermanentInstanceBindingHTTPAndRestart(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	database := filepath.Join(t.TempDir(), "bound.db")
	engine, err := Open(database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { engine.Close() }()
	server := NewServer(engine, Config{Mock: true, Domain: "stock.nanoda.work", SessionSecret: "local-test-session-secret-32-characters"})
	stubTestCaptcha(t, server)
	request := func(method, path string, body any, token, binding string) (int, map[string]any) {
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.RemoteAddr = "127.0.0.1:12345"
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if binding != "" {
			r.Header.Set("X-MMEX-Instance", binding)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		var result map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return w.Code, result
	}
	identity, other := newTestIdentity(t), newTestIdentity(t)
	register := func(name string, i testIdentity) (int, map[string]any) {
		return request("POST", "/api/register", map[string]any{"username": name, "password": "test-password-123", "turnstileToken": turnstileTestToken, "acceptedNotice": "2026-10-03", "report": i.report(now, 1000, now.Unix())}, "", "")
	}
	code, result := register("绑定测试", identity)
	if code != 201 {
		t.Fatal(code, result)
	}
	token, upload := result["token"].(string), result["uploadToken"].(string)
	player := result["player"].(map[string]any)
	binding := player["binding"].(map[string]any)["key"].(string)
	id := int64(player["id"].(float64))
	if code, _ = register("同实例新用户", identity); code != 400 {
		t.Fatal("同实例不应再注册")
	}
	if code, _ = register("绑定测试", other); code != 400 {
		t.Fatal("用户名必须唯一")
	}
	if code, _ = request("GET", "/api/account", nil, token, ""); code != 401 {
		t.Fatal("交易会话必须绑定实例")
	}
	if code, _ = request("GET", "/api/account", nil, token, binding); code != 200 {
		t.Fatal("正确实例会话不可用")
	}
	login := func(i testIdentity, captcha string) (int, map[string]any) {
		return request("POST", "/api/login", map[string]any{"username": "绑定测试", "password": "test-password-123", "turnstileToken": captcha, "report": i.report(now, 0, 0)}, "", "")
	}
	if code, _ = login(identity, ""); code != 400 {
		t.Fatal("登录也必须过验证码")
	}
	code, result = login(other, turnstileTestToken)
	if code != 403 {
		t.Fatal("禁止账户换绑", code, result)
	}
	code, result = login(identity, turnstileTestToken)
	if code != 200 {
		t.Fatal("同实例登录无需行动力真实性验证", code, result)
	}
	oldObserved := now.Unix()
	now = now.Add(16 * time.Second)
	code, result = request("POST", "/api/quotes", map[string]any{"report": other.report(now, 9999, now.Unix())}, upload, binding)
	if code != 400 {
		t.Fatal("上传凭据不能跨实例使用")
	}
	code, result = request("POST", "/api/quotes", map[string]any{"report": identity.report(now, 9999, now.Unix())}, upload, binding)
	if code != 200 {
		t.Fatal("应直接接受当前实例自报的行动力", code, result)
	}
	account, _ := engine.Account(id)
	if account.Player.Quote.Price != 999900 {
		t.Fatal("报价应等于自报行动力")
	}
	code, _ = request("POST", "/api/quotes", map[string]any{"report": identity.report(now, 1000, oldObserved)}, upload, binding)
	if code != 400 {
		t.Fatal("旧记录不能覆盖更新报价")
	}
	// 同一缓存分桶内经过可配置有效期，过期标记和 ETag 必须改变。
	ttl := engine.Settings().Active.QuoteTTLSeconds
	now = now.Add(time.Duration(ttl) * time.Second)
	_, etag, _ := engine.MarketJSON()
	now = now.Add(time.Second)
	raw, expiredTag, _ := engine.MarketJSON()
	var market Market
	_ = json.Unmarshal(raw, &market)
	if !market.Stocks[0].Stale || etag == expiredTag {
		t.Fatal("报价过期缓存未失效")
	}
	if err = engine.Close(); err != nil {
		t.Fatal(err)
	}
	engine, err = Open(database, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if engine.SessionBinding(id) != binding {
		t.Fatal("重启丢失永久绑定")
	}
	b, _ := verifyInstance(identity.report(now, 0, now.Unix()), now)
	_, _, err = engine.Register("重启再注册", "test-password-123", 0, now.Unix(), b)
	requireCode(t, err, "INSTANCE_TAKEN")
	var count int
	if err = engine.db.QueryRow("SELECT count(*) FROM instance_bindings WHERE binding_key=? AND player_id=?", binding, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("SQL 永久绑定约束失效：%d %v", count, err)
	}
}
