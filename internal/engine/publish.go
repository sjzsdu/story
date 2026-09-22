package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// publishers 持有已注册的平台发布器（key = domain.Platform）。
// 由 App.Bootstrap 注入，engine 不知道具体平台实现。
type publishProviders map[domain.Platform]port.PlatformPublisher

// Publish 发布一集成片到指定平台（§19）。
func (e *Engine) Publish(ctx context.Context, episodeID string, opts port.PublishOptions, providers publishProviders) ([]*domain.PublishJob, error) {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	finalNode := ep.ActiveNodeOfStage(domain.StageFinal)
	if finalNode == nil || !finalNode.Done() || len(finalNode.Outputs) == 0 {
		return nil, fmt.Errorf("集 %s 尚无成片，请先合成", episodeID)
	}
	videoPath := finalNode.Outputs[0]

	// 确定要发布的平台列表
	platforms := opts.Platforms
	if len(platforms) == 0 {
		return nil, fmt.Errorf("未指定发布平台，请用 --platform 指定")
	}

	// 读取故事内容
	title := ep.Title
	description := ""
	if finalNode.Story != nil {
		title = finalNode.Story.Title
		description = finalNode.Story.Summary
	}
	if opts.Title != "" {
		title = opts.Title
	}
	if opts.Description != "" {
		description = opts.Description
	}

	var jobs []*domain.PublishJob
	for _, p := range platforms {
		platform := domain.NormalizePlatform(p)
		publisher, ok := providers[platform]
		if !ok {
			return nil, fmt.Errorf("平台 %s 未注册发布能力", domain.PlatformLabel(platform))
		}

		jobID := fmt.Sprintf("%s-%s-%d", ep.ID, platform, time.Now().UnixMilli())
		job := &domain.PublishJob{
			ID:          jobID,
			EpisodeID:   ep.ID,
			SeriesID:    series.ID,
			Platform:    platform,
			NodeID:      finalNode.ID,
			Status:      domain.PublishPending,
			VideoPath:   videoPath,
			CoverPath:   opts.CoverPath,
			Title:       truncateTitle(title, platform),
			Description: buildDescription(description, series, platform),
			Tags:        buildTags(series, title),
			Category:    opts.Category,
			MaxRetries:  3,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// 定时发布
		if opts.ScheduledAt != nil {
			job.ScheduledAt = opts.ScheduledAt
		}

		if err := e.repo.CreatePublishJob(ctx, job); err != nil {
			return nil, fmt.Errorf("创建 %s 发布任务: %w", domain.PlatformLabel(platform), err)
		}

		// 查找该平台的账号
		accounts, _ := e.repo.ListPlatformAccountsByPlatform(ctx, platform)
		var account *domain.PlatformAccount
		if len(accounts) > 0 {
			account = accounts[0]
		}

		// 非定时任务立即上传
		if job.ScheduledAt == nil {
			job.Status = domain.PublishUploading
			_ = e.repo.SaveEpisode(ctx, ep) // 保存中间状态
			_ = e.repo.UpdatePublishJob(ctx, job)

			result, err := publisher.Upload(ctx, port.PublishRequest{
				VideoPath:   job.VideoPath,
				CoverPath:   job.CoverPath,
				Title:       job.Title,
				Description: job.Description,
				Tags:        job.Tags,
				Category:    job.Category,
				ScheduledAt: job.ScheduledAt,
				Account:     account,
			})
			if err != nil {
				job.Status = domain.PublishFailed
				job.Error = err.Error()
				job.Attempts++
				_ = e.repo.UpdatePublishJob(ctx, job)
				jobs = append(jobs, job)
				continue
			}
			job.PlatformVideoID = result.VideoID
			job.PlatformURL = result.URL
			if result.Error != "" {
				job.Status = domain.PublishFailed
				job.Error = result.Error
			} else {
				job.Status = domain.PublishUploaded
				// 非定时且上传成功 → 直接发布
				if job.ScheduledAt == nil {
					pubResult, pubErr := publisher.Publish(ctx, result.VideoID, account)
					if pubErr != nil {
						job.Status = domain.PublishFailed
						job.Error = pubErr.Error()
					} else if pubResult.Error != "" {
						job.Status = domain.PublishFailed
						job.Error = pubResult.Error
					} else {
						job.Status = domain.PublishPublished
						job.PlatformURL = pubResult.URL
						if pubResult.VideoID != "" {
							job.PlatformVideoID = pubResult.VideoID
						}
					}
					job.Attempts++
				}
			}
			_ = e.repo.UpdatePublishJob(ctx, job)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// CancelPublish 取消发布任务（§19）。
func (e *Engine) CancelPublish(ctx context.Context, jobID string) error {
	job, err := e.repo.GetPublishJob(ctx, jobID)
	if err != nil {
		return err
	}
	if !job.CanCancel() {
		return fmt.Errorf("发布任务 %s 状态为 %s，不可取消", jobID, job.Status)
	}
	job.Status = domain.PublishCanceled
	job.UpdatedAt = time.Now()
	return e.repo.UpdatePublishJob(ctx, job)
}

// DeletePublished 删除已发布的视频（§19）。
func (e *Engine) DeletePublished(ctx context.Context, jobID string, providers publishProviders) error {
	job, err := e.repo.GetPublishJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status != domain.PublishPublished {
		return fmt.Errorf("发布任务 %s 状态为 %s，只能删除已发布的任务", jobID, job.Status)
	}
	publisher, ok := providers[job.Platform]
	if !ok {
		return fmt.Errorf("平台 %s 未注册发布能力", domain.PlatformLabel(job.Platform))
	}
	accounts, _ := e.repo.ListPlatformAccountsByPlatform(ctx, job.Platform)
	var account *domain.PlatformAccount
	if len(accounts) > 0 {
		account = accounts[0]
	}
	if err := publisher.Delete(ctx, job.PlatformVideoID, account); err != nil {
		return fmt.Errorf("删除 %s 视频: %w", domain.PlatformLabel(job.Platform), err)
	}
	job.Status = domain.PublishCanceled
	job.UpdatedAt = time.Now()
	return e.repo.UpdatePublishJob(ctx, job)
}

// ---- 辅助函数 ----

func truncateTitle(title string, platform domain.Platform) string {
	limit := domain.PlatformTitleLimit(platform)
	if len(title) <= limit {
		return title
	}
	return title[:limit-3] + "..."
}

func buildDescription(summary string, series *domain.Series, platform domain.Platform) string {
	var sb strings.Builder
	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}
	// 自动添加标签
	limit := domain.PlatformDescLimit(platform)
	desc := sb.String()
	if len(desc) > limit {
		desc = desc[:limit]
	}
	return desc
}

func buildTags(series *domain.Series, title string) []string {
	var tags []string
	if series.Config.Dynasty != "" {
		tags = append(tags, series.Config.Dynasty)
	}
	tags = append(tags, series.Name)
	// 从标题中提取关键词（简单实现：取前 4 个字）
	runeTitle := []rune(title)
	if len(runeTitle) > 4 {
		tags = append(tags, string(runeTitle[:4]))
	} else {
		tags = append(tags, title)
	}
	return tags
}
