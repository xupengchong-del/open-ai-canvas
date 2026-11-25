// 参考素材的公网副本：上游只能下载公网地址，而按存储策略保存的视频在服务器本地。
// 派发前为本地资源派生一份对象存储副本（不搬走、不替换原件），同一资源只派生一次。

package app

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

const providerInputVariantPurpose = "provider-input"

// 变量形式便于测试替换远端删除；生产环境就是 deleteOSSObject。
var deleteProviderInputObject = deleteOSSObject

// providerVisibleResourceURL 返回上游可直接下载的地址。对象存储资源用预签名地址；
// 本地资源（按存储策略落盘的视频）先确保存在对象存储副本，否则上游拿到的是内网平台地址。
//
// 这里必须严格区分「读」和「写」两种语义：
//   - 读已存在的 S3 对象：放行。对象早已落地，读取不产生新的存储占用；生产库里有大量
//     provider=s3 的存量素材（图片生成参考、创作对话引用、历史批量视频素材等），
//     一律按「当前用途不是视频参考图」拦读，会让策略收紧的当天这批历史资源全部不可用。
//   - 写/新建 S3 副本：继续严格受限。非视频参考图用途既不能新建 S3 对象，也不能把本地
//     资源派生成 S3 副本，见下面的图片类型校验、activeResourceOSSSetting 的默认拒绝、
//     ensureProviderInputCopyURL 的报错以及 offload worker 的取消逻辑。
func (s *Service) providerVisibleResourceURL(userID string, resource *model.Resource, storagePurposes ...string) (string, error) {
	if resource == nil {
		return "", errors.New("任务参考资源为空")
	}
	// 读放行：provider 归一化后是 S3（含大小写/空格变体）说明对象已经存在于 S3，
	// 不按本次用途报错，直接签发地址。
	if isS3Provider(resource.Provider) {
		return s.providerResourceURL(resource, time.Time{})
	}
	purpose := firstStoragePurpose(storagePurposes)
	// 写受限：只有本次用途允许落 S3 时才校验「要被写入/派生的素材本身必须是图片」，
	// 挡住把视频、音频、文档派生成 S3 副本的路径。
	if storagePurposeAllowsS3(purpose) && (resource.Kind != "image" || !strings.HasPrefix(strings.ToLower(resource.MimeType), "image/")) {
		return "", errors.New("S3 仅允许保存视频参考图片")
	}
	if resource.Provider != "local" {
		return s.providerResourceURL(resource, time.Time{})
	}
	// 平台地址本身公网可达时，签名地址上游直接能下载；本地/内网地址必须派生对象存储副本。
	if signed, err := s.providerResourceURL(resource, time.Time{}); err == nil && s.platformReachableFromInternet() {
		return signed, nil
	}
	return s.ensureProviderInputCopyURL(userID, resource, storagePurposes...)
}

// platformReachableFromInternet 判断服务器访问地址是否是公网可达主机；
// 本机、局域网和 mDNS 地址只有用户自己的浏览器能打开。
func (s *Service) platformReachableFromInternet() bool {
	base, err := s.publicResourceBaseURL()
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(base.Hostname()))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
	}
	return true
}

