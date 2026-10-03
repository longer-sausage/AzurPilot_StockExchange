package exchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIdentityCodesMigrateAndSurviveAccountChanges(t *testing.T) {
	f := setup(t)
	ownerCode, traderCode := f.owner.IdentityCode, f.trader.IdentityCode
	if !identityCodePattern.MatchString(ownerCode) || !identityCodePattern.MatchString(traderCode) || ownerCode == traderCode {
		t.Fatal("新账户须获得不同且有效的身份码")
	}
	// 模拟升级前的数据库：两个旧账户都没有身份码字段和对应索引。
	if _, err := f.e.db.Exec("DELETE FROM player_identity_codes"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.db.Exec("UPDATE players SET data=json_remove(data,'$.player.identityCode')"); err != nil {
		t.Fatal(err)
	}
	f.e.Close()
	var err error
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := f.e.Account(f.owner.ID)
	trader, _ := f.e.Account(f.trader.ID)
	ownerCode, traderCode = owner.Player.IdentityCode, trader.Player.IdentityCode
	if !identityCodePattern.MatchString(ownerCode) || !identityCodePattern.MatchString(traderCode) || ownerCode == traderCode {
		t.Fatal("迁移须为所有旧账户补发唯一身份码")
	}
	if _, err = f.e.db.Exec("UPDATE player_identity_codes SET code=? WHERE player_id=?", ownerCode, f.trader.ID); err == nil {
		t.Fatal("数据库须拒绝重复身份码")
	}
	name, password, disabled := "改名后的猫", "replacement-password", true
	if _, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Username: &name, Password: &password, Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	disabled = false
	if err = f.e.Disable(f.trader.ID, disabled); err != nil {
		t.Fatal(err)
	}
	f.now = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err = f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	f.e.Close()
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	trader, _ = f.e.Account(f.trader.ID)
	if trader.Player.IdentityCode != traderCode || trader.Player.SessionVersion != 2 || trader.Player.Username != name {
		t.Fatal("改名、密码重设、停用恢复、赛季重置和重启不得改变身份码，凭据版本需保留")
	}
	if _, err = f.e.Login(name, password); err != nil {
		t.Fatal("重启后新用户名和新密码不可用", err)
	}
	if _, err = f.e.Login(f.trader.Username, password); err == nil {
		t.Fatal("旧用户名不得继续登录")
	}
	// JSON 字段遗失时使用已保存的唯一索引恢复，避免重新发码。
	if _, err = f.e.db.Exec("UPDATE players SET data=json_remove(data,'$.player.identityCode') WHERE id=?", f.trader.ID); err != nil {
		t.Fatal(err)
	}
	f.e.Close()
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	trader, _ = f.e.Account(f.trader.ID)
	if trader.Player.IdentityCode != traderCode {
		t.Fatal("恢复身份码时不应重新发码")
	}
}

func TestAdminUpdatesValidateAndRollback(t *testing.T) {
	f := setup(t)
	duplicate := "股东猫猫"
	_, err := f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Username: &duplicate})
	requireCode(t, err, "USERNAME_TAKEN")
	invalidName, shortPassword := "bad name", "short"
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Username: &invalidName})
	requireCode(t, err, "INVALID_USERNAME")
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Password: &shortPassword})
	requireCode(t, err, "INVALID_PASSWORD")
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{})
	requireCode(t, err, "INVALID_UPDATE")
	name, credit, cash := "ＡＢＣ猫", int64(12345), f.trader.Cash
	updated, err := f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Username: &name, CashAdjustment: &credit, ExpectedCash: &cash})
	if err != nil || updated.Player.Username != "ABC猫" || updated.Player.Cash != cash+credit || updated.Player.InitialCash != f.trader.InitialCash {
		t.Fatal("改名和现金调整须原子提交并保留赛季本金", err)
	}
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{CashAdjustment: &credit, ExpectedCash: &cash})
	requireCode(t, err, "CASH_CHANGED")
	if _, err = f.e.Login("abc猫", "test-password-123"); err != nil {
		t.Fatal("更改后的名称仍须全角归一与大小写折叠", err)
	}
	_, _, err = f.e.Register("ａｂｃ猫", "test-password-123", 1, f.now.Unix())
	requireCode(t, err, "USERNAME_TAKEN")
	if f.e.names[f.trader.Folded] != 0 {
		t.Fatal("改名后须清除旧名称索引")
	}
	before, _ := f.e.Account(f.trader.ID)
	revision := f.e.state.Revision
	f.e.db.Close()
	name, password, disabled := "磁盘回滚猫", "new-password-123", true
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Username: &name, Password: &password, Disabled: &disabled})
	if err == nil {
		t.Fatal("数据库失效不得保存")
	}
	after, _ := f.e.Account(f.trader.ID)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if !bytes.Equal(beforeJSON, afterJSON) || after.Player.SessionVersion != before.Player.SessionVersion || f.e.state.Revision != revision || f.e.names[name] != 0 {
		t.Fatal("保存失败须完整恢复账户、凭据版本和索引")
	}
}

