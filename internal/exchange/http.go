package exchange

import (
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Domain        string
	Listen        string
	Database      string
	Frontend      string
	AdminPassword string
	SessionSecret string
	BotAPIToken   string
	DefaultsFile  string
	Origins       []string
	SecureCookies bool
}

func LoadConfig() (Config, error) {
	get := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	c := Config{Domain: get("EXCHANGE_DOMAIN", "stock.azurpilot.com"), Listen: get("LISTEN_ADDR", "127.0.0.1:8888"), Database: os.Getenv("DATABASE_URL"), Frontend: get("FRONTEND_DIR", "frontend/dist"), AdminPassword: os.Getenv("ADMIN_PASSWORD"), SessionSecret: os.Getenv("SESSION_SECRET"), BotAPIToken: os.Getenv("BOT_API_TOKEN"), DefaultsFile: get("DEFAULTS_FILE", "config/defaults.json"), Origins: strings.Split(os.Getenv("ALLOWED_ORIGINS"), ","), SecureCookies: os.Getenv("COOKIE_SECURE") != "false"}
	if len(c.AdminPassword) < 16 || len(c.SessionSecret) < 48 || len(c.BotAPIToken) < 48 || !strings.HasPrefix(c.Database, "postgres") {
		return c, fmt.Errorf("需配置外部 Postgres DATABASE_URL、至少 16 字符 ADMIN_PASSWORD、至少 48 字符 SESSION_SECRET 与 BOT_API_TOKEN")
	}
	if strings.ContainsAny(c.Domain, "/\r\n \t;") {
		return c, fmt.Errorf("EXCHANGE_DOMAIN 无效")
	}
	return c, nil
}

type session struct {
	ID       int64  `json:"id"`
	Role     string `json:"role"`
	Version  int64  `json:"version"`
	Expires  int64  `json:"expires"`
	Remember bool   `json:"remember"`
}
type Server struct {
	engine        *Engine
	config        Config
	mux           *http.ServeMux
	passwordSlots chan struct{}
}

