package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/server"
	webui "github.com/sjzsdu/story/web"
)

var (
	serveAddr       string
	serveDaemon     bool
	serveDaemonized bool // 隐藏 flag，父进程 fork 子进程时附加；子进程据此进入 daemon 主循环。
)

// pidInfo PID 文件内容；JSON 落盘以便 stop/status 反查 addr 与启动时间。
// 跨平台类型，daemon 实现按 build tag 分文件。
type pidInfo struct {
	PID       int       `json:"pid"`
	Addr      string    `json:"addr"`
	StartedAt time.Time `json:"started_at"`
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 Web UI（预览各阶段产物并在线操作流水线）",
	Long: "启动 Web UI 服务，默认前台运行；带 --daemon 或子命令 start 后台守护进程运行。\n\n" +
		"子命令：\n" +
		"  start    后台启动（等价于 --daemon）\n" +
		"  stop     停止后台守护进程\n" +
		"  status   查询后台守护进程状态\n" +
		"  restart  重启后台守护进程",
	RunE: func(cmd *cobra.Command, args []string) error {
		if serveDaemonized {
			return runDaemonized(serveAddr)
		}
		if serveDaemon {
			return daemonize(serveAddr)
		}
		assets, built := webui.DistFS()
		if !built {
			fmt.Println("提示: 前端尚未构建，仅提供 API；请在 web/ 执行 npm run build（开发可用 npm run dev）")
		}
		srv := server.New(rootCtx, application, assets)
		return srv.ListenAndServe(serveAddr)
	},
}

var serveStartCmd = &cobra.Command{
	Use:   "start",
	Short: "后台启动 serve 守护进程",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonize(serveAddr)
	},
}

var serveStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止后台 serve 守护进程",
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopDaemon()
	},
}

var serveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查询 serve 守护进程状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		return statusDaemon()
	},
}

var serveRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "重启 serve 守护进程",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 用户未显式传 --addr 时，沿用旧 PID 文件中的 addr，避免 restart 回退到默认端口。
		if !cmd.Flags().Changed("addr") {
			if p, err := readPIDInfo(); err == nil && p.Addr != "" {
				serveAddr = p.Addr
			}
		}
		if err := stopDaemon(); err != nil {
			return err
		}
		return daemonize(serveAddr)
	},
}

func init() {
	// --addr 用 PersistentFlags，子命令 start/restart 也继承。
	serveCmd.PersistentFlags().StringVar(&serveAddr, "addr", "127.0.0.1:7878", "监听地址")
	serveCmd.Flags().BoolVar(&serveDaemon, "daemon", false, "后台守护进程方式启动")
	// start 本身即后台启动，此处再注册一次，兼容 `story serve start --daemon` 的显式写法。
	serveStartCmd.Flags().BoolVar(&serveDaemon, "daemon", false, "后台守护进程方式启动（start 已隐含此行为）")
	serveCmd.Flags().BoolVar(&serveDaemonized, "daemonized", false, "内部用：子进程模式")
	_ = serveCmd.Flags().MarkHidden("daemonized")
	serveCmd.AddCommand(serveStartCmd)
	serveCmd.AddCommand(serveStopCmd)
	serveCmd.AddCommand(serveStatusCmd)
	serveCmd.AddCommand(serveRestartCmd)
	rootCmd.AddCommand(serveCmd)
}
