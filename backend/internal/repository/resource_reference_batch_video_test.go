package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
)

func TestResourceReferenceSnapshotChunksTasksAndLogsAcrossResourceBatches(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "chunked-reference.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	resourceIDs := make([]string, 0, 250)
	for index := range 250 {
		resourceIDs = append(resourceIDs, fmt.Sprintf("resource-%03d", index))
	}
	tasks := []model.Task{
		{ID: "task-first", UserID: "user-1", InputJSON: `{"storageKey":"resource:resource-001"}`, Status: model.TaskStatusRunning},
		{ID: "task-second", UserID: "user-1", ResultJSON: `{"url":"/api/resources/resource-150/file"}`, Status: model.TaskStatusSucceeded},
		{ID: "task-cross", UserID: "user-1", InputJSON: `{"resourceId":"resource-050"}`, ResultJSON: `{"resourceId":"resource-240"}`, Status: model.TaskStatusSucceeded},
		{ID: "task-unrelated", UserID: "user-1", InputJSON: `{"resourceId":"resource-999"}`, Status: model.TaskStatusRunning},
		{ID: "task-other-user", UserID: "user-2", InputJSON: `{"resourceId":"resource-001"}`, Status: model.TaskStatusRunning},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	logs := []model.TaskLog{
		{ID: "log-first", UserID: "user-1", TaskID: "task-first", Payload: `{"message":"resource-001"}`},
		{ID: "log-third", UserID: "user-1", TaskID: "task-second", Payload: `{"message":"resource-240"}`},
		{ID: "log-unrelated", UserID: "user-1", TaskID: "task-unrelated", Payload: `{"message":"resource-999"}`},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	foundTasks, err := New(db).referenceTasks("user-1", resourceIDs)
	if err != nil {
		t.Fatal(err)
	}
	taskIDs := make([]string, 0, len(foundTasks))
	for _, task := range foundTasks {
		taskIDs = append(taskIDs, task.ID)
	}
	if strings.Join(taskIDs, ",") != "task-cross,task-first,task-second" {
		t.Fatalf("chunked tasks = %v, want three matching tasks once each", taskIDs)
	}
	foundLogs, err := New(db).referenceTaskLogs("user-1", resourceIDs)
	if err != nil {
		t.Fatal(err)
	}
	logIDs := make([]string, 0, len(foundLogs))
	for _, log := range foundLogs {
		logIDs = append(logIDs, log.ID)
	}
	if strings.Join(logIDs, ",") != "log-first,log-third" {
		t.Fatalf("chunked logs = %v, want two matching logs once each", logIDs)
	}
}

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
