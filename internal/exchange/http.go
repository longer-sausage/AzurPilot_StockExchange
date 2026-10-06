package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Domain, Listen, Database, Frontend, SecretKey, AdminPassword, SessionSecret string
	Origins                                                                     []string
	Mock                                                                        bool
}

// Google 官方测试凭据只用于本机 Mock，测试 token 仍须请求 Siteverify。
const recaptchaTestSecret = "6LeIxAcTAAAAAGG-vFI1TnRWxMZNFuojJ4WifJWe"

func LoadConfig() (Config, error) {
	get := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	c := Config{Domain: get("EXCHANGE_DOMAIN", "stock.nanoda.work"), Listen: get("LISTEN_ADDR", "127.0.0.1:8080"), Database: get("DATABASE_PATH", "data/exchange.db"), Frontend: get("FRONTEND_DIR", "frontend/dist"), SecretKey: strings.TrimSpace(os.Getenv("RECAPTCHA_SECRET_KEY")), AdminPassword: os.Getenv("ADMIN_PASSWORD"), SessionSecret: os.Getenv("SESSION_SECRET"), Mock: os.Getenv("MOCK_MODE") == "true", Origins: strings.Split(get("ALLOWED_ORIGINS", "http://localhost:*,http://127.0.0.1:*"), ",")}
	if c.Mock {
		host, _, err := net.SplitHostPort(c.Listen)
		if err != nil || host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return c, fmt.Errorf("mock 只能监听本机回环地址")
		}
		if c.AdminPassword == "" {
			c.AdminPassword = "mock-admin-password"
		}
		if c.SessionSecret == "" {
			c.SessionSecret = "local-mock-session-secret-32-characters"
		}
	}
	if len(c.AdminPassword) < 12 || len(c.SessionSecret) < 32 {
		return c, fmt.Errorf("ADMIN_PASSWORD 至少 12 字符，SESSION_SECRET 至少 32 字符")
	}
	if !c.Mock && (c.SecretKey == "" || c.SecretKey == recaptchaTestSecret || strings.HasPrefix(c.SecretKey, "replace-") || strings.HasPrefix(c.AdminPassword, "replace-") || strings.HasPrefix(c.SessionSecret, "replace-")) {
		return c, fmt.Errorf("请配置真实 RECAPTCHA_SECRET_KEY、管理员密码和会话密钥")
	}
	if strings.ContainsAny(c.Domain, "/\r\n \t;") {
		return c, fmt.Errorf("EXCHANGE_DOMAIN 无效")
	}
	return c, nil
}

type recaptchaResult struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}
type session struct {
	ID      int64  `json:"id"`
	Role    string `json:"role"`
	Expires int64  `json:"expires"`
	Binding string `json:"binding,omitempty"`
	Version uint64 `json:"version,omitempty"`
}
type Server struct {
	engine        *Engine
	config        Config
	client        *http.Client
	mux           *http.ServeMux
	Slots         chan struct{}
	passwordSlots chan struct{}
	SiteverifyURL string
}

