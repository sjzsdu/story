package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/doctor"
)

var (
	configPath  string
	rootCfg     config.Config
	application *app.App
	rootCtx     context.Context
	// version 由构建时通过 -ldflags "-X main.version=..." 注入。
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "story",
	Version: version,
	Short:   "中国历史故事 AI 视频流水线",
	Long: "story —— 从中国历史典籍取材，经 AI 生成故事、分镜、视频与旁白，" +
		"最终由 ffmpeg 合成带字幕的多平台 MP4 短视频。",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 信号可取消的全局 context。
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		rootCtx = ctx
		go func() {
			<-ctx.Done()
			stop()
		}()

		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		rootCfg = cfg
		// serve 的控制子命令（start/stop/status/restart）与 --daemon 父进程
		// 不需要 app 容器：它们只操作 PID 文件或 fork 子进程。
		// --daemonized 子进程需要 app（实际跑 HTTP 服务）。
		if needsBootstrap(cmd) {
			application, err = app.Bootstrap(rootCtx, cfg)
			if err != nil {
				return err
			}
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if application != nil {
			_ = application.Close()
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "story.yaml", "配置文件路径（不存在则使用默认配置）")
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化数据目录、检查并安装所有依赖",
	Long: `初始化 story 运行环境：
  1. 创建数据目录与数据库
  2. 检查所有 CLI 依赖（bl, ffmpeg, python3, sau, Chrome）
  3. 尝试自动安装缺失的依赖（pip install sau 等）
  4. 输出完整的健康状态报告`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		cfg := application.Cfg

		fmt.Println("story 初始化")
		fmt.Println("═══════════════════════════════════════")

		// 1. 数据目录
		fmt.Printf("数据目录: %s\n", cfg.DataDir)
		fmt.Printf("数据库:   %s\n", cfg.DBPath())
		fmt.Println()

		// 2. 运行健康检查
		d := doctor.New(cfg.BLBin, cfg.FFMPEGBin, cfg.PythonBin, cfg.SAUBin)
		results := d.RunAll(cmd.Context())

		fmt.Println("依赖检查:")
		for _, r := range results {
			status := r.Status.String()
			version := r.Version
			if version == "" {
				version = "-"
			}
			fmt.Printf("  %s %-26s  %s\n", status, r.Name, version)
			if r.Message != "" {
				fmt.Printf("     %s\n", r.Message)
			}
		}

		ok, warn, fail := doctor.Summary(results)
		fmt.Println()

		// 3. 尝试自动安装缺失项
		if fail > 0 {
			fmt.Println("尝试自动安装缺失依赖...")
			autoInstalled := 0
			for _, r := range results {
				if r.Status == doctor.StatusMissing {
					if installed := attemptInstall(cmd, r.Name); installed {
						autoInstalled++
						fmt.Printf("  ✅ 已安装 %s\n", r.Name)
					}
				}
			}
			if autoInstalled > 0 {
				fmt.Println()
				// 重新检查
				results = d.RunAll(cmd.Context())
				ok, warn, fail = doctor.Summary(results)
			}
		}

		// 4. 输出最终状态
		fmt.Println("═══════════════════════════════════════")
		fmt.Printf("共 %d 项: ✅ %d 正常  ⚠️ %d 警告  ❌ %d 缺失\n",
			len(results), ok, warn, fail)

		if fail > 0 {
			fmt.Println("\n💡 以下依赖需要手动安装:")
			for _, r := range results {
				if r.Status == doctor.StatusMissing && r.Fix != "" {
					fmt.Printf("  • %s: %s\n", r.Name, r.Fix)
				}
			}
			fmt.Println("\n安装后执行 story doctor 验证")
		} else if warn > 0 {
			fmt.Println("\n⚠️ 部分依赖版本偏低，功能可用但可能不稳定")
		} else {
			fmt.Println("\n🎉 所有依赖就绪！使用 `story series create --help` 开始创建系列")
		}
		return nil
	},
}

// attemptInstall 尝试自动安装缺失的依赖。
func attemptInstall(cmd *cobra.Command, name string) bool {
	switch name {
	case "sau (social-auto-upload)":
		return installSAU()
	}
	return false
}

// installSAU 安装 social-auto-upload。
// 上游项目需要 clone + conf.py 才能正常工作，不能简单 pip install。
func installSAU() bool {
	home, _ := os.UserHomeDir()
	sauDir := home + "/.story/sau"

	// 1. clone（已存在则 pull）
	if _, err := os.Stat(sauDir + "/.git"); err == nil {
		fmt.Println("     sau 仓库已存在，执行 git pull...")
		c := exec.Command("git", "-C", sauDir, "pull", "--ff-only")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		_ = c.Run() // pull 失败不阻断
	} else {
		fmt.Printf("     克隆 social-auto-upload 到 %s...\n", sauDir)
		os.MkdirAll(home+"/.story", 0755)
		c := exec.Command("git", "clone", "--depth", "1",
			"https://github.com/dreammis/social-auto-upload.git", sauDir)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Println("     git clone 失败:", err)
			return false
		}
	}

	// 2. 生成 conf.py（从 conf.example.py）
	confExample := sauDir + "/conf.example.py"
	confFile := sauDir + "/conf.py"
	if _, err := os.Stat(confFile); err != nil {
		if data, err := os.ReadFile(confExample); err == nil {
			os.WriteFile(confFile, data, 0644)
			fmt.Println("     已生成 conf.py")
		}
	}

	// 3. uv sync 安装依赖 + playwright（上游漏声明）
	fmt.Println("     安装 sau 依赖...")
	c := exec.Command("uv", "sync")
	c.Dir = sauDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Println("     uv sync 失败:", err)
		return false
	}
	c = exec.Command("uv", "pip", "install", "--python", sauDir+"/.venv/bin/python", "playwright")
	c.Dir = sauDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run() // playwright 可能已安装，忽略错误

	// 4. 创建 wrapper 脚本到 ~/.local/bin/sau
	wrapperDir := home + "/.local/bin"
	os.MkdirAll(wrapperDir, 0755)
	wrapper := wrapperDir + "/sau"
	script := fmt.Sprintf(`#!/bin/sh
cd %s && exec uv run sau "$@"`, sauDir)
	if err := os.WriteFile(wrapper, []byte(script), 0755); err != nil {
		fmt.Println("     创建 wrapper 失败:", err)
		return false
	}
	fmt.Printf("     已创建 %s\n", wrapper)
	return true
}

func init() {
	rootCmd.AddCommand(initCmd)
}

// needsBootstrap 判断当前命令是否需要 app 容器。
// serve 的控制子命令（start/stop/status/restart）只操作 PID 文件，
// --daemon 父进程只 fork 子进程，均不需要 Bootstrap；--daemonized 子进程
// 实际跑 HTTP 服务需要 Bootstrap。
func needsBootstrap(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "start", "stop", "status", "restart":
		return false
	}
	if cmd.Name() == "serve" {
		if v, _ := cmd.Flags().GetBool("daemonized"); v {
			return true
		}
		if v, _ := cmd.Flags().GetBool("daemon"); v {
			return false
		}
	}
	return true
}
