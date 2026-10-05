package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"stock.nanoda.work/exchange/internal/exchange"
	"syscall"
	"time"
	_ "time/tzdata"
)

func main() {
	c, err := exchange.LoadConfig()
	if err != nil {
		slog.Error("配置错误", "error", err)
		os.Exit(1)
	}
	e, err := exchange.Open(c.Database, nil)
	if err != nil {
		slog.Error("数据库启动失败", "error", err)
		os.Exit(1)
	}
	defer e.Close()
	if c.Mock {
		if err = exchange.SeedMock(e); err != nil {
			slog.Error("模拟数据初始化失败", "error", err)
			os.Exit(1)
		}
	}
	handler := exchange.NewServer(e, c)
	server := &http.Server{Addr: c.Listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 12 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := e.Tick(); err != nil {
					slog.Error("结算失败", "error", err)
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("茗喵证券交易所已启动", "listen", c.Listen, "mock", c.Mock)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP 服务错误", "error", err)
		os.Exit(1)
	}
}
