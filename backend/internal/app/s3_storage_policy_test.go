package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/storage"
)

// s3OnlySystemSetting 是「生产上只有 S3 可用」的基线配置；用例可在它的副本上覆写分发策略。
func s3OnlySystemSetting() ossSettingValue {
	return ossSettingValue{
		Enabled: true, Provider: s3Provider, Endpoint: "https://s3.example.com",
		Bucket: "test-bucket", AccessKeyID: "ak-test", AccessKeySecret: "sk-test",
		Region: "us-east-1", S3Preset: "aws", PathPrefix: defaultOSSPathPrefix,
		VideoStorageTarget: storage.VideoStorageTargetOSS,
	}
}

func saveSystemOSSValue(t *testing.T, svc *Service, value ossSettingValue) {
	t.Helper()
	settingJSON, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: ossSettingKey, ValueJSON: string(settingJSON)}); err != nil {
		t.Fatal(err)
	}
}

func seedSystemS3Only(t *testing.T, svc *Service) {
	t.Helper()
	saveSystemOSSValue(t, svc, s3OnlySystemSetting())
}

func TestS3OnlyStorageKeepsOrdinaryUploadsLocal(t *testing.T) {
	svc := newResourceTestService(t)
	seedSystemS3Only(t, svc)

	data := mediaTestPNG(t)
	resource, err := svc.UploadResourceFile("user-1", "ordinary.png", int64(len(data)), "image", 0, 0, 0, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("UploadResourceFile() error = %v", err)
	}
	assertLocalResourceStored(t, svc, "user-1", resource.ID)

	_, _, enabled, err := svc.activeResourceOSSSetting("user-1")
	if err != nil {
		t.Fatalf("activeResourceOSSSetting() error = %v", err)
	}
	if enabled {
		t.Fatal("activeResourceOSSSetting() enabled S3 without video reference purpose")
	}

	_, _, enabled, err = svc.activeResourceOSSSetting("user-1", BatchVideoPurposeVideoReference)
	if err != nil {
		t.Fatalf("activeResourceOSSSetting(video_reference) error = %v", err)
	}
	if !enabled {
		t.Fatal("activeResourceOSSSetting(video_reference) did not allow S3")
	}
}

