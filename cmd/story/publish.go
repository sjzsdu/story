package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/provider/publish/sau"
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "发布成片到各平台",
	Long: `将指定集的成片发布到配置的目标平台。
支持多平台同时发布、多账号、定时发布。

前置条件：
  1. pip install social-auto-upload（或 uv add social-auto-upload）
  2. story platform login <platform> --account <name>  # 扫码登录一次
`,
}

var (
	publishPlatforms []string
	publishAccount   string
	publishTitle     string
	publishDesc      string
	publishCover     string
	publishCategory  string
	publishSchedule  string
)

var publishRunCmd = &cobra.Command{
	Use:   "run <episode-id>",
	Short: "发布到指定平台",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化，请先执行 story init")
		}

		epID := args[0]
		platforms := publishPlatforms
		if len(platforms) == 0 {
			return fmt.Errorf("请用 --platform 指定发布平台")
		}

		// 确定账号
		accountName := publishAccount
		if accountName == "" {
			accountName = application.Cfg.DefaultPublishAccount
		}

		// 解析定时发布时间
		var scheduledAt *time.Time
		if publishSchedule != "" {
			t, err := time.Parse(time.RFC3339, publishSchedule)
			if err != nil {
				t, err = time.ParseInLocation("2006-01-02T15:04:05", publishSchedule, time.Local)
				if err != nil {
					return fmt.Errorf("无效的时间格式 %q: %w", publishSchedule, err)
				}
			}
			scheduledAt = &t
		}

		jobs, err := application.Publish(cmd.Context(), epID, app.PublishInput{
			Platforms:   platforms,
			Title:       publishTitle,
			Description: publishDesc,
			CoverPath:   publishCover,
			Category:    publishCategory,
			ScheduledAt: scheduledAt,
		})
		if err != nil {
			return err
		}

		for _, job := range jobs {
			statusLabel := statusEmoji(job.Status)
			fmt.Printf("%s %-8s  %s\n", statusLabel, job.Platform, job.Title)
			if job.Error != "" {
				fmt.Printf("   状态: %s\n", job.Error)
			}
			if job.PlatformURL != "" {
				fmt.Printf("   链接: %s\n", job.PlatformURL)
			}
		}
		return nil
	},
}

var publishStatusCmd = &cobra.Command{
	Use:   "status [episode-id]",
	Short: "查看发布状态",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		if len(args) == 0 {
			fmt.Println("用法: story publish status <episode-id>")
			return nil
		}
		jobs, err := application.ListPublishJobs(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			fmt.Println("该集暂无发布任务")
			return nil
		}
		for _, job := range jobs {
			fmt.Printf("%s %-8s  %s  (尝试 %d/%d)\n",
				statusEmoji(job.Status), job.Platform, job.Title, job.Attempts, job.MaxRetries)
			if job.Error != "" {
				fmt.Printf("   错误: %s\n", job.Error)
			}
			if job.PlatformURL != "" {
				fmt.Printf("   链接: %s\n", job.PlatformURL)
			}
		}
		return nil
	},
}

var publishCancelCmd = &cobra.Command{
	Use:   "cancel <job-id>",
	Short: "取消发布任务",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		if err := application.CancelPublish(cmd.Context(), args[0]); err != nil {
			return err
		}
		fmt.Printf("发布任务 %s 已取消\n", args[0])
		return nil
	},
}

var publishDeleteCmd = &cobra.Command{
	Use:   "delete <job-id>",
	Short: "删除已发布的视频",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		if err := application.DeletePublished(cmd.Context(), args[0]); err != nil {
			return err
		}
		fmt.Printf("已删除发布任务 %s 对应的平台视频\n", args[0])
		return nil
	},
}

// ---- platform 子命令（登录/检查） ----

var platformCmd = &cobra.Command{
	Use:   "platform",
	Short: "平台账号管理（登录/检查状态）",
	Long: `管理各平台的登录状态。
首次使用需执行 story platform login <platform> --account <name> 扫码登录，
登录后 Cookie 自动持久化，后续发布无需重复登录。

支持的平台: ` + strings.Join(sau.SupportedPlatforms, ", "),
}

