package douyin

import (
	"context"
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/provider/publish/sau"
)

// Provider 抖音发布 provider（sau CLI 包装）。
type Provider struct {
	client *sau.Client
}

// New 创建抖音 provider。
func New(client *sau.Client) *Provider {
	return &Provider{client: client}
}

// Platform 返回平台标识。
func (p *Provider) Platform() domain.Platform {
	return domain.PlatformDouyin
}

// Upload 上传视频到抖音。
func (p *Provider) Upload(ctx context.Context, req port.PublishRequest) (*port.PublishResult, error) {
	account := req.Account
	if account == nil {
		return nil, fmt.Errorf("抖音发布需要账号凭证")
	}
	result, err := p.client.UploadVideo(ctx, sau.UploadVideoRequest{
		Platform: "douyin",
		Account:  account.AccountName,
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

// Publish 抖音通过 sau 上传即发布，无需额外步骤。
func (p *Provider) Publish(ctx context.Context, videoID string, account *domain.PlatformAccount) (*port.PublishResult, error) {
	return &port.PublishResult{
		VideoID: videoID,
		Status:  domain.PublishPublished,
	}, nil
}

// Status sau 不提供状态查询，返回不支持。
func (p *Provider) Status(ctx context.Context, videoID string, account *domain.PlatformAccount) (domain.PublishStatus, string, error) {
	return domain.PublishPublished, "已通过 sau 发布", nil
}

// Delete sau 不支持删除视频。
func (p *Provider) Delete(ctx context.Context, videoID string, account *domain.PlatformAccount) error {
	return fmt.Errorf("sau 暂不支持删除视频，请手动操作")
}

// UploadCover sau 不支持单独上传封面。
func (p *Provider) UploadCover(ctx context.Context, videoID, coverPath string, account *domain.PlatformAccount) error {
	return fmt.Errorf("sau 封面请在上传时通过 cover 参数设置")
}

func firstError(r *sau.UploadResult) string {
	if !r.Success {
		return r.Message
	}
	return ""
}
