//go:build windows

// windows stub：daemon 模式依赖 POSIX（Setsid/SIGTERM/SIGKILL），Windows 暂不支持。
// 命令仍可注册并给出明确错误，避免跨平台编译缺失符号。

package main

import "fmt"

func readPIDInfo() (*pidInfo, error) { return nil, fmt.Errorf("daemon 模式暂不支持 Windows") }

func daemonize(addr string) error {
	return fmt.Errorf("daemon 模式暂不支持 Windows（缺少 POSIX setsid/信号语义）")
}

func runDaemonized(addr string) error {
	return fmt.Errorf("daemon 模式暂不支持 Windows")
}

func stopDaemon() error {
	return fmt.Errorf("daemon 模式暂不支持 Windows")
}

func statusDaemon() error {
	return fmt.Errorf("daemon 模式暂不支持 Windows")
}
