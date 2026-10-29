package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/storage"
)

// seedOSSWithPublicBase 启用对象存储并写入平台访问地址，用于验证内网地址不会直接给上游。
func seedOSSWithPublicBase(t *testing.T, svc *Service, publicBaseURL string) {
	t.Helper()
	settingJSON, err := json.Marshal(ossSettingValue{
		Enabled: true, Provider: aliyunOSSProvider, Endpoint: "https://oss-cn-test.aliyuncs.com",
		Bucket: "test-bucket", AccessKeyID: "ak-test", AccessKeySecret: "sk-test",
		PublicBaseURL: publicBaseURL, VideoStorageTarget: storage.VideoStorageTargetLocal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: ossSettingKey, ValueJSON: string(settingJSON)}); err != nil {
		t.Fatal(err)
	}
}

func stubOSSForProviderCopy(t *testing.T) (*[]string, *[]string) {
	t.Helper()
	puts := []string{}
	deletes := []string{}
	originalPut, originalSign, originalDelete := putOSSObject, signedOriginObjectURL, deleteProviderInputObject
	putOSSObject = func(_ storage.Settings, objectKey string, _ string, _ int64, body io.Reader) (string, error) {
		if _, err := io.ReadAll(body); err != nil {
			return "", err
		}
		puts = append(puts, objectKey)
		return "etag", nil
	}
	signedOriginObjectURL = func(_ storage.Settings, objectKey string, _ time.Time) (string, error) {
		return "https://oss.example/" + objectKey + "?sig=1", nil
	}
	deleteProviderInputObject = func(_ ossSettingValue, objectKey string) error {
		deletes = append(deletes, objectKey)
		return nil
	}
	t.Cleanup(func() {
		putOSSObject, signedOriginObjectURL, deleteProviderInputObject = originalPut, originalSign, originalDelete
	})
	return &puts, &deletes
}

func seedLocalVideoResource(t *testing.T, svc *Service) *model.Resource {
	t.Helper()
	data := mediaTestMP4()
	objectKey := "users/user-1/video/2026/10/29/clip.mp4"
	path := filepath.Join(svc.dataDir, "resources", filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	resource := &model.Resource{
		ID: "res-local-video", UserID: "user-1", Kind: "video", Status: model.ResourceStatusReady,
		Provider: "local", ObjectKey: objectKey, MimeType: "video/mp4", Size: int64(len(data)),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateResource(resource); err != nil {
		t.Fatal(err)
	}
	return resource
}

func TestProviderVisibleURLDerivesObjectStorageCopyForLocalVideo(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	seedOSSWithPublicBase(t, svc, "https://192.168.1.56:3050")
	puts, _ := stubOSSForProviderCopy(t)
	resource := seedLocalVideoResource(t, svc)

	first, err := svc.providerVisibleResourceURL("user-1", resource)
	if err != nil {
		t.Fatalf("providerVisibleResourceURL() error = %v", err)
	}
	if !strings.Contains(first, "provider-input") || !strings.Contains(first, "users/user-1/video") {
		t.Fatalf("signed url = %q, want derived copy key", first)
	}
	if len(*puts) != 1 {
		t.Fatalf("put calls = %d, want 1", len(*puts))
	}

	second, err := svc.providerVisibleResourceURL("user-1", resource)
	if err != nil {
		t.Fatalf("second providerVisibleResourceURL() error = %v", err)
	}
	if len(*puts) != 1 {
		t.Fatalf("copy uploaded again: put calls = %d, want 1", len(*puts))
	}
	if !strings.HasPrefix(second, "https://oss.example/") {
		t.Fatalf("second url = %q", second)
	}

	variants, err := svc.repo.ResourceStorageVariants(resource.ID)
	if err != nil || len(variants) != 1 || variants[0].Purpose != providerInputVariantPurpose {
		t.Fatalf("variants = %#v, err = %v", variants, err)
	}
}

func TestProviderVisibleURLRequiresObjectStorageForLocalVideo(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	settingJSON, err := json.Marshal(ossSettingValue{Enabled: false, VideoStorageTarget: storage.VideoStorageTargetLocal})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: ossSettingKey, ValueJSON: string(settingJSON)}); err != nil {
		t.Fatal(err)
	}
	puts, _ := stubOSSForProviderCopy(t)
	resource := seedLocalVideoResource(t, svc)

	_, err = svc.providerVisibleResourceURL("user-1", resource)
	if err == nil || !strings.Contains(err.Error(), "对象存储") {
		t.Fatalf("error = %v, want object storage requirement", err)
	}
	if len(*puts) != 0 {
		t.Fatalf("put calls = %d, want 0", len(*puts))
	}
}

func TestDeleteStoredResourceObjectRemovesDerivedCopy(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)
	_, deletes := stubOSSForProviderCopy(t)
	resource := seedLocalVideoResource(t, svc)

	if _, err := svc.providerVisibleResourceURL("user-1", resource); err != nil {
		t.Fatalf("providerVisibleResourceURL() error = %v", err)
	}
	if err := svc.deleteStoredResourceObject("user-1", resource); err != nil {
		t.Fatalf("deleteStoredResourceObject() error = %v", err)
	}
	if len(*deletes) != 1 || !strings.Contains((*deletes)[0], "provider-input") {
		t.Fatalf("deleted keys = %#v, want derived copy", *deletes)
	}
	variants, err := svc.repo.ResourceStorageVariants(resource.ID)
	if err != nil || len(variants) != 0 {
		t.Fatalf("variants after delete = %#v, err = %v", variants, err)
	}
}
