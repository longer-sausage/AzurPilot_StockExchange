package exchange

import (
	"fmt"
	"testing"
	"time"
)

// 空闲账户数量不应拖慢定时任务和无变更事务。
func benchmarkIdleEngine(b *testing.B, count int) *Engine {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	e := &Engine{clock: func() time.Time { return now }, players: map[int64]*Player{}}
	e.state.Settings = DefaultSettings()
	e.state.Season = seasonFor(now, e.state.Settings)
	for id := int64(1); id <= int64(count); id++ {
		e.players[id] = &Player{ID: id, Username: fmt.Sprint(id), Cash: InitialCash, Quote: Quote{Price: 100000}, Positions: map[int64]*Position{}}
	}
	e.reindex()
	return e
}

func BenchmarkIdleTick10000Players(b *testing.B) {
	e := benchmarkIdleEngine(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := e.Tick(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIdleTransaction10000Players(b *testing.B) {
	e := benchmarkIdleEngine(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := e.transaction(func() error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
