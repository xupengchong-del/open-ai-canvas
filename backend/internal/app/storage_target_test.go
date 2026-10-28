package app

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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

// 画布生成的视频由浏览器回传，走普通上传接口；它必须和任务落盘路径遵守同一份存储策略。
func mediaTestMP4() []byte {
	return []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
}

func videoUploadHeader(t *testing.T, data []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="clip.mp4"`)
	h.Set("Content-Type", "video/mp4")
	p, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	p.Write(data)
	w.Close()
	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { form.RemoveAll() })
	return form.File["file"][0]
}

func TestUploadVideoHonorsLocalStorageTarget(t *testing.T) {
	svc := newResourceTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)

	resource, err := svc.UploadResource("user-1", videoUploadHeader(t, mediaTestMP4()), "video", 0, 0, 0)
	if err != nil {
		t.Fatalf("UploadResource() error = %v", err)
	}
	if resource.Kind != "video" {
		t.Fatalf("resource.Kind = %q, want video", resource.Kind)
	}
	assertLocalResourceStored(t, svc, "user-1", resource.ID)
}

func TestUploadVideoFileHonorsLocalStorageTarget(t *testing.T) {
	svc := newResourceTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)

	data := mediaTestMP4()
	resource, err := svc.UploadResourceFile("user-1", "clip.mp4", int64(len(data)), "video", 0, 0, 0, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("UploadResourceFile() error = %v", err)
	}
	assertLocalResourceStored(t, svc, "user-1", resource.ID)
}

func TestImportVideoHonorsLocalStorageTarget(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	svc := newResourceTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)

	data := mediaTestMP4()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	resource, err := svc.ImportResourceURL("user-1", server.URL+"/clip.mp4", "video", 0, 0, 0, "import-video")
	if err != nil {
		t.Fatalf("ImportResourceURL() error = %v", err)
	}
	assertLocalResourceStored(t, svc, "user-1", resource.ID)
}

func TestUploadVideoStillUsesOSSWhenStorageTargetIsOSS(t *testing.T) {
	svc := newResourceTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetOSS)

	if _, err := svc.UploadResource("user-1", videoUploadHeader(t, mediaTestMP4()), "video", 0, 0, 0); err == nil {
		t.Fatal("UploadResource() error = nil, want OSS upload failure")
	}
}

// 图片保持原行为：批量视频参考图依赖对象存储的公网地址。
func TestUploadImageStillUsesOSSWhenStorageTargetIsLocal(t *testing.T) {
	svc := newResourceTestService(t)
	seedSystemOSSStorageTarget(t, svc, storage.VideoStorageTargetLocal)

	if _, err := svc.UploadResource("user-1", contentUploadHeader(t, mediaTestPNG(t)), "image", 0, 0, 0); err == nil {
		t.Fatal("UploadResource() error = nil, want OSS upload attempt for images")
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
