package exchange

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWatchlistIsPrivateIdempotentAndPersistent(t *testing.T) {
	f := setup(t)
	for range 2 {
		list, err := f.e.SetWatchlist(f.trader.ID, f.owner.ID, true)
		if err != nil || len(list) != 1 || list[0] != f.owner.ID {
			t.Fatal("重复添加应保持一条自选", list, err)
		}
	}
	if _, err := f.e.SetWatchlist(f.trader.ID, 999999, true); err == nil {
		t.Fatal("不能添加不存在的证券")
	}
	market, _, _ := f.e.MarketJSON()
	if strings.Contains(string(market), "watchlist") {
		t.Fatal("公开行情不能泄漏账户自选")
	}
	f.e.Close()
	var err error
	f.e, err = Open(f.path, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	account, _ := f.e.Account(f.trader.ID)
	if len(account.Player.Watchlist) != 1 {
		t.Fatal("重启丢失自选")
	}
	f.now = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err := f.e.Tick(); err != nil {
		t.Fatal(err)
	}
	account, _ = f.e.Account(f.trader.ID)
	if len(account.Player.Watchlist) != 1 {
		t.Fatal("赛季重置丢失自选")
	}
	list, err := f.e.SetWatchlist(f.trader.ID, f.owner.ID, false)
	if err != nil || len(list) != 0 {
		t.Fatal("移除自选失败", err)
	}
}

func TestUpdatesOnlyPublishCommittedChangesAndQuotesHaveNoDelay(t *testing.T) {
	f := setup(t)
	_, changed := f.e.updates()
	f.now = f.now.Add(time.Second)
	if err := f.e.Upload(f.upload, 1001, f.now.Unix()); err != nil {
		t.Fatal("新报价应立即受理，无 15 秒间隔", err)
	}
	select {
	case <-changed:
	default:
		t.Fatal("已提交报价没有唤醒订阅者")
	}
	revision, changed := f.e.updates()
	if err := f.e.Upload(f.upload, 1002, f.now.Unix()); err == nil {
		t.Fatal("同时间冲突报价应继续拒绝")
	}
	select {
	case <-changed:
		t.Fatal("失败事务发布了变更")
	default:
	}
	current, _ := f.e.updates()
	if current != revision {
		t.Fatal("失败事务改变了版本")
	}
	f.e.db.Close()
	if _, err := f.e.SetWatchlist(f.trader.ID, f.owner.ID, true); err == nil {
		t.Fatal("磁盘失败不能更新自选")
	}
	select {
	case <-changed:
		t.Fatal("磁盘失败发布了变更")
	default:
	}
	account, _ := f.e.Account(f.trader.ID)
	if len(account.Player.Watchlist) != 0 {
		t.Fatal("自选修改失败没有回滚")
	}
}

func TestEventStreamDeliversChangesAndDisconnects(t *testing.T) {
	f := setup(t)
	server := httptest.NewUnstartedServer(NewServer(f.e, Config{}))
	server.Config.WriteTimeout = 50 * time.Millisecond
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/events", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Accel-Buffering") != "no" || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("事件流没有关闭缓冲")
	}
	scanner := bufio.NewScanner(response.Body)
	readRevision := func() uint64 {
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
				var update struct{ Revision uint64 }
				if err := json.Unmarshal([]byte(line[6:]), &update); err != nil {
					t.Fatal(err)
				}
				return update.Revision
			}
		}
		t.Fatal("没有收到实时事件", scanner.Err())
		return 0
	}
	before := readRevision()
	if _, err := f.e.SetWatchlist(f.trader.ID, f.owner.ID, true); err != nil {
		t.Fatal(err)
	}
	for readRevision() == before {
	}
	if readRevision() <= before {
		t.Fatal("事件流没有越过普通请求写入期限")
	}
}

func TestCompleteOrderHistoryAndUnlimitedPendingOrders(t *testing.T) {
	f := setup(t)
	for i := range 130 {
		if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "market", 1, 0, fmt.Sprintf("history_%04d", i))); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 25 {
		if _, err := f.e.Submit(f.trader.ID, order(f, "buy", "limit", 1, 100, fmt.Sprintf("pending_%04d", i))); err != nil {
			t.Fatal("挂单应超过旧 20 笔上限", err)
		}
	}
	// 模拟老版本曾从账户快照裁掉记录，完整回执仍可恢复。
	if err := f.e.transaction(func() error { f.e.touch(f.trader.ID).Orders = f.e.players[f.trader.ID].Orders[100:]; return nil }); err != nil {
		t.Fatal(err)
	}
	orders, err := f.e.orderHistory(f.trader.ID)
	if err != nil || len(orders) != 155 {
		t.Fatal("完整订单回执没有恢复", err)
	}
	server := NewServer(f.e, Config{})
	for range 260 {
		reply := httptest.NewRecorder()
		server.ServeHTTP(reply, httptest.NewRequest("GET", "/api/market", nil))
		if reply.Code != 200 {
			t.Fatal("普通请求不应受到旧限流", reply.Code)
		}
	}
}

func TestLargeOrdersUseFundingAndLedgerSafetyInsteadOfFixedQuotas(t *testing.T) {
	f := setup(t)
	f.now = f.now.Add(time.Second)
	if err := f.e.Upload(f.upload, 1_000_000, f.now.Unix()); err != nil {
		t.Fatal(err)
	}
	if err := f.e.transaction(func() error { f.e.touch(f.trader.ID).Cash = MaxNotional; return nil }); err != nil {
		t.Fatal(err)
	}
	in := order(f, "buy", "market", 1_000_001, 0, "large_order_001")
	in.Leverage = 2
	if _, err := f.e.Submit(f.trader.ID, in); err != nil {
		t.Fatal("资金足够时应允许超过旧数量和名义金额配额", err)
	}
	in.ClientID, in.Quantity = "overflow_order_001", 1<<62
	if _, err := f.e.Submit(f.trader.ID, in); err == nil || publicError(err).Code != "NUMERICAL_LIMIT" {
		t.Fatal("必须在金额乘法前拒绝整数溢出", err)
	}
}
