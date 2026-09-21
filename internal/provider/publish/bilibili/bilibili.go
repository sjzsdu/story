package bilibili

import (
	"context"
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/provider/publish/sau"
)

// Provider B站发布 provider（sau CLI 包装）。
type Provider struct {
	client *sau.Client
	// DefaultTid 默认分区 ID（B站投稿需要，249 = 知识科普）。
	DefaultTid int
}

// New 创建 B站 provider。defaultTid 为 0 时默认 249（知识科普）。
func New(client *sau.Client, defaultTid int) *Provider {
	if defaultTid <= 0 {
		defaultTid = 249
	}
	return &Provider{client: client, DefaultTid: defaultTid}
}

func (p *Provider) Platform() domain.Platform { return domain.PlatformBilibili }

func (p *Provider) Upload(ctx context.Context, req port.PublishRequest) (*port.PublishResult, error) {
	if req.Account == nil {
		return nil, fmt.Errorf("B站发布需要账号凭证")
	}
	tid := p.DefaultTid
	// 从 Extra 中读取自定义分区 ID（如果设置了的话）
	if req.Extra != nil {
		if tidVal, ok := req.Extra["tid"].(int); ok && tidVal > 0 {
			tid = tidVal
		}
	}
	result, err := p.client.UploadVideo(ctx, sau.UploadVideoRequest{
		Platform: "bilibili",
		Account:  req.Account.AccountName,
		FilePath: req.VideoPath,
		Title:    req.Title,
		Desc:     req.Description,
		Tags:     req.Tags,
		Tid:      tid,
	})
	if err != nil {
		return nil, err
	}
	return &port.PublishResult{
		VideoID: result.VideoID,
		URL:     result.VideoURL,
		Status:  domain.PublishPublished,
		Error:   firstError(result),
	}, nil
}

func (p *Provider) Publish(ctx context.Context, videoID string, account *domain.PlatformAccount) (*port.PublishResult, error) {
	return &port.PublishResult{VideoID: videoID, Status: domain.PublishPublished}, nil
}

func (p *Provider) Status(ctx context.Context, videoID string, account *domain.PlatformAccount) (domain.PublishStatus, string, error) {
	return domain.PublishPublished, "已通过 sau 发布", nil
}

func (p *Provider) Delete(ctx context.Context, videoID string, account *domain.PlatformAccount) error {
	return fmt.Errorf("B站请在创作中心手动删除视频")
}

func (p *Provider) UploadCover(ctx context.Context, videoID, coverPath string, account *domain.PlatformAccount) error {
	return fmt.Errorf("B站封面请在上传时设置")
}

func firstError(r *sau.UploadResult) string {
	if !r.Success {
		return r.Message
	}
	return ""
}