func NewServer(e *Engine, c Config) *Server {
	s := &Server{engine: e, config: c, mux: http.NewServeMux(), passwordSlots: make(chan struct{}, max(1, runtime.GOMAXPROCS(0)))}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		s.json(w, r, 200, map[string]any{"ok": true, "apiVersion": 2})
	})
	s.mux.HandleFunc("GET /api/v2/public/market", s.market)
	s.mux.HandleFunc("GET /api/v2/public/rules", s.rules)
	s.mux.HandleFunc("GET /api/v2/public/events", s.events)
	s.mux.HandleFunc("GET /api/v2/public/stocks/{id}/chart", s.chart)
	s.mux.HandleFunc("GET /api/v2/public/seasons", s.seasons)
	s.mux.HandleFunc("GET /api/v2/public/stocks/{id}/detail", s.stockDetail)
	s.mux.HandleFunc("GET /api/v2/public/stocks/{id}/chart-detail", s.chartDetail)
	s.mux.HandleFunc("POST /api/v2/auth/login", s.passwordAdmission(s.login))
	s.mux.HandleFunc("GET /api/v2/auth/me", s.player(false, s.me))
	s.mux.HandleFunc("POST /api/v2/auth/password", s.player(false, s.passwordAdmission(s.password)))
	s.mux.HandleFunc("POST /api/v2/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s.clearCookie(w, "player")
		s.json(w, r, 200, map[string]bool{"ok": true})
	})
	s.mux.HandleFunc("GET /api/v2/account", s.player(true, s.account))
	s.mux.HandleFunc("POST /api/v2/orders/preview", s.player(true, s.preview))
	s.mux.HandleFunc("POST /api/v2/orders/confirm", s.player(true, s.confirm))
	s.mux.HandleFunc("DELETE /api/v2/orders/{id}", s.player(true, s.cancel))
	s.mux.HandleFunc("POST /api/v2/admin/login", s.passwordAdmission(s.adminLogin))
	s.mux.HandleFunc("POST /api/v2/admin/logout", func(w http.ResponseWriter, r *http.Request) {
		s.clearCookie(w, "admin")
		s.json(w, r, 200, map[string]bool{"ok": true})
	})
	s.mux.HandleFunc("GET /api/v2/admin/me", s.admin(func(w http.ResponseWriter, r *http.Request) { s.json(w, r, 200, map[string]bool{"ok": true}) }))
	s.mux.HandleFunc("GET /api/v2/admin/settings", s.admin(func(w http.ResponseWriter, r *http.Request) { s.json(w, r, 200, e.Settings()) }))
	s.mux.HandleFunc("PUT /api/v2/admin/settings", s.admin(s.setSettings))
	s.mux.HandleFunc("GET /api/v2/admin/accounts", s.admin(func(w http.ResponseWriter, r *http.Request) {
		s.json(w, r, 200, map[string]any{"accounts": e.AdminAccounts()})
	}))
	s.mux.HandleFunc("PUT /api/v2/admin/accounts/{id}", s.admin(s.updateAccount))
	s.mux.HandleFunc("GET /api/v2/admin/stocks", s.admin(func(w http.ResponseWriter, r *http.Request) {
		s.json(w, r, 200, map[string]any{"stocks": e.AdminStocks()})
	}))
	s.mux.HandleFunc("PUT /api/v2/admin/stocks/{id}", s.admin(s.updateStock))
	s.mux.HandleFunc("POST /api/v2/admin/stocks/{id}/status", s.admin(s.stockStatus))
	s.mux.HandleFunc("POST /api/v2/admin/accounts/{id}/reset-password", s.admin(s.passwordAdmission(s.adminReset)))
	s.mux.HandleFunc("PUT /api/v2/account/watchlist/{id}", s.player(true, s.watchlist))
	s.mux.HandleFunc("DELETE /api/v2/admin/stocks/{id}", s.admin(s.deleteStock))
	s.mux.HandleFunc("GET /api/v2/admin/accounts/{id}", s.admin(s.adminAccount))
	s.mux.HandleFunc("POST /api/v2/bot/register", s.bot(s.passwordAdmission(s.botRegister)))
	s.mux.HandleFunc("POST /api/v2/bot/reset-password", s.bot(s.passwordAdmission(s.botReset)))
	s.mux.HandleFunc("GET /api/v2/bot/portfolio", s.bot(s.botPortfolio))
	s.mux.HandleFunc("GET /api/v2/bot/stock", s.bot(s.botStock))
	s.mux.HandleFunc("POST /api/v2/bot/bind", s.bot(s.botBind))
	s.mux.HandleFunc("POST /api/v2/bot/orders/preview", s.bot(s.botPreview))
	s.mux.HandleFunc("POST /api/v2/bot/orders/confirm", s.bot(s.botConfirm))
	for _, action := range []string{"register", "status", "report", "manifest", "reconcile"} {
		s.mux.HandleFunc("POST /api/v2/instance/"+action, s.instance(action))
	}
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		s.error(w, r, 404, fail("NOT_FOUND", "API 路径不存在，请升级客户端至 v2"))
	})
	s.mux.HandleFunc("/", s.static)
	return s
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	origin := r.Header.Get("Origin")
	if origin != "" {
		allowed := origin == "https://"+s.config.Domain
		for _, v := range s.config.Origins {
			if origin == strings.TrimSpace(v) {
				allowed = true
			}
		}
		if !allowed {
			s.error(w, r, 403, fail("ORIGIN_DENIED", "访问来源未授权"))
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Vary", "Origin")
	}
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,If-None-Match")
		w.WriteHeader(204)
		return
	}
	s.mux.ServeHTTP(w, r)
}
func (s *Server) json(w http.ResponseWriter, r *http.Request, status int, value any) {
	s.bytes(w, r, status, encode(value))
}
func (s *Server) bytes(w http.ResponseWriter, r *http.Request, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(data) > 512 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.WriteHeader(status)
		zw, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		_, _ = zw.Write(data)
		_ = zw.Close()
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
func (s *Server) error(w http.ResponseWriter, r *http.Request, status int, err error) {
	f := publicError(err)
	if f.Code == "INTERNAL" {
		slog.Error("API 持久化或内部错误", "method", r.Method, "path", r.URL.Path)
	}
	s.json(w, r, status, map[string]any{"error": f})
}
func (s *Server) result(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		status := 400
		code := publicError(err).Code
		if code == "INTERNAL" {
			status = 500
		} else if code == "UNAUTHORIZED" || code == "INVALID_CREDENTIALS" {
			status = 401
		} else if code == "STOCK_DELETED" {
			status = 410
		} else if code == "NOT_FOUND" || code == "STOCK_NOT_REGISTERED" {
			status = 404
		}
		s.error(w, r, status, err)
		return
	}
	s.json(w, r, 200, value)
}
func (s *Server) decode(w http.ResponseWriter, r *http.Request, out any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		s.error(w, r, 400, fail("INVALID_JSON", "请求 JSON 无效或包含未知字段"))
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		s.error(w, r, 400, fail("INVALID_JSON", "请求只能包含一个 JSON 对象"))
		return false
	}
	return true
}
func (s *Server) passwordAdmission(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.passwordSlots <- struct{}{}:
			defer func() { <-s.passwordSlots }()
			next(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			s.error(w, r, 503, fail("AUTH_BUSY", "密码校验繁忙，请稍后重试"))
		}
	}
}
func (s *Server) setCookie(w http.ResponseWriter, v session) {
	body := base64.RawURLEncoding.EncodeToString(encode(v))
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	mac.Write([]byte(body))
	token := body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	c := &http.Cookie{Name: "mmex_" + v.Role, Value: token, Path: "/api/v2", HttpOnly: true, Secure: s.config.SecureCookies, SameSite: http.SameSiteStrictMode}
	if v.Remember {
		c.MaxAge = 30 * 86400
		c.Expires = time.Unix(v.Expires, 0)
	}
	http.SetCookie(w, c)
}
func (s *Server) clearCookie(w http.ResponseWriter, role string) {
	http.SetCookie(w, &http.Cookie{Name: "mmex_" + role, Value: "", Path: "/api/v2", MaxAge: -1, HttpOnly: true, Secure: s.config.SecureCookies, SameSite: http.SameSiteStrictMode})
}
func (s *Server) getSession(r *http.Request, role string) (session, error) {
	var v session
	c, err := r.Cookie("mmex_" + role)
	if err != nil {
		return v, fail("UNAUTHORIZED", "请先登录")
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return v, fail("UNAUTHORIZED", "登录无效")
	}
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return v, fail("UNAUTHORIZED", "登录无效")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(raw, &v) != nil || v.Role != role || v.Expires < s.engine.clock().Unix() {
		return v, fail("UNAUTHORIZED", "登录已过期")
	}
	if role == "player" {
		if _, err = s.engine.CheckSession(v.ID, v.Version); err != nil {
			return v, err
		}
	}
	return v, nil
}
func (s *Server) player(ready bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := s.getSession(r, "player")
		if err != nil {
			s.result(w, r, nil, err)
			return
		}
		a, err := s.engine.CheckSession(v.ID, v.Version)
		if err != nil {
			s.result(w, r, nil, err)
			return
		}
		if ready {
			if err := s.engine.Ready(a.ID); err != nil {
				s.result(w, r, nil, err)
				return
			}
		}
		next(w, r)
	}
}
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.getSession(r, "admin"); err != nil {
			s.result(w, r, nil, err)
			return
		}
		next(w, r)
	}
}
func (s *Server) bot(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if s.config.BotAPIToken == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(s.config.BotAPIToken)) != 1 {
			s.error(w, r, 401, fail("UNAUTHORIZED", "机器人服务凭据无效"))
			return
		}
		next(w, r)
	}
}
func (s *Server) market(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.Tick(); err != nil {
		s.result(w, r, nil, err)
		return
	}
	data, version := s.engine.Market()
	tag := fmt.Sprintf("\"v2-%d\"", version)
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(304)
		return
	}
	s.bytes(w, r, 200, data)
}
func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	data, tag := s.engine.RulesData()
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(304)
		return
	}
	s.bytes(w, r, 200, data)
}
func (s *Server) chart(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	now := s.engine.clock().UnixMilli()
	from, to, interval := now-30*86400000, now, int64(60)
	var err error
	for name, p := range map[string]*int64{"from": &from, "to": &to, "interval": &interval} {
		if v := r.URL.Query().Get(name); v != "" {
			*p, err = strconv.ParseInt(v, 10, 64)
			if err != nil {
				s.result(w, r, nil, fail("INVALID_RANGE", "图表参数须为整数"))
				return
			}
		}
	}
	tag, err := s.engine.ChartTag(id, from, to, interval)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(304)
		return
	}
	points, actual, err := s.engine.cachedChart(id, from, to, interval)
	s.result(w, r, map[string]any{"stockId": id, "interval": actual, "points": points}, err)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.error(w, r, 500, fail("INTERNAL", "SSE 不可用"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	last := uint64(0)
	for {
		version, changes := s.engine.Changes()
		if version != last {
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprintf(w, "event: change\ndata: {\"version\":%d}\n\n", version); err != nil {
				return
			}
			flusher.Flush()
			last = version
		}
		select {
		case <-r.Context().Done():
			return
		case <-changes:
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	id, version, force, err := s.engine.Login(in.Username, in.Password)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	seconds := int64(86400)
	if in.Remember {
		seconds = 30 * 86400
	}
	s.setCookie(w, session{ID: id, Role: "player", Version: version, Expires: s.engine.clock().Unix() + seconds, Remember: in.Remember})
	account, err := s.engine.WatchAccount(id)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	s.json(w, r, 200, map[string]any{"account": account, "forcePasswordChange": force})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	v, _ := s.getSession(r, "player")
	account, err := s.engine.WatchAccount(v.ID)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	a, _ := s.engine.CheckSession(v.ID, v.Version)
	s.json(w, r, 200, map[string]any{"account": account, "forcePasswordChange": a.ForcePasswordChange})
}
func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, _ := s.getSession(r, "player")
	version, err := s.engine.ChangePassword(v.ID, in.CurrentPassword, in.Password)
	if err == nil {
		v.Version = version
		s.setCookie(w, v)
	}
	s.result(w, r, map[string]bool{"ok": true}, err)
}
func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	v, _ := s.getSession(r, "player")
	tag, err := s.engine.PortfolioTag(v.ID)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(304)
		return
	}
	out, err := s.engine.Portfolio(v.ID)
	s.result(w, r, out, err)
}
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var in OrderInput
	if !s.decode(w, r, &in) {
		return
	}
	v, _ := s.getSession(r, "player")
	out, err := s.engine.Preview(v.ID, in)
	s.result(w, r, out, err)
}
func (s *Server) confirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PreviewID string `json:"previewId"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, _ := s.getSession(r, "player")
	out, err := s.engine.Confirm(v.ID, in.PreviewID)
	s.result(w, r, map[string]any{"order": out}, err)
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	v, _ := s.getSession(r, "player")
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	out, err := s.engine.Cancel(v.ID, id)
	s.result(w, r, map[string]any{"order": out}, err)
}
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	given := sha256.Sum256([]byte(in.Password))
	expected := sha256.Sum256([]byte(s.config.AdminPassword))
	if subtle.ConstantTimeCompare(given[:], expected[:]) != 1 {
		s.error(w, r, 401, fail("INVALID_CREDENTIALS", "管理员密码错误"))
		return
	}
	seconds := int64(86400)
	if in.Remember {
		seconds = 30 * 86400
	}
	s.setCookie(w, session{Role: "admin", Expires: s.engine.clock().Unix() + seconds, Remember: in.Remember})
	s.json(w, r, 200, map[string]bool{"ok": true})
}
func (s *Server) setSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if !s.decode(w, r, &in) {
		return
	}
	s.result(w, r, in, s.engine.SetSettings(in))
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	var in AccountEdit
	if !s.decode(w, r, &in) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.result(w, r, map[string]bool{"ok": true}, s.engine.EditAccount(id, in))
}

func (s *Server) updateStock(w http.ResponseWriter, r *http.Request) {
	var in Stock
	if !s.decode(w, r, &in) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.result(w, r, map[string]bool{"ok": true}, s.engine.UpdateStock(id, in))
}
func (s *Server) botRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OfficialUserID string `json:"officialUserId"`
		Username       string `json:"username"`
		RequestID      string `json:"requestId"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.engine.BotRegister(in.OfficialUserID, in.Username, in.RequestID, s.config.SessionSecret)
	s.result(w, r, out, err)
}
func (s *Server) botReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OfficialUserID string `json:"officialUserId"`
		Confirmed      bool   `json:"confirmed"`
		RequestID      string `json:"requestId"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		s.result(w, r, nil, fail("CONFIRMATION_REQUIRED", "必须先二次确认"))
		return
	}
	password, err := s.engine.ResetPassword(in.OfficialUserID, in.RequestID, s.config.SessionSecret)
	s.result(w, r, map[string]any{"password": password, "forcePasswordChange": true}, err)
}
func (s *Server) botPortfolio(w http.ResponseWriter, r *http.Request) {
	id, err := s.engine.BotAccount(r.URL.Query().Get("officialUserId"))
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	if err = s.engine.Ready(id); err != nil {
		s.result(w, r, nil, err)
		return
	}
	out, err := s.engine.Portfolio(id)
	s.result(w, r, out, err)
}
func (s *Server) botStock(w http.ResponseWriter, r *http.Request) {
	id, err := s.engine.BotAccount(r.URL.Query().Get("officialUserId"))
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	if err = s.engine.Ready(id); err != nil {
		s.result(w, r, nil, err)
		return
	}
	out, err := s.engine.Stock(r.URL.Query().Get("query"), id)
	s.result(w, r, map[string]any{"stock": out}, err)
}
func (s *Server) botBind(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OfficialUserID string `json:"officialUserId"`
		BindingCode    string `json:"bindingCode"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	id, err := s.engine.BotAccount(in.OfficialUserID)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	out, err := s.engine.Bind(id, in.BindingCode)
	s.result(w, r, map[string]any{"stock": out}, err)
}
func (s *Server) botPreview(w http.ResponseWriter, r *http.Request) {
	var in OrderInput
	if !s.decode(w, r, &in) {
		return
	}
	id, err := s.engine.BotAccount(in.OfficialUserID)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	out, err := s.engine.Preview(id, in)
	s.result(w, r, out, err)
}
func (s *Server) botConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OfficialUserID string `json:"officialUserId"`
		PreviewID      string `json:"previewId"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	id, err := s.engine.BotAccount(in.OfficialUserID)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	out, err := s.engine.Confirm(id, in.PreviewID)
	s.result(w, r, map[string]any{"order": out}, err)
}
func (s *Server) instance(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in InstanceRequest
		if !s.decode(w, r, &in) {
			return
		}
		var out any
		var err error
		switch action {
		case "register":
			out, err = s.engine.InstanceRegister(in)
		case "status":
			out, err = s.engine.InstanceStatus(in)
		case "report":
			out, err = s.engine.InstanceReport(in)
		case "reconcile":
			out, err = s.engine.InstanceReconcile(in)
		case "manifest":
			var months []Month
			months, err = s.engine.InstanceManifest(in)
			out = map[string]any{"months": months}
		}
		s.result(w, r, out, err)
	}
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	path := filepath.Clean("/" + r.URL.Path)
	file := filepath.Join(s.config.Frontend, filepath.FromSlash(path))
	info, err := os.Stat(file)
	if strings.HasPrefix(r.URL.Path, "/assets/") && (err != nil || info.IsDir()) {
		http.NotFound(w, r)
		return
	}
	if err != nil || info.IsDir() {
		file = filepath.Join(s.config.Frontend, "index.html")
		info, err = os.Stat(file)
	}
	if err != nil {
		http.Error(w, "前端尚未部署", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFile(w, r, file)
	_ = info
}
