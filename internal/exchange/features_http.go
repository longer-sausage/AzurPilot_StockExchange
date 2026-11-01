package exchange

import (
	"net/http"
	"strconv"
	"strings"
)

func matchesETag(header, tag string) bool {
	for _, v := range strings.Split(header, ",") {
		v = strings.TrimSpace(v)
		if v == "*" || strings.TrimPrefix(v, "W/") == tag {
			return true
		}
	}
	return false
}
func (s *Server) seasons(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.Tick(); err != nil {
		s.result(w, r, nil, err)
		return
	}
	s.json(w, r, 200, s.engine.Seasons())
}
func (s *Server) stockDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	from, to := int64(0), s.engine.clock().UnixMilli()
	for name, p := range map[string]*int64{"from": &from, "to": &to} {
		if v := r.URL.Query().Get(name); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				s.result(w, r, nil, fail("INVALID_RANGE", "时间须为非负整数毫秒"))
				return
			}
			*p = n
		}
	}
	if from > to {
		s.result(w, r, nil, fail("INVALID_RANGE", "时间范围无效"))
		return
	}
	out, err := s.engine.StockData(id, from, to)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	months, err := s.engine.store.Manifest(id)
	if err != nil {
		s.result(w, r, nil, err)
		return
	}
	out["months"] = months
	s.json(w, r, 200, out)
}
func (s *Server) watchlist(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	v, _ := s.getSession(r, "player")
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.result(w, r, map[string]bool{"ok": true}, s.engine.SetWatchlist(v.ID, id, in.Enabled))
}
func (s *Server) deleteStock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirmed bool `json:"confirmed"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	if !in.Confirmed {
		s.result(w, r, nil, fail("CONFIRMATION_REQUIRED", "必须确认删除、平仓及解除绑定"))
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.result(w, r, map[string]bool{"ok": true}, s.engine.DeleteStock(id))
}
func (s *Server) adminAccount(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	out, err := s.engine.Portfolio(id)
	s.result(w, r, out, err)
}