var platformLoginCmd = &cobra.Command{
	Use:   "login <platform>",
	Short: "登录平台（扫码/Cookie 持久化）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		platform := args[0]
		account := publishAccount
		if account == "" {
			return fmt.Errorf("请用 --account 指定账号名，如 --account my_douyin")
		}

		fmt.Printf("正在打开 %s 登录页面，请在浏览器中扫码登录...\n", sau.PlatformLabel(platform))
		if err := application.SauLogin(cmd.Context(), platform, account); err != nil {
			return fmt.Errorf("登录失败: %w", err)
		}
		fmt.Printf("✅ %s 账号 %s 登录成功，Cookie 已保存\n", sau.PlatformLabel(platform), account)
		return nil
	},
}

var platformCheckCmd = &cobra.Command{
	Use:   "check <platform>",
	Short: "检查平台登录状态",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		platform := args[0]
		account := publishAccount
		if account == "" {
			account = application.Cfg.DefaultPublishAccount
		}
		if account == "" {
			return fmt.Errorf("请用 --account 指定账号名")
		}

		valid, err := application.SauCheck(cmd.Context(), platform, account)
		if err != nil {
			return fmt.Errorf("检查失败: %w", err)
		}
		if valid {
			fmt.Printf("✅ %s 账号 %s 登录有效\n", sau.PlatformLabel(platform), account)
		} else {
			fmt.Printf("❌ %s 账号 %s 未登录或已过期，请重新登录\n", sau.PlatformLabel(platform), account)
		}
		return nil
	},
}

var platformListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出支持的平台",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("支持的平台:")
		for _, p := range sau.SupportedPlatforms {
			fmt.Printf("  %-12s %s\n", p, sau.PlatformLabel(p))
		}
		return nil
	},
}

func init() {
	// publish run flags
	publishRunCmd.Flags().StringSliceVar(&publishPlatforms, "platform", nil, "目标平台（逗号分隔），如 douyin,kuaishou,bilibili")
	publishRunCmd.Flags().StringVar(&publishAccount, "account", "", "账号名（对应 sau --account）")
	publishRunCmd.Flags().StringVar(&publishTitle, "title", "", "自定义标题（留空使用故事标题）")
	publishRunCmd.Flags().StringVar(&publishDesc, "desc", "", "自定义描述（留空使用故事摘要）")
	publishRunCmd.Flags().StringVar(&publishCover, "cover", "", "封面图片路径")
	publishRunCmd.Flags().StringVar(&publishCategory, "category", "", "分类")
	publishRunCmd.Flags().StringVar(&publishSchedule, "schedule", "", "定时发布时间（RFC3339 或 2006-01-02T15:04:05）")

	// platform login/check flags
	platformLoginCmd.Flags().StringVar(&publishAccount, "account", "", "账号名（自定义标识，如 company_douyin）")
	platformCheckCmd.Flags().StringVar(&publishAccount, "account", "", "账号名")

	// 子命令树
	publishCmd.AddCommand(publishRunCmd)
	publishCmd.AddCommand(publishStatusCmd)
	publishCmd.AddCommand(publishCancelCmd)
	publishCmd.AddCommand(publishDeleteCmd)

	platformCmd.AddCommand(platformLoginCmd)
	platformCmd.AddCommand(platformCheckCmd)
	platformCmd.AddCommand(platformListCmd)

	rootCmd.AddCommand(publishCmd)
	rootCmd.AddCommand(platformCmd)
}

func statusEmoji(s domain.PublishStatus) string {
	switch s {
	case domain.PublishPublished:
		return "✅"
	case domain.PublishUploading, domain.PublishUploaded:
		return "⏳"
	case domain.PublishFailed:
		return "❌"
	case domain.PublishRejected:
		return "🚫"
	case domain.PublishCanceled:
		return "⬜"
	default:
		return "📝"
	}
}

// publishTagsToDisplay 格式化标签为逗号分隔字符串。
func publishTagsToDisplay(tags []string) string {
	return strings.Join(tags, ", ")
}
