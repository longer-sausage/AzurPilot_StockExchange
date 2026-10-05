package exchange

import (
	"net/http"
	"strconv"
)

func (s *Server) listPlayers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, pageSize := 1, 20
	for key, target := range map[string]*int{"page": &page, "pageSize": &pageSize} {
		if q.Has(key) {
			v, err := strconv.Atoi(q.Get(key))
			if err != nil || v < 1 || key == "page" && v > 1_000_000 || key == "pageSize" && v > 100 {
				s.error(w, 400, fail("INVALID_QUERY", "页码须为 1–1000000，单页条数须为 1–100"))
				return
			}
			*target = v
		}
	}
	status := q.Get("status")
	if len([]rune(q.Get("q"))) > 128 || status != "" && status != "all" && status != "active" && status != "disabled" {
		s.error(w, 400, fail("INVALID_QUERY", "搜索词过长或账户状态无效"))
		return
	}
	s.json(w, 200, s.engine.AdminPlayers(q.Get("q"), status, page, pageSize))
}

func playerPathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("player"), 10, 64)
	if err != nil || id < 1 {
		return 0, fail("INVALID_ID", "账户标识无效")
	}
	return id, nil
}

func (s *Server) playerError(w http.ResponseWriter, err error) {
	status := 400
	switch publicError(err).Code {
	case "NOT_FOUND":
		status = 404
	case "CASH_CHANGED":
		status = 409
	}
	s.error(w, status, err)
}

func (s *Server) getPlayer(w http.ResponseWriter, r *http.Request) {
	id, err := playerPathID(r)
	if err != nil {
		s.playerError(w, err)
		return
	}
	v, err := s.engine.Account(id)
	if err != nil {
		s.playerError(w, err)
		return
	}
	v.Player.Orders, err = s.engine.orderHistory(id)
	if err != nil {
		s.playerError(w, err)
		return
	}
	s.json(w, 200, v)
}

func (s *Server) updatePlayer(w http.ResponseWriter, r *http.Request) {
	id, err := playerPathID(r)
	if err != nil {
		s.playerError(w, err)
		return
	}
	var in AdminPlayerUpdate
	if err := decode(w, r, &in); err != nil {
		s.playerError(w, err)
		return
	}
	if in.Password != nil {
		if !s.passwordSlot(w) {
			return
		}
		defer func() { <-s.passwordSlots }()
	}
	v, err := s.engine.UpdatePlayer(id, in)
	if err != nil {
		s.playerError(w, err)
		return
	}
	s.json(w, 200, v)
}