func TestCashAdjustmentProtectsOrdersMarginAndSettlements(t *testing.T) {
	f := setup(t)
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 100, 50000, "admin_pending_1")); err != nil {
		t.Fatal(err)
	}
	account, _ := f.e.Account(f.trader.ID)
	debit, cash := -account.Available-1, account.Player.Cash
	_, err := f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{CashAdjustment: &debit, ExpectedCash: &cash})
	requireCode(t, err, "CASH_IN_USE")
	debit = -account.Available
	updated, err := f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{CashAdjustment: &debit, ExpectedCash: &cash})
	if err != nil || updated.Available != 0 || updated.Player.Orders[0].Status != "pending" {
		t.Fatal("仅扣减可用资金时应保留冻结委托", err)
	}
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{CashAdjustment: &debit})
	requireCode(t, err, "INVALID_CASH")
	debit = -MaxNotional
	cash = updated.Player.Cash
	_, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{CashAdjustment: &debit, ExpectedCash: &cash})
	requireCode(t, err, "INVALID_CASH")
	// 交收资金和空头保证金均不能被管理员误扣为可用现金。
	g := setup(t)
	if _, err = g.e.Submit(g.trader.ID, order(g, "short", "market", 100, 0, "admin_short_001")); err != nil {
		t.Fatal(err)
	}
	if err = g.e.transaction(func() error {
		p := g.e.touch(g.trader.ID)
		p.Settlements = []Settlement{{100000, g.now.Add(time.Hour).Unix()}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	account, _ = g.e.Account(g.trader.ID)
	debit, cash = -account.Available-1, account.Player.Cash
	_, err = g.e.UpdatePlayer(g.trader.ID, AdminPlayerUpdate{CashAdjustment: &debit, ExpectedCash: &cash})
	requireCode(t, err, "CASH_IN_USE")
}

func TestAdminSearchPaginationAndCacheInvalidation(t *testing.T) {
	f := setup(t)
	for i := 0; i < 22; i++ {
		if _, _, err := f.e.Register(fmt.Sprintf("筛选猫%02d", i), "test-password-123", 1000, f.now.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	list := f.e.AdminPlayers("筛选猫", "active", 2, 20)
	if list.Total != 22 || len(list.Players) != 2 || list.Page != 2 {
		t.Fatal("用户筛选及分页错误", list)
	}
	if list = f.e.AdminPlayers(f.owner.IdentityCode, "all", 1, 20); list.Total != 1 || list.Players[0].ID != f.owner.ID {
		t.Fatal("管理员应能通过完整身份码核实用户")
	}
	if err := f.e.Disable(f.owner.ID, true); err != nil {
		t.Fatal(err)
	}
	list = f.e.AdminPlayers("", "disabled", 9, 20)
	if list.Total != 1 || list.Page != 1 || list.Players[0].Equity == 0 || list.Disabled != 1 || list.Active != 23 {
		t.Fatal("停用用户仍应有完整净值及状态，页码需校正")
	}
	if err := f.e.Disable(f.owner.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 1, 1, "rename_pending1")); err != nil {
		t.Fatal(err)
	}
	_, oldETag, err := f.e.StockDetailJSON(f.owner.ID, "time", "2026-10", "2026-10-06")
	if err != nil {
		t.Fatal(err)
	}
	name := "改名挂单猫"
	if _, err = f.e.UpdatePlayer(f.trader.ID, AdminPlayerUpdate{Username: &name}); err != nil {
		t.Fatal(err)
	}
	raw, newETag, err := f.e.StockDetailJSON(f.owner.ID, "time", "2026-10", "2026-10-06")
	if err != nil || oldETag == newETag || !strings.Contains(string(raw), name) {
		t.Fatal("用户改名须使公开挂单缓存失效", err)
	}
}

func TestUserManagementHTTPPrivacyAndCredentialRevocation(t *testing.T) {
	f := setup(t)
	identity := newTestIdentity(t)
	binding, _ := verifyInstance(identity.report(f.now, 1000, f.now.Unix()), f.now)
	if _, err := f.e.BindInstance(f.trader.ID, binding); err != nil {
		t.Fatal(err)
	}
	s := NewServer(f.e, Config{SessionSecret: "private-test-session-secret-32-characters"})
	admin, player := s.sign(0, "admin", time.Hour), s.sign(f.trader.ID, "player", time.Hour)
	request := func(method, path, token string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-MMEX-Instance", binding.Key)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/console/players", fmt.Sprintf("/api/console/players/%d", f.owner.ID)} {
		for _, token := range []string{"", player} {
			if w := request("GET", path, token, nil); w.Code != 401 || strings.Contains(w.Body.String(), f.owner.IdentityCode) {
				t.Fatal("匿名或普通用户不得查看其他用户身份码", w.Code)
			}
		}
		if w := request("GET", path, admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), f.owner.IdentityCode) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("管理员应能读取私有用户资料且不得缓存", w.Code)
		}
	}
	if w := request("GET", "/api/account?player=1", player, nil); w.Code != 200 || !strings.Contains(w.Body.String(), f.trader.IdentityCode) || strings.Contains(w.Body.String(), f.owner.IdentityCode) {
		t.Fatal("普通用户只能取得本人身份码", w.Code)
	}
	for _, path := range []string{"/api/market", "/api/seasons", "/api/history/1", "/api/stocks/1?period=time&month=2026-10&day=2026-10-06"} {
		w := request("GET", path, "", nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "identityCode") || strings.Contains(w.Body.String(), f.owner.IdentityCode) || strings.Contains(w.Body.String(), f.trader.IdentityCode) {
			t.Fatal("公开市场、排行榜、行情和成交记录不得泄漏身份码", path, w.Code)
		}
	}
	path := fmt.Sprintf("/api/console/players/%d", f.trader.ID)
	if w := request("PUT", path, player, map[string]any{"disabled": true}); w.Code != 401 {
		t.Fatal("普通用户不得修改管理资料")
	}
	for _, input := range []map[string]any{{"identityCode": f.owner.IdentityCode}, {"binding": binding}, {"cash": 0}, {}} {
		if w := request("PUT", path, admin, input); w.Code != 400 {
			t.Fatal("管理接口须拒绝改码、换绑、直接覆写现金及空请求", w.Code)
		}
	}
	if w := request("GET", "/api/console/players?page=0", admin, nil); w.Code != 400 {
		t.Fatal("分页参数须验证")
	}
	if w := request("GET", "/api/console/players/999999", admin, nil); w.Code != 404 {
		t.Fatal("不存在用户应返回 404")
	}
	if w := request("PUT", path, admin, map[string]any{"password": "replacement-password"}); w.Code != 200 || strings.Contains(w.Body.String(), "replacement-password") || strings.Contains(w.Body.String(), "Hash") || strings.Contains(w.Body.String(), "sessionVersion") {
		t.Fatal("重设密码应成功且不回传任何凭据", w.Code)
	}
	if w := request("GET", "/api/account", player, nil); w.Code != 401 {
		t.Fatal("密码重设后旧会话须失效")
	}
	player = s.sign(f.trader.ID, "player", time.Hour)
	if w := request("GET", "/api/account", player, nil); w.Code != 200 {
		t.Fatal("新会话应能读取本人资料")
	}
	if w := request("PUT", path, admin, map[string]any{"disabled": true}); w.Code != 200 {
		t.Fatal("应可停用账户")
	}
	if w := request("PUT", path, admin, map[string]any{"disabled": false}); w.Code != 200 {
		t.Fatal("应可恢复账户")
	}
	if w := request("GET", "/api/account", player, nil); w.Code != 401 {
		t.Fatal("恢复账户不得复活停用前的会话")
	}
}
