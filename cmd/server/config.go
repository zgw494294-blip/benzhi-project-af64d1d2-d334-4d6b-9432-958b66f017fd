package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type config struct {
	Addr, DataDir string
	Selfcheck     bool
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("tapemaster-gate", flag.ContinueOnError)
	var cfg config
	fs.StringVar(&cfg.Addr, "addr", "", "回环监听地址")
	fs.StringVar(&cfg.DataDir, "data", ".tapemaster-data", "事件日志目录")
	fs.BoolVar(&cfg.Selfcheck, "selfcheck", false, "运行有界端到端自检后退出")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, errors.New("存在未识别的位置参数")
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:19081"
		if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
			port, err := strconv.Atoi(raw)
			if err != nil || port < 1024 || port > 65535 {
				return cfg, fmt.Errorf("PORT 必须是 1024 到 65535 的端口号")
			}
			cfg.Addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}
	host, portText, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return cfg, fmt.Errorf("addr 必须采用 host:port 格式: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return cfg, errors.New("addr 必须使用明确的回环 IP 地址")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		return cfg, errors.New("addr 端口必须在 1024 到 65535 之间")
	}
	return cfg, nil
}
