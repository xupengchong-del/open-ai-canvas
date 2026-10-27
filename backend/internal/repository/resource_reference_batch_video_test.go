package repository

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
)

func TestResourceReferenceSnapshotIncludesOwnedBatchVideoReferences(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "resource-reference.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	project := model.BatchVideoProject{ID: "batch-project", UserID: "user-1", Name: "批量视频项目"}
	reference := model.BatchVideoReferenceAsset{ID: "batch-reference", ProjectID: project.ID, UserID: "user-1", ResourceID: "resource-1", Title: "产品主视觉"}
	otherUserReference := model.BatchVideoReferenceAsset{ID: "other-reference", ProjectID: "other-project", UserID: "user-2", ResourceID: "resource-1", Title: "其他用户"}
	for _, item := range []any{&project, &reference, &otherUserReference} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := New(db).ResourceReferenceSnapshot("user-1", "", []string{"resource-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Direct) != 1 {
		t.Fatalf("direct references = %+v, want only owned batch video reference", snapshot.Direct)
	}
	got := snapshot.Direct[0]
	if got.Kind != "批量视频参考素材" || got.ID != reference.ID || got.Title != reference.Title || got.ResourceID != reference.ResourceID {
		t.Fatalf("direct reference = %+v", got)
	}
}
