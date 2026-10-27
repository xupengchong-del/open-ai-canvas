package repository

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
)

func TestBatchVideoRoundsForPlanningIncludesRunningStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch-video-planning.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	rounds := []model.BatchVideoCreativeRound{
		{ID: "round-generating", ProjectID: "project-1", UserID: "user-1", TaskID: "task-1", Status: "generating"},
		{ID: "round-running", ProjectID: "project-1", UserID: "user-1", TaskID: "task-2", Status: "running"},
		{ID: "round-reviewing", ProjectID: "project-1", UserID: "user-1", TaskID: "task-3", Status: "reviewing"},
	}
	for index := range rounds {
		if err := db.Create(&rounds[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	items, err := New(db).BatchVideoRoundsForPlanning(20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Status] = true
	}
	if !seen["generating"] || !seen["running"] {
		t.Fatalf("planning rounds = %+v, want both generating and running statuses", items)
	}
	if seen["reviewing"] {
		t.Fatalf("planning rounds = %+v, reviewing round must not be reconciled", items)
	}
}