func NewServer(e *Engine, c Config) *Server {
	// 网络等待和密码计算分别准入；使用可用 CPU，忙时立即拒绝，避免积压。
	cpus := max(1, runtime.GOMAXPROCS(0))
	verifySlots := max(4, cpus*2)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns, transport.MaxIdleConnsPerHost, transport.MaxConnsPerHost = verifySlots, verifySlots, verifySlots
	s := &Server{engine: e, config: c, client: &http.Client{Timeout: 8 * time.Second, Transport: transport}, mux: http.NewServeMux(), Slots: make(chan struct{}, verifySlots), passwordSlots: make(chan struct{}, cpus), SiteverifyURL: "https://www.recaptcha.net/recaptcha/api/siteverify"}
	e.RequireBinding = true
	s.mux.HandleFunc("GET /api/meta", s.meta)
	s.mux.HandleFunc("POST /api/register", s.register)
	s.mux.HandleFunc("POST /api/login", s.login)
	s.mux.HandleFunc("GET /api/market", s.market)
	s.mux.HandleFunc("GET /api/events", s.events)
	s.mux.HandleFunc("GET /api/history/{stock}", s.history)
	s.mux.HandleFunc("GET /api/stocks/{stock}", s.stockDetail)
	s.mux.HandleFunc("POST /api/quote-history", s.uploadHistory)
	s.mux.HandleFunc("GET /api/quote-history/manifest", s.historyManifest)
	s.mux.HandleFunc("GET /api/seasons", s.seasons)
	s.mux.HandleFunc("POST /api/quotes", s.upload)
	s.mux.HandleFunc("GET /api/account", s.player(s.account))
	s.mux.HandleFunc("POST /api/watchlist", s.player(s.watchlist))
	s.mux.HandleFunc("POST /api/upload-token", s.player(s.rotate))
	s.mux.HandleFunc("POST /api/orders", s.player(s.submit))
	s.mux.HandleFunc("GET /api/orders", s.player(s.orders))
	s.mux.HandleFunc("DELETE /api/orders/{order}", s.player(s.cancel))
	s.mux.HandleFunc("POST /api/console/login", s.adminLogin)
	s.mux.HandleFunc("GET /api/console/settings", s.admin(s.getSettings))
	s.mux.HandleFunc("PUT /api/console/settings", s.admin(s.setSettings))
	s.mux.HandleFunc("GET /api/console/players", s.admin(s.listPlayers))
	s.mux.HandleFunc("GET /api/console/players/{player}", s.admin(s.getPlayer))
	s.mux.HandleFunc("PUT /api/console/players/{player}", s.admin(s.updatePlayer))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { s.json(w, 200, map[string]bool{"ok": true}) })
	s.mux.HandleFunc("/", s.static)
	return s
}
func (s *Server) allowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if origin == "https://"+s.config.Domain {
		return true
	}
	for _, pattern := range s.config.Origins {
		pattern = strings.TrimSpace(pattern)
		if origin == pattern {
			return true
		}
		if strings.HasSuffix(pattern, ":*") {
			base := strings.TrimSuffix(pattern, ":*")
			if u.Scheme+"://"+u.Hostname() == base && u.Port() != "" {
				return true
			}
		}
	}
	return false
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	ancestors := []string{"'self'"}
	for _, v := range s.config.Origins {
		if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") {
			ancestors = append(ancestors, strings.TrimSpace(v))
		}
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://www.recaptcha.net/recaptcha/ https://www.gstatic.com/recaptcha/ https://www.gstatic.cn/recaptcha/; frame-src https://www.recaptcha.net/recaptcha/; connect-src 'self' https://www.recaptcha.net/recaptcha/; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors "+strings.Join(ancestors, " "))
	if origin := r.Header.Get("Origin"); origin != "" {
		if !s.allowed(origin) {
			s.error(w, 403, fail("ORIGIN_DENIED", "访问来源未授权"))
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-MMEX-Instance, If-None-Match")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	}
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	defer func() {
		if v := recover(); v != nil {
			slog.Error("请求异常", "path", r.URL.Path)
			s.error(w, 500, fmt.Errorf("panic"))
		}
	}()
	s.mux.ServeHTTP(w, r)
}
func (s *Server) json(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (s *Server) error(w http.ResponseWriter, status int, err error) {
	if publicError(err).Code == "INTERNAL" {
		status = 500
		slog.Error("业务操作失败", "error", err)
	}
	s.json(w, status, map[string]any{"error": publicError(err)})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fail("INVALID_JSON", "请求格式无效")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fail("INVALID_JSON", "仅允许一个 JSON 对象")
	}
	return nil
}
func (s *Server) verify(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 16384 {
		return fail("CAPTCHA_REQUIRED", "请完成 reCAPTCHA 人机验证")
	}
	secret := s.config.SecretKey
	if s.config.Mock {
		secret = recaptchaTestSecret
	}
	data := url.Values{"secret": {secret}, "response": {token}}
	req, err := http.NewRequestWithContext(ctx, "POST", s.SiteverifyURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.client.Do(req)
	if err != nil {
		return fail("CAPTCHA_UNAVAILABLE", "验证码服务暂不可用，请重新验证")
	}
	defer res.Body.Close()
	var result recaptchaResult
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 16384)).Decode(&result) != nil {
		return fail("CAPTCHA_UNAVAILABLE", "验证码服务暂不可用，请重新验证")
	}
	if !result.Success {
		for _, code := range result.ErrorCodes {
			if code == "missing-input-secret" || code == "invalid-input-secret" {
				return fail("CAPTCHA_UNAVAILABLE", "验证码服务配置错误，请联系管理员")
			}
		}
		return fail("CAPTCHA_FAILED", "验证码无效、已过期或已使用，请重新验证")
	}
	// 此站点已停用域名验证；当前复选框不发送 action，不匹配来源或用途。
	return nil
}
func (s *Server) sign(id int64, role string, duration time.Duration, versions ...uint64) string {
	binding := ""
	if role == "player" {
		binding = s.engine.SessionBinding(id)
	}
	version := s.engine.SessionVersion(id)
	if len(versions) > 0 {
		version = versions[0]
	}
	b, _ := json.Marshal(session{ID: id, Role: role, Expires: time.Now().Add(duration).Unix(), Binding: binding, Version: version})
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Server) authenticate(r *http.Request, role string) (int64, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(token) > 1024 {
		return 0, fail("UNAUTHORIZED", "请重新登录")
	}
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, fail("UNAUTHORIZED", "请重新登录")
	}
	var v session
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(b, &v) != nil || v.Role != role || v.Expires <= time.Now().Unix() {
		return 0, fail("UNAUTHORIZED", "登录已过期，请重新登录")
	}
	if role == "player" && !s.engine.PlayerExists(v.ID) {
		return 0, fail("UNAUTHORIZED", "账户已停用")
	}
	if role == "player" && v.Version != s.engine.SessionVersion(v.ID) {
		return 0, fail("UNAUTHORIZED", "账户凭据已更新，请重新登录")
	}
	if role == "player" && (v.Binding == "" || v.Binding != s.engine.SessionBinding(v.ID) || r.Header.Get("X-MMEX-Instance") != v.Binding) {
		return 0, fail("INSTANCE_MISMATCH", "交易会话与当前 AzurPilot 实例不一致，请重新登录")
	}
	return v.ID, nil
}
func (s *Server) player(fn func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.authenticate(r, "player")
		if err != nil {
			s.error(w, 401, err)
			return
		}
		fn(w, r, id)
	}
}
func (s *Server) admin(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.authenticate(r, "admin"); err != nil {
			s.error(w, 401, err)
			return
		}
		fn(w, r)
	}
}
func (s *Server) authSlot(w http.ResponseWriter, r *http.Request) bool {
	select {
	case s.Slots <- struct{}{}:
		return true
	default:
		s.error(w, 503, fail("BUSY", "认证繁忙，请稍后重试"))
		return false
	}
}
func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	s.json(w, 200, map[string]any{"name": "茗喵证券交易所", "domain": s.config.Domain, "mock": s.config.Mock, "allowedOrigins": s.config.Origins, "initialCash": s.engine.InitialFunding(), "noticeVersion": "2026-10-03", "nativeAzurPilot": true})
}
func (s *Server) passwordSlot(w http.ResponseWriter) bool {
	select {
	case s.passwordSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "1")
		s.error(w, 503, fail("BUSY", "认证繁忙，请稍后重试"))
		return false
	}
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string         `json:"username"`
		Password string         `json:"password"`
		Token    string         `json:"recaptchaToken"`
		Accepted string         `json:"acceptedNotice"`
		Report   InstanceReport `json:"report"`
	}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	if !s.authSlot(w, r) {
		return
	}
	defer func() { <-s.Slots }()
	if in.Accepted != "2026-10-03" {
		s.error(w, 400, fail("NOTICE_REQUIRED", "请阅读并同意最新注意事项"))
		return
	}
	if err := s.verify(r.Context(), in.Token); err != nil {
		s.error(w, 400, err)
		return
	}
	v, err := verifyInstance(in.Report, s.engine.clock())
	if err != nil {
		s.error(w, 400, err)
		return
	}
	if !s.passwordSlot(w) {
		return
	}
	defer func() { <-s.passwordSlots }()
	p, upload, err := s.engine.Register(in.Username, in.Password, in.Report.ActionPoints, in.Report.ObservedAt, v)
	if err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 201, map[string]any{"token": s.sign(p.ID, "player", 24*time.Hour, p.SessionVersion), "uploadToken": upload, "player": p})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string         `json:"username"`
		Password string         `json:"password"`
		Token    string         `json:"recaptchaToken"`
		Report   InstanceReport `json:"report"`
	}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	if !s.authSlot(w, r) {
		return
	}
	defer func() { <-s.Slots }()
	if err := s.verify(r.Context(), in.Token); err != nil {
		s.error(w, 400, err)
		return
	}
	v, err := verifyInstance(in.Report, s.engine.clock())
	if err != nil {
		s.error(w, 400, err)
		return
	}
	if !s.passwordSlot(w) {
		return
	}
	defer func() { <-s.passwordSlots }()
	p, err := s.engine.Login(in.Username, in.Password)
	if err != nil {
		s.error(w, 401, err)
		return
	}
	authenticatedVersion := p.SessionVersion
	p, err = s.engine.BindInstance(p.ID, v)
	if err != nil {
		s.error(w, 403, err)
		return
	}
	if p.SessionVersion != authenticatedVersion {
		s.error(w, 401, fail("LOGIN_FAILED", "账户凭据已更新，请重新登录"))
		return
	}
	s.json(w, 200, map[string]any{"token": s.sign(p.ID, "player", 24*time.Hour, authenticatedVersion), "player": p})
}
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Token    string `json:"recaptchaToken"`
	}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	if !s.authSlot(w, r) {
		return
	}
	defer func() { <-s.Slots }()
	if err := s.verify(r.Context(), in.Token); err != nil {
		s.error(w, 400, err)
		return
	}
	a := sha256.Sum256([]byte(in.Password))
	b := sha256.Sum256([]byte(s.config.AdminPassword))
	if subtle.ConstantTimeCompare(a[:], b[:]) != 1 {
		s.error(w, 401, fail("LOGIN_FAILED", "管理员密码错误"))
		return
	}
	s.json(w, 200, map[string]string{"token": s.sign(0, "admin", 2*time.Hour)})
}
func (s *Server) market(w http.ResponseWriter, r *http.Request) {
	b, etag, err := s.engine.MarketJSON()
	if err != nil {
		s.error(w, 500, err)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(304)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}
func matchesETag(header, etag string) bool {
	// nginx 压缩会将 ETag 标为弱校验；GET 仍可复用同一份行情缓存。
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("stock"), 10, 64)
	if err != nil {
		s.error(w, 400, fail("INVALID_ID", "股票标识无效"))
		return
	}
	v, err := s.engine.History(id)
	if err != nil {
		s.error(w, 500, err)
		return
	}
	s.json(w, 200, v)
}
func (s *Server) seasons(w http.ResponseWriter, r *http.Request) {
	v, err := s.engine.Seasons()
	if err != nil {
		s.error(w, 500, err)
		return
	}
	s.json(w, 200, v)
}
func (s *Server) account(w http.ResponseWriter, r *http.Request, id int64) {
	v, err := s.engine.Account(id)
	if err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, v)
}
func (s *Server) rotate(w http.ResponseWriter, r *http.Request, id int64) {
	v, err := s.engine.RotateUpload(id)
	if err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, map[string]string{"uploadToken": v})
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Report InstanceReport `json:"report"`
	}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	v, err := verifyInstance(in.Report, s.engine.clock())
	if err != nil {
		s.error(w, 400, err)
		return
	}
	if err := s.engine.Upload(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), in.Report.ActionPoints, in.Report.ObservedAt, v); err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, map[string]bool{"ok": true})
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request, id int64) {
	var in OrderInput
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	v, err := s.engine.Submit(id, in)
	if err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 201, v)
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request, id int64) {
	order, err := strconv.ParseInt(r.PathValue("order"), 10, 64)
	if err != nil {
		s.error(w, 400, fail("INVALID_ID", "委托标识无效"))
		return
	}
	if err = s.engine.Cancel(id, order); err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, map[string]bool{"ok": true})
}
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	s.json(w, 200, s.engine.Settings())
}
func (s *Server) setSettings(w http.ResponseWriter, r *http.Request) {
	in := Settings{DelistThreshold: s.engine.Settings().DelistThreshold}
	if err := decode(w, r, &in); err != nil {
		s.error(w, 400, err)
		return
	}
	if err := s.engine.SetSettings(in); err != nil {
		s.error(w, 400, err)
		return
	}
	s.json(w, 200, s.engine.Settings())
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Clean(r.URL.Path)
	if strings.HasPrefix(path, "/api/") {
		http.NotFound(w, r)
		return
	}
	name := filepath.Join(s.config.Frontend, filepath.FromSlash(strings.TrimPrefix(path, "/")))
	info, err := os.Stat(name)
	if err != nil || info.IsDir() {
		if strings.HasPrefix(path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		name = filepath.Join(s.config.Frontend, "index.html")
	} else if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	}
	http.ServeFile(w, r, name)
}
