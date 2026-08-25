package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"tapemastergate/internal/application"
	"tapemastergate/internal/httpapi"
	"tapemastergate/internal/journal"
	"tapemastergate/internal/webui"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("TapeMaster Gate 退出: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	dataDir := cfg.DataDir
	if cfg.Selfcheck {
		dataDir, err = os.MkdirTemp("", "tapemaster-selfcheck-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dataDir)
	} else {
		abs, absErr := filepath.Abs(dataDir)
		if absErr == nil {
			dataDir = abs
		}
	}
	store, err := journal.Open(dataDir)
	if err != nil {
		return fmt.Errorf("打开事件日志: %w", err)
	}
	defer store.Close()
	app := application.NewService(store)
	api := httpapi.New(app)
	mux := http.NewServeMux()
	mux.Handle("/api/", api.Handler())
	mux.Handle("/", webui.Handler())
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.Addr, err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	log.Printf("TapeMaster Gate 已监听 http://%s", cfg.Addr)
	if cfg.Selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		checkErr := runSelfcheck(ctx, cfg.Addr)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		serveErr := <-errCh
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		if checkErr != nil {
			return fmt.Errorf("selfcheck 失败: %w", checkErr)
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		log.Printf("selfcheck 通过：建档、采集、冻结、发证、验证及审计链路完整")
		return nil
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err = <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-sigCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
