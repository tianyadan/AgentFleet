package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"colleague-avatar/server/internal/store"
)

// UploadAgentAvatar 压缩并上传到 OSS，更新 avatar_url。
func (s *Service) UploadAgentAvatar(ctx context.Context, agentID int64, raw []byte) (*store.ManagedAgent, error) {
	if !s.Cfg.OSSConfigured() {
		return nil, fmt.Errorf("未配置 OSS")
	}
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, err
	}
	if len(raw) > 8<<20 {
		return nil, fmt.Errorf("图片过大（上限 8MB）")
	}
	jpg, err := CompressAvatarImage(raw, avatarMaxEdge)
	if err != nil {
		return nil, err
	}
	objName := fmt.Sprintf("agent-%d-%d.jpg", agentID, time.Now().Unix())
	prefix := strings.Trim(s.Cfg.OSSPrefix, "/")
	key := objName
	if prefix != "" {
		key = prefix + "/" + objName
	}
	client, err := oss.New(s.Cfg.OSSEndpoint, s.Cfg.OSSAccessKeyID, s.Cfg.OSSAccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("oss client: %w", err)
	}
	bucket, err := client.Bucket(s.Cfg.OSSBucket)
	if err != nil {
		return nil, fmt.Errorf("oss bucket: %w", err)
	}
	if err := bucket.PutObject(key, bytes.NewReader(jpg), oss.ContentType("image/jpeg")); err != nil {
		return nil, fmt.Errorf("oss upload: %w", err)
	}
	url := BuildAvatarObjectURL(s.Cfg.OSSPublicBase, s.Cfg.OSSPrefix, objName)
	if err := s.Store.SetManagedAgentAvatarURL(ctx, agentID, url); err != nil {
		return nil, err
	}
	return s.Store.GetManagedAgent(ctx, agentID)
}

// ClearAgentAvatar 清空头像（展示默认图）。
func (s *Service) ClearAgentAvatar(ctx context.Context, agentID int64) (*store.ManagedAgent, error) {
	a, err := s.Store.GetManagedAgent(ctx, agentID)
	if err != nil || a == nil {
		if a == nil && err == nil {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, err
	}
	if err := s.Store.SetManagedAgentAvatarURL(ctx, agentID, ""); err != nil {
		return nil, err
	}
	return s.Store.GetManagedAgent(ctx, agentID)
}
