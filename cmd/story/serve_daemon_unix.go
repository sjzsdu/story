//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sjzsdu/story/internal/server"
	webui "github.com/sjzsdu/story/web"
)

// pidInfo PID 文件内容；JSON 落盘以便 stop/status 反查 addr 与启动时间。
// 类型定义在跨平台文件 serve.go 中，便于 windows stub 复用。

func pidFilePath() string { return filepath.Join(rootCfg.DataDir, "serve.pid") }
func logFilePath() string { return filepath.Join(rootCfg.DataDir, "serve.log") }

func readPIDInfo() (*pidInfo, error) {
	b, err := os.ReadFile(pidFilePath())
	if err != nil {
		return nil, err
	}
	var p pidInfo
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func writePIDInfo(p pidInfo) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(pidFilePath(), b, 0o644)
}

func removePIDFile() { _ = os.Remove(pidFilePath()) }

// processAlive 用 signal 0 探活；进程不存在返回 ESRCH。
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// daemonize 是父进程逻辑：fork 一个脱离会话的子进程并等待其写 PID 文件后退出。
// 子进程接收 --daemonized 隐藏 flag，进入 runDaemonized 主循环。
func daemonize(addr string) error {
	// 冲突检测：已有运行中的守护进程则拒绝。
	if p, err := readPIDInfo(); err == nil && processAlive(p.PID) {
		return fmt.Errorf("serve 已在运行（pid=%d, addr=%s）；如需重启请用 `story serve restart`", p.PID, p.Addr)
	}
	removePIDFile() // 清理 stale PID 文件

	// 端口预检：前台 `story serve`（或其它进程）占着端口时不写 PID 文件，
	// 只查 PID 文件发现不了它；不预检则子进程 bind 失败秒退，父进程只能报出
	// 「2s 内未完成启动」这种指不到病根的错。
	if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		_ = conn.Close()
		return fmt.Errorf("端口 %s 已被占用（可能有一个非守护进程的 serve 在运行，如前台 `story serve`）；请先停止它再启动守护进程", addr)
	}

	if err := os.MkdirAll(rootCfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	logFile, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("打开日志文件: %w", err)
	}
	defer logFile.Close()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("解析可执行文件: %w", err)
	}
	cmd := exec.Command(exe, "serve", "--addr", addr, "--daemonized")
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Setsid 让子进程脱离终端会话，父进程退出不会拖死子进程。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动守护进程: %w", err)
	}
	// 父进程不 Wait，立即释放句柄；子进程被 init 收养，不会成为僵尸。
	_ = cmd.Process.Release()

	// 轮询 PID 文件就绪（最多 2s）：子进程写好 PID 文件代表它已进入主循环。
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if p, err := readPIDInfo(); err == nil && p.PID != 0 && processAlive(p.PID) {
			fmt.Printf("已启动守护进程：pid=%d, addr=%s\n", p.PID, p.Addr)
			fmt.Printf("日志: %s\n", logFilePath())
			fmt.Println("查询状态: story serve status")
			fmt.Println("停止:     story serve stop")
			return nil
		}
	}
	return fmt.Errorf("守护进程在 2s 内未完成启动；请查看日志 %s", logFilePath())
}

// runDaemonized 是子进程主循环：写 PID 文件、启动 HTTP 服务、监听 SIGTERM 优雅关闭。
func runDaemonized(addr string) error {
	// 忽略 SIGHUP：守护进程虽已 Setsid 脱离终端，但父进程所在会话结束时仍可能被
	// 连带 HUP，默认动作是终止进程——表现为「启动成功、几秒后无声消失」，
	// 日志里连一行错误都不会留。
	signal.Ignore(syscall.SIGHUP)

	if err := os.MkdirAll(rootCfg.DataDir, 0o755); err != nil {
		return err
	}
	pid := os.Getpid()
	info := pidInfo{PID: pid, Addr: addr, StartedAt: time.Now()}
	if err := writePIDInfo(info); err != nil {
		return fmt.Errorf("写 PID 文件: %w", err)
	}
	defer removePIDFile()

	assets, built := webui.DistFS()
	if !built {
		fmt.Fprintln(os.Stderr, "提示: 前端尚未构建，仅提供 API")
	}
	srv := server.New(rootCtx, application, assets)
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	fmt.Fprintf(os.Stderr, "[daemon] Web UI: http://%s (pid=%d)\n", addr, pid)

	// 监听 SIGTERM/SIGINT 做 graceful shutdown。rootCtx 的 signal.NotifyContext 也会触发，
	// 但 http.Server 需显式 Shutdown 才能等当前请求退出，此处独立监听。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "[daemon] 收到信号 %v，开始优雅关闭...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()

	err := httpSrv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	fmt.Fprintln(os.Stderr, "[daemon] 已退出")
	return nil
}

func stopDaemon() error {
	p, err := readPIDInfo()
	if err != nil {
		fmt.Println("未运行（无 PID 文件）")
		return nil
	}
	if !processAlive(p.PID) {
		removePIDFile()
		fmt.Printf("未运行（清理了过期 PID 文件，原 pid=%d）\n", p.PID)
		return nil
	}
	proc, _ := os.FindProcess(p.PID)
	fmt.Printf("发送 SIGTERM → pid=%d\n", p.PID)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("发送 SIGTERM: %w", err)
	}
	// 轮询退出：8s 超时则 SIGKILL。
	for i := 0; i < 80; i++ {
		time.Sleep(100 * time.Millisecond)
		if !processAlive(p.PID) {
			fmt.Printf("已停止（pid=%d）\n", p.PID)
			return nil
		}
	}
	fmt.Fprintln(os.Stderr, "8s 未退出，强制 SIGKILL")
	_ = proc.Signal(syscall.SIGKILL)
	removePIDFile()
	fmt.Printf("已强制终止（pid=%d）\n", p.PID)
	return nil
}

func statusDaemon() error {
	p, err := readPIDInfo()
	if err != nil {
		fmt.Println("状态: 未运行（无 PID 文件）")
		return nil
	}
	if !processAlive(p.PID) {
		fmt.Printf("状态: 未运行（PID 文件过期，原 pid=%d）\n", p.PID)
		return nil
	}
	uptime := time.Since(p.StartedAt).Round(time.Second)
	fmt.Printf("状态: 运行中\n")
	fmt.Printf("  PID:       %d\n", p.PID)
	fmt.Printf("  Addr:      %s\n", p.Addr)
	fmt.Printf("  启动时间:  %s\n", p.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  已运行:    %s\n", uptime)
	fmt.Printf("  日志:      %s\n", logFilePath())
	// TCP 端口探活：进程存活但 server 没起来也能识别。
	if conn, err := net.DialTimeout("tcp", p.Addr, time.Second); err == nil {
		_ = conn.Close()
		fmt.Printf("  端口探活:  正常（%s 可连接）\n", p.Addr)
	} else {
		fmt.Printf("  端口探活:  异常（%s 不可连接: %v）\n", p.Addr, err)
	}
	return nil
}
