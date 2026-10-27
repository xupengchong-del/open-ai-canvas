package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"yingce/backend/internal/model"
)

const (
	BatchVideoPurposeMaterialImage  = "material_image"
	BatchVideoPurposeMaterialVideo  = "material_video"
	BatchVideoPurposeDocument       = "material_document"
	BatchVideoPurposeVideoReference = "video_reference"
)

// BatchVideoLocalStoragePolicy 写入任务输入 metadata；批量视频的生成产物不跟随全局 OSS 策略。
const BatchVideoLocalStoragePolicy = "local_only_except_video_reference"

// TaskForcesLocalGeneratedMedia 判断任务是否声明了产物本地保存。
func TaskForcesLocalGeneratedMedia(task *model.Task) bool {
	if task == nil || strings.TrimSpace(task.InputJSON) == "" {
		return false
	}
	var input struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
		return false
	}
	policy, _ := input.Metadata["storagePolicy"].(string)
	return strings.TrimSpace(policy) == BatchVideoLocalStoragePolicy
}

// BatchVideoStoragePurposeAllowsOSS 只允许视频生成实际引用的参考图进入对象存储。
// 其余批量视频文件必须留在服务器本地，即使全局 OSS 开关开启。
func BatchVideoStoragePurposeAllowsOSS(purpose string) bool {
	return purpose == BatchVideoPurposeVideoReference
}

func normalizeBatchVideoPurpose(purpose string) (string, error) {
	switch strings.TrimSpace(purpose) {
	case BatchVideoPurposeMaterialImage:
		return BatchVideoPurposeMaterialImage, nil
	case BatchVideoPurposeMaterialVideo:
		return BatchVideoPurposeMaterialVideo, nil
	case BatchVideoPurposeDocument:
		return BatchVideoPurposeDocument, nil
	default:
		return "", BadAuthRequest("批量视频上传用途仅支持 material_image、material_video 或 material_document")
	}
}

// UploadBatchVideoResource 是批量视频公开上传入口；所有上传均保存在本地。
// 视频参考图的 OSS 副本只能由后续受控生成流程创建，不能由客户端直接指定。
func (s *Service) UploadBatchVideoResource(userID string, header *multipart.FileHeader, kind, purpose string, width, height int, durationMs int64, uploadIdentity ...string) (*model.Resource, error) {
	if header == nil {
		return nil, BadAuthRequest("请选择要上传的文件")
	}
	purpose, err := normalizeBatchVideoPurpose(purpose)
	if err != nil {
		return nil, err
	}
	if kind == "" {
		kind = "file"
	}
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	mimeType, err := detectUploadedMimeType(file)
	if err != nil {
		return nil, err
	}
	if purpose == BatchVideoPurposeDocument {
		extension := strings.ToLower(filepath.Ext(header.Filename))
		if extension != ".md" && extension != ".txt" {
			return nil, BadAuthRequest("批量视频文档仅支持 .md 或 .txt 文件")
		}
		switch mimeType {
		case "text/plain", "text/markdown", "text/x-markdown":
		default:
			return nil, BadAuthRequest("批量视频文档内容必须是纯文本或 Markdown")
		}
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}
	contentHash := hex.EncodeToString(hasher.Sum(nil))
	if existing, err := s.repo.ResourceByContentHashProvider(userID, contentHash, "local"); err == nil && existing != nil {
		return existing, nil
	}
	resource, err := s.uploadResource(userID, header, kind, width, height, durationMs, true, uploadIdentity...)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, fmt.Errorf("上传资源失败")
	}
	if resource.ContentHash == nil || *resource.ContentHash == "" {
		if err := s.repo.SaveResourceContentHash(userID, resource.ID, contentHash); err != nil {
			// 并发上传同一内容时唯一索引会拒绝第二份哈希；返回同 provider 的已存在资源。
			if existing, lookupErr := s.repo.ResourceByContentHashProvider(userID, contentHash, resource.Provider); lookupErr == nil && existing != nil {
				return existing, nil
			}
			return nil, err
		}
		resource.ContentHash = &contentHash
	}
	if resource.Provider != "local" {
		return nil, BadAuthRequest("批量视频上传文件必须保存在服务器本地")
	}
	return resource, nil
}
