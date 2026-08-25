package main

import "testing"

func TestConfigDefaultAndLoopbackValidation(t *testing.T) {
	t.Setenv("PORT", "")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19081" {
		t.Fatalf("默认地址错误: %s", cfg.Addr)
	}
	if _, err = parseConfig([]string{"-addr=0.0.0.0:19081"}); err == nil {
		t.Fatal("不应允许非回环监听")
	}
	if _, err = parseConfig([]string{"-addr=127.0.0.1:8080"}); err != nil {
		t.Fatalf("显式高于 1024 的端口应允许: %v", err)
	}
}
func TestConfigPortFallback(t *testing.T) {
	t.Setenv("PORT", "19123")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19123" {
		t.Fatalf("PORT 回退错误: %s", cfg.Addr)
	}
}