func TestS3OffloadJobIsCancelledByPolicy(t *testing.T) {
	svc, db := newMediaRecoveryTestService(t)
	seedSystemS3Only(t, svc)
	puts, _ := stubOSSForProviderCopy(t)

	data := []byte("generated-image")
	localKey := "users/user-1/image/2026/11/25/generated.png"
	path := filepath.Join(svc.dataDir, "resources", filepath.FromSlash(localKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	resource := &model.Resource{
		ID: newID(), UserID: "user-1", Kind: "image", Status: model.ResourceStatusReady,
		Provider: "local", ObjectKey: localKey, MimeType: "image/png", Size: int64(len(data)),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateResource(resource); err != nil {
		t.Fatal(err)
	}
	job := &model.ResourceOffloadJob{
		ID: newID(), UserID: "user-1", ResourceID: resource.ID, TaskID: "task-1",
		Size: resource.Size, LocalObjectKey: localKey,
		TargetProvider: "S3", TargetEndpoint: "https://s3.example.com", TargetBucket: "test-bucket",
		TargetObjectKey: "prefix/generated.png", Status: model.ResourceOffloadStatusProcessing,
		NextAttemptAt: time.Now(), LeaseOwner: "worker", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.UpsertResourceOffloadJob(job); err != nil {
		t.Fatal(err)
	}

	svc.processResourceOffloadJob(context.Background(), job, "worker", map[string]bool{})
	var stored model.ResourceOffloadJob
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.ResourceOffloadStatusCancelled {
		t.Fatalf("offload status = %q, want cancelled", stored.Status)
	}
	if len(*puts) != 0 {
		t.Fatalf("S3 put calls = %d, want 0", len(*puts))
	}
}

// 存量 S3 素材必须可读：对象已经在 S3 里，读取不产生新的存储占用，
// 所以「provider 是 S3」这一事实本身就是放行条件，不再按本次用途判定。
// 大写/空格变体同样放行，避免大小写绕过归一化后落到「按用途报错」的旧分支。
func TestProviderVisibleURLAllowsExistingUppercaseS3Resource(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	// 生产 NAS 的 S3 通过 CDN 分发；这里用同一份配置，让签发走真实分支，
	// 从而验证「放行与否」只取决于 provider 是否为 S3，而不是分发细节。
	systemValue := s3OnlySystemSetting()
	systemValue.CDNBaseURL = "https://media.example.com"
	systemValue.Delivery = storage.DeliverySettings{CDNAuthMode: "public"}
	saveSystemOSSValue(t, svc, systemValue)
	puts, _ := stubOSSForProviderCopy(t)

	for _, provider := range []string{"S3", "s3", " S3 "} {
		resource := &model.Resource{
			ID: newID(), UserID: "user-1", Kind: "image", Status: model.ResourceStatusReady,
			Provider: provider, Endpoint: "https://s3.example.com", Bucket: "test-bucket",
			ObjectKey: "users/user-1/image/reference.png", MimeType: "image/png", Size: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := svc.repo.CreateResource(resource); err != nil {
			t.Fatal(err)
		}
		// 不传 storagePurpose 等价于「非视频参考图用途」（图片生成参考、创作对话引用等）。
		signedURL, err := svc.providerVisibleResourceURL("user-1", resource)
		if err != nil {
			t.Fatalf("provider %q: providerVisibleResourceURL() error = %v, want existing S3 resource readable", provider, err)
		}
		if signedURL == "" {
			t.Fatalf("provider %q: providerVisibleResourceURL() returned empty url", provider)
		}

		// 生成派发路径（hydrateProviderMedia）同样要能读到存量 S3 素材。
		media := providerMedia{StorageKey: "resource:" + resource.ID}
		if err := svc.hydrateProviderMedia("user-1", &media, providerMediaHydrationPolicy{preferURL: true}); err != nil {
			t.Fatalf("provider %q: hydrateProviderMedia() error = %v, want existing S3 resource readable", provider, err)
		}
		if media.URL == "" || media.DataURL != "" {
			t.Fatalf("provider %q: hydrated media = %#v, want url only", provider, media)
		}
	}
	// 读放行不等于写放行：读库存量对象不应产生任何 S3 put。
	if len(*puts) != 0 {
		t.Fatalf("put calls = %d, want 0", len(*puts))
	}
}

func seedLocalImageResourceForS3Copy(t *testing.T, svc *Service) *model.Resource {
	t.Helper()
	data := mediaTestPNG(t)
	objectKey := "users/user-1/image/2026/11/25/reference.png"
	path := filepath.Join(svc.dataDir, "resources", filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	resource := &model.Resource{
		ID: newID(), UserID: "user-1", Kind: "image", Status: model.ResourceStatusReady,
		Provider: "local", ObjectKey: objectKey, MimeType: "image/png", Size: int64(len(data)),
		Width: 2, Height: 3, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateResource(resource); err != nil {
		t.Fatal(err)
	}
	return resource
}

func TestProviderVisibleURLUsesS3OnlyForVideoReferenceImage(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	seedSystemS3Only(t, svc)
	puts, _ := stubOSSForProviderCopy(t)
	image := seedLocalImageResourceForS3Copy(t, svc)

	if _, err := svc.providerVisibleResourceURL("user-1", image); err == nil || !strings.Contains(err.Error(), "S3") {
		t.Fatalf("ordinary image error = %v, want S3 restriction", err)
	}
	if len(*puts) != 0 {
		t.Fatalf("ordinary image put calls = %d, want 0", len(*puts))
	}

	if _, err := svc.providerVisibleResourceURL("user-1", image, BatchVideoPurposeVideoReference); err != nil {
		t.Fatalf("video reference image error = %v", err)
	}
	if len(*puts) != 1 {
		t.Fatalf("video reference image put calls = %d, want 1", len(*puts))
	}

	video := seedLocalVideoResourceForCopy(t, svc)
	if _, err := svc.providerVisibleResourceURL("user-1", video, BatchVideoPurposeVideoReference); err == nil || !strings.Contains(err.Error(), "图片") {
		t.Fatalf("video reference purpose with video error = %v, want image restriction", err)
	}
	if len(*puts) != 1 {
		t.Fatalf("put calls after video resource = %d, want 1", len(*puts))
	}
}
