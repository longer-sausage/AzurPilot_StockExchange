package exchange

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestAuthenticationUsesAvailableCPUWithoutQueueing(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	f := setup(t)
	s := NewServer(f.e, Config{Mock: true})
	stubTestCaptcha(t, s)
	identity := newTestIdentity(t)
	body, _ := json.Marshal(map[string]any{"username": "并发准入猫", "password": "test-password-123", "acceptedNotice": "2026-10-03", "recaptchaToken": recaptchaTestToken, "report": identity.report(f.now, 1000, f.now.Unix())})
	if cap(s.Slots) != 4 || cap(s.passwordSlots) != 1 {
		t.Fatal("网络等待与密码计算应按 CPU 分别准入")
	}
	s.passwordSlots <- struct{}{}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/register", bytes.NewReader(body)))
	if w.Code != 503 || len(f.e.players) != 2 {
		t.Fatal("密码计算繁忙时应立即拒绝，不创建账户", w.Code)
	}
	<-s.passwordSlots
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/register", bytes.NewReader(body)))
	if w.Code != 201 || len(f.e.players) != 3 {
		t.Fatal("资源释放后应能正常开户", w.Code, w.Body.String())
	}
	runtime.GOMAXPROCS(3)
	scaled := NewServer(f.e, Config{})
	if cap(scaled.Slots) != 6 || cap(scaled.passwordSlots) != 3 {
		t.Fatal("多核机器不应被固定为两个认证槽")
	}
}

func TestFailedNewRegistrationRollsBackPlayerAndIndexes(t *testing.T) {
	f := setup(t)
	next, count, revision := f.e.state.NextPlayer, len(f.e.players), f.e.state.Revision
	f.e.db.Close()
	if _, _, err := f.e.Register("磁盘失败猫", "test-password-123", 1000, f.now.Unix()); err == nil {
		t.Fatal("落盘失败不能开户")
	}
	if len(f.e.players) != count || f.e.state.NextPlayer != next || f.e.state.Revision != revision || f.e.names["磁盘失败猫"] != 0 {
		t.Fatal("落盘失败遗留玩家、索引或状态")
	}
	if _, err := f.e.Login(f.trader.Username, "test-password-123"); err != nil {
		t.Fatal("回滚破坏了已有账户", err)
	}
}
