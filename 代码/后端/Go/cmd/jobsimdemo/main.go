// main.go
// JobsIM Native Demo Host
// Created by Jobs on 2026年11月18日，星期三.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "JobsIM demo:", err)
		os.Exit(1)
	}
}

func run() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("此原生演示宿主仅支持MacOS")
	}
	if len(os.Args) != 2 {
		return fmt.Errorf("用法：JobsIMDemoHost prepare|serve|check；没有远程停止/删库入口")
	}
	host, err := resolveHost()
	if err != nil {
		return err
	}
	switch os.Args[1] {
	case "prepare":
		return host.prepare()
	case "check":
		return host.check()
	case "serve":
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return host.serve(ctx)
	default:
		return fmt.Errorf("未知动作；仅prepare/serve/check")
	}
}