func (s *Service) ensureProviderInputCopyURL(userID string, resource *model.Resource, storagePurposes ...string) (string, error) {
	purpose := firstStoragePurpose(storagePurposes)
	setting, storageSettingID, enabled, err := s.activeResourceOSSSetting(userID, purpose)
	if err != nil {
		return "", err
	}
	if !enabled {
		blocked, blockedErr := s.s3OnlyStorageBlocked(userID)
		if blockedErr != nil {
			return "", blockedErr
		}
		if blocked {
			return "", BadAuthRequest("S3 仅允许保存视频生成实际引用的参考图；其他素材请使用服务器本地或其他对象存储")
		}
		return "", BadAuthRequest("参考素材需要对象存储提供上游可下载的地址，请先在后台启用存储服务，或改用公网素材链接")
	}
	setting = s.storageSettingWithRuntimePolicy(setting)
	expiresAt := time.Now().Add(providerInputURLTTL(setting))

	variant, lookupErr := s.repo.ResourceStorageVariant(resource.ID, providerInputVariantPurpose, setting.Provider, setting.Bucket)
	switch {
	case lookupErr == nil && variant != nil:
		if signed, signErr := signedOriginObjectURL(setting, variant.ObjectKey, expiresAt); signErr == nil {
			return signed, nil
		}
	case lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound):
		return "", lookupErr
	}

	objectKey := providerInputCopyKey(setting, resource)
	source, err := os.Open(filepath.Join(s.dataDir, "resources", filepath.FromSlash(resource.ObjectKey)))
	if err != nil {
		return "", fmt.Errorf("读取本地参考素材失败：%w", err)
	}
	defer source.Close()
	if _, err := putOSSObject(setting, objectKey, resource.MimeType, resource.Size, source); err != nil {
		return "", fmt.Errorf("参考素材上传对象存储失败：%w", err)
	}
	created := &model.ResourceStorageVariant{
		ID:               newID(),
		ResourceID:       resource.ID,
		UserID:           resource.UserID,
		Purpose:          providerInputVariantPurpose,
		Provider:         setting.Provider,
		Endpoint:         setting.Endpoint,
		Bucket:           setting.Bucket,
		ObjectKey:        objectKey,
		StorageSettingID: storageSettingID,
		CreatedAt:        time.Now(),
	}
	if err := s.repo.CreateResourceStorageVariant(created); err != nil {
		// 并发派发时唯一索引会拒绝第二份记录，复用已存在的那一份。
		existing, lookupErr := s.repo.ResourceStorageVariant(resource.ID, providerInputVariantPurpose, setting.Provider, setting.Bucket)
		if lookupErr != nil || existing == nil {
			return "", err
		}
		objectKey = existing.ObjectKey
	}
	return signedOriginObjectURL(setting, objectKey, expiresAt)
}

// deleteProviderInputVariants 在资源删除后清理派生副本；失败只记日志，不阻断删除。
func (s *Service) deleteProviderInputVariants(userID string, resource *model.Resource) {
	variants, err := s.repo.ResourceStorageVariants(resource.ID)
	if err != nil || len(variants) == 0 {
		return
	}
	// 派生的 S3 副本只允许服务于视频参考图；清理时按同样用途解析配置，
	// 否则 S3-only 环境会因为默认拒绝 S3 而永远无法删除这些副本。
	for _, variant := range variants {
		target := &model.Resource{
			ID: resource.ID, UserID: variant.UserID, Provider: variant.Provider,
			Endpoint: variant.Endpoint, Bucket: variant.Bucket,
			StorageSettingID: variant.StorageSettingID, ObjectKey: variant.ObjectKey,
		}
		variantSetting, variantErr := s.ossSettingForResource(variant.UserID, target)
		if variantErr != nil || variantSetting.AccessKeyID == "" || variantSetting.AccessKeySecret == "" {
			slog.Warn("参考素材副本配置不可用，跳过清理", "resourceId", resource.ID, "variantId", variant.ID, "error", variantErr)
			continue
		}
		variantSetting = s.storageSettingWithRuntimePolicy(variantSetting)
		if err := deleteProviderInputObject(variantSetting, variant.ObjectKey); err != nil {
			log.Printf("清理参考素材副本失败 resource=%s key=%s: %v", resource.ID, variant.ObjectKey, err)
			continue
		}
		if err := s.repo.DeleteResourceStorageVariant(variant.ID); err != nil {
			log.Printf("删除参考素材副本记录失败 variant=%s: %v", variant.ID, err)
		}
	}
}

func providerInputURLTTL(setting ossSettingValue) time.Duration {
	if setting.Runtime.ProviderAccessURLTTL > 0 {
		return setting.Runtime.ProviderAccessURLTTL
	}
	return 4 * time.Hour
}

func providerInputCopyKey(setting ossSettingValue, resource *model.Resource) string {
	ext := resourceFileExtension(resource.ObjectKey, resource.MimeType, resource.Kind)
	return path.Join(setting.PathPrefix, "users", safeObjectSegment(resource.UserID), resource.Kind, "provider-input", resource.ID+ext)
}
