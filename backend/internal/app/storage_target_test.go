package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/storage"
)

func seedSystemOSSStorageTarget(t *testing.T, svc *Service, target string) {
	t.Helper()
	settingJSON, err := json.Marshal(ossSettingValue{
		Enabled:            true,
		Provider:           aliyunOSSProvider,
		Endpoint:           "http://127.0.0.1:1",
		Bucket:             "test-bucket",
		AccessKeyID:        "ak-test",
		AccessKeySecret:    "sk-test",
		VideoStorageTarget: target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: ossSettingKey, ValueJSON: string(settingJSON)}); err != nil {
		t.Fatal(err)
	}
}

func assertLocalResourceStored(t *testing.T, svc *Service, userID string, resourceID string) {
	t.Helper()
	if resourceID == "" {
		t.Fatal("result has no resourceId")
	}
	resource, err := svc.repo.ResourceForUser(userID, resourceID)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Provider != "local" {
		t.Fatalf("resource.Provider = %q, want local", resource.Provider)
	}
	if _, err := os.Stat(filepath.Join(svc.dataDir, "resources", filepath.FromSlash(resource.ObjectKey))); err != nil {
		t.Fatalf("local object missing: %v", err)
	}
}

func TestPersistGeneratedImageHonorsLocalStorageTarget(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)

	stored, err := svc.persistGeneratedMediaResult("user-1", generatedImageResult(mediaTestPNG(t)))
	if err != nil {
		t.Fatalf("persistGeneratedMediaResult() error = %v", err)
	}
	images, ok := stored["images"].([]interface{})
	if !ok || len(images) != 1 {
		t.Fatalf("stored images = %#v", stored["images"])
	}
	item, ok := images[0].(map[string]interface{})
	if !ok {
		t.Fatalf("stored image = %#v", images[0])
	}
	assertLocalResourceStored(t, svc, "user-1", stringField(item, "resourceId"))
}

func TestPersistGeneratedImageUsesOSSWhenStorageTargetIsOSS(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetOSS)

	if _, err := svc.persistGeneratedMediaResult("user-1", generatedImageResult(mediaTestPNG(t))); err == nil {
		t.Fatal("persistGeneratedMediaResult() error = nil, want OSS upload failure")
	}
}

func TestPersistGeneratedAudioIgnoresVisualStorageTarget(t *testing.T) {
	svc, _ := newMediaRecoveryTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)

	_, err := svc.persistGeneratedMediaResult("user-1", map[string]interface{}{
		"mode":  "audio",
		"audio": map[string]interface{}{"dataUrl": dataURL("audio/mpeg", []byte("fake-mp3"))},
	})
	if err == nil {
		t.Fatal("persistGeneratedMediaResult() error = nil, want OSS upload failure for audio")
	}
}

func generatedImageResult(data []byte) map[string]interface{} {
	return map[string]interface{}{
		"mode": "image",
		"images": []interface{}{map[string]interface{}{
			"dataUrl":  dataURL("image/png", data),
			"mimeType": "image/png",
		}},
	}
}

func TestMaterializeGeneratedMediaHonorsLocalStorageTarget(t *testing.T) {
	for _, mode := range []string{"image", "video"} {
		t.Run(mode, func(t *testing.T) {
			s, db := newMediaRecoveryTestService(t)
			seedSystemOSSStorageTarget(t, s, storage.VideoStorageTargetLocal)
			task := seedMediaTask(t, db, providerConfig{})

			data := mediaTestPNG(t)
			mimeType := "image/png"
			if mode == "video" {
				data = []byte("fake-mp4-bytes")
				mimeType = "video/mp4"
			}
			tempName, err := s.stageInlineMedia(data)
			if err != nil {
				t.Fatal(err)
			}
			checkpoint := &mediaCheckpoint{
				Mode:      mode,
				StartedAt: time.Now(),
				Items:     []mediaCheckpointItem{{TempName: tempName, MIMEType: mimeType}},
			}
			output, err := s.materializeTaskMedia(context.Background(), task, providerConfig{}, checkpoint)
			if err != nil {
				t.Fatalf("materializeTaskMedia() error = %v", err)
			}
			var item map[string]interface{}
			if mode == "image" {
				items, ok := output["images"].([]interface{})
				if !ok || len(items) != 1 {
					t.Fatalf("output images = %#v", output["images"])
				}
				item, _ = items[0].(map[string]interface{})
			} else {
				item, _ = output["video"].(map[string]interface{})
			}
			assertLocalResourceStored(t, s, task.UserID, stringField(item, "resourceId"))
		})
	}
}
