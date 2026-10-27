package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestBatchVideoUploadRejectsMissingUnknownAndReferencePurposes(t *testing.T) {
	for _, purpose := range []string{"", "unknown", BatchVideoPurposeVideoReference} {
		t.Run(purpose, func(t *testing.T) {
			s, db := newMediaRecoveryTestService(t)
			seedOSSEnabled(t, db, "user-1", "http://127.0.0.1:1")
			header := multipartFileHeader(t, "reference.png", "image/png", mediaTestPNG(t))
			resource, err := s.UploadBatchVideoResource("user-1", header, "image", purpose, 2, 3, 0)
			if err == nil || resource != nil {
				t.Fatalf("purpose %q accepted: resource=%+v err=%v", purpose, resource, err)
			}
			var count int64
			if err := db.Model(&model.Resource{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("rejected upload persisted %d resources", count)
			}
		})
	}
}

func TestBatchVideoUploadKeepsAcceptedPurposesLocalWithOSSEnabled(t *testing.T) {
	mp4 := append([]byte{0x00, 0x00, 0x00, 0x14}, []byte("ftypisom\x00\x00\x00\x00isom")...)
	for _, tc := range []struct {
		purpose  string
		name     string
		declared string
		data     []byte
		kind     string
		mimeType string
	}{
		{purpose: BatchVideoPurposeMaterialImage, name: "material.png", declared: "image/png", data: mediaTestPNG(t), kind: "image", mimeType: "image/png"},
		{purpose: BatchVideoPurposeMaterialVideo, name: "material.mp4", declared: "video/mp4", data: mp4, kind: "video", mimeType: "video/mp4"},
		{purpose: BatchVideoPurposeDocument, name: "material.MD", declared: "application/pdf", data: []byte("# Product\n\nPlain text"), kind: "file", mimeType: "text/plain"},
	} {
		t.Run(tc.purpose, func(t *testing.T) {
			s, db := newMediaRecoveryTestService(t)
			seedOSSEnabled(t, db, "user-1", "http://127.0.0.1:1")
			header := multipartFileHeader(t, tc.name, tc.declared, tc.data)
			resource, err := s.UploadBatchVideoResource("user-1", header, tc.kind, tc.purpose, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if resource.Provider != "local" || resource.Kind != tc.kind || resource.MimeType != tc.mimeType {
				t.Fatalf("unexpected stored resource: %+v", resource)
			}
		})
	}
}

func TestBatchVideoDocumentUploadValidatesServerSideExtensionAndMIME(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared string
		data     []byte
	}{
		{name: "document.pdf", declared: "application/pdf", data: []byte("%PDF-1.7\nexample")},
		{name: "document.md", declared: "text/markdown", data: []byte("<html><script>alert(1)</script></html>")},
		{name: "document.txt", declared: "image/png", data: mediaTestPNG(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db := newMediaRecoveryTestService(t)
			header := multipartFileHeader(t, tc.name, tc.declared, tc.data)
			resource, err := s.UploadBatchVideoResource("user-1", header, "file", BatchVideoPurposeDocument, 0, 0, 0)
			if err == nil || resource != nil {
				t.Fatalf("invalid document accepted: resource=%+v err=%v", resource, err)
			}
			var count int64
			if err := db.Model(&model.Resource{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("rejected document persisted %d resources", count)
			}
		})
	}
}
