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
	Short: "初始化数据目录与数据库，并检查 bl/ffmpeg 环境",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("数据目录: %s\n", application.Cfg.DataDir)
		fmt.Printf("数据库:   %s\n", application.Cfg.DBPath())
		if err := checkBin("bl", application.Cfg.BLBin, "--version"); err != nil {
			fmt.Println("bl 检查:   异常 —", err)
		} else {
			fmt.Println("bl 检查:   正常")
		}
		if err := checkBin("ffmpeg", application.Cfg.FFMPEGBin, "-version"); err != nil {
			fmt.Println("ffmpeg 检查: 异常 —", err)
		} else {
			fmt.Println("ffmpeg 检查: 正常")
		}
		if err := checkBin("ffprobe", application.Cfg.FFProbeBin(), "-version"); err != nil {
			fmt.Println("ffprobe 检查: 异常 —", err)
		} else {
			fmt.Println("ffprobe 检查: 正常")
		}
		fmt.Println("\n初始化完成。使用 `story series create --help` 开始创建系列。")
		return nil
	},
}

func checkBin(name, bin string, arg ...string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("未找到 %s（%s）", name, bin)
	}
	c := exec.Command(bin, arg...)
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s 无法运行: %w", name, err)
	}
	return nil
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
