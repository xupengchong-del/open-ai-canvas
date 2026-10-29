// 参考素材的公网副本：上游只能下载公网地址，而按存储策略保存的视频在服务器本地。
// 派发前为本地资源派生一份对象存储副本（不搬走、不替换原件），同一资源只派生一次。

package app

import (
	"errors"
	"fmt"
	"log"
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
func (s *Service) providerVisibleResourceURL(userID string, resource *model.Resource) (string, error) {
	if resource == nil {
		return "", errors.New("任务参考资源为空")
	}
	if resource.Provider != "local" {
		return s.providerResourceURL(resource, time.Time{})
	}
	// 平台地址本身公网可达时，签名地址上游直接能下载；本地/内网地址必须派生对象存储副本。
	if signed, err := s.providerResourceURL(resource, time.Time{}); err == nil && s.platformReachableFromInternet() {
		return signed, nil
	}
	return s.ensureProviderInputCopyURL(userID, resource)
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

func (s *Service) ensureProviderInputCopyURL(userID string, resource *model.Resource) (string, error) {
	setting, storageSettingID, enabled, err := s.activeResourceOSSSetting(userID)
	if err != nil {
		return "", err
	}
	if !enabled {
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
	setting, _, enabled, err := s.activeResourceOSSSetting(userID)
	if err != nil || !enabled {
		return
	}
	setting = s.storageSettingWithRuntimePolicy(setting)
	for _, variant := range variants {
		if variant.Provider != setting.Provider || variant.Bucket != setting.Bucket {
			continue
		}
		if err := deleteProviderInputObject(setting, variant.ObjectKey); err != nil {
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
