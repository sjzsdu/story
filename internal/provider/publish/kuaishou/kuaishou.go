package kuaishou

import (
	"context"
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/provider/publish/sau"
)

// Provider 快手发布 provider（sau CLI 包装）。
type Provider struct {
	client *sau.Client
}

// New 创建快手 provider。
func New(client *sau.Client) *Provider {
	return &Provider{client: client}
}

func (p *Provider) Platform() domain.Platform { return domain.PlatformKuaishou }

func (p *Provider) Upload(ctx context.Context, req port.PublishRequest) (*port.PublishResult, error) {
	if req.Account == nil {
		return nil, fmt.Errorf("快手发布需要账号凭证")
	}
	result, err := p.client.UploadVideo(ctx, sau.UploadVideoRequest{
		Platform: "kuaishou",
		Account:  req.Account.AccountName,
		FilePath: req.VideoPath,
		Title:    req.Title,
		Desc:     req.Description,
		Tags:     req.Tags,
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
	return fmt.Errorf("sau 暂不支持删除视频")
}

func (p *Provider) UploadCover(ctx context.Context, videoID, coverPath string, account *domain.PlatformAccount) error {
	return fmt.Errorf("sau 封面请在上传时设置")
}

func firstError(r *sau.UploadResult) string {
	if !r.Success {
		return r.Message
	}
	return ""
}
