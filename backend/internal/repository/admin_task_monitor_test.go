package repository

import (
	"testing"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAdminTaskMonitorTestDB(t *testing.T) *Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	return &Repository{db: db}
}

func seedAdminTaskMonitor(t *testing.T, repo *Repository) time.Time {
	t.Helper()
	now := time.Now().UTC()
	users := []model.User{
		{ID: "u1", Username: "alice", DisplayName: "Alice", Role: model.UserRoleUser, Status: model.UserStatusActive},
		{ID: "u2", Username: "bob", DisplayName: "Bob", Role: model.UserRoleUser, Status: model.UserStatusActive},
	}
	for index := range users {
		if err := repo.db.Create(&users[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	tasks := []model.Task{
		{ID: "t1", UserID: "u1", Type: "canvas_video", Status: model.TaskStatusSucceeded, ResultJSON: `{"video":{"url":"/api/resources/r1/file"}}`, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "t2", UserID: "u1", Type: "canvas_video", Status: model.TaskStatusFailed, Error: "upstream 503", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "t3", UserID: "u2", Type: "canvas_image", Status: model.TaskStatusSucceeded, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "t4", UserID: "u2", Type: "canvas_image", Status: model.TaskStatusRunning, CreatedAt: now.Add(-30 * time.Minute)},
		{ID: "t5", UserID: "u2", Type: "canvas_text", Status: model.TaskStatusQueued, CreatedAt: now.Add(-10 * time.Minute)},
	}
	for index := range tasks {
		if err := repo.db.Create(&tasks[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	return now
}

func adminTaskRange(now time.Time) (time.Time, time.Time) {
	return now.Add(-24 * time.Hour), now.Add(time.Hour)
}

func TestAdminTasksReturnsCountsAndUserNames(t *testing.T) {
	repo := newAdminTaskMonitorTestDB(t)
	now := seedAdminTaskMonitor(t, repo)
	from, to := adminTaskRange(now)

	rows, total, err := repo.AdminTasks(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: from, To: to}, Page: 1, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(rows) != 5 {
		t.Fatalf("expected 5 tasks, got total=%d rows=%d", total, len(rows))
	}
	byID := map[string]AdminTaskRow{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if byID["t1"].UserUsername != "alice" || byID["t1"].UserDisplayName != "Alice" {
		t.Fatalf("task t1 should carry joined user identity, got %+v", byID["t1"])
	}
	// 成功视频有结果正文，可播放；失败任务没有正文，不应显示播放入口。
	if byID["t1"].ResultJSONLength == 0 {
		t.Fatal("t1 should expose a non-zero result length without returning the body")
	}
	if byID["t2"].ResultJSONLength != 0 {
		t.Fatal("t2 has no result body")
	}

	counts, err := repo.AdminTaskCounts(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: from, To: to}})
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 5 || counts.Succeeded != 2 || counts.Failed != 1 || counts.Running != 1 || counts.Queued != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestAdminTasksFiltersByStatusCapabilityAndKeyword(t *testing.T) {
	repo := newAdminTaskMonitorTestDB(t)
	now := seedAdminTaskMonitor(t, repo)
	from, to := adminTaskRange(now)

	videoRows, _, err := repo.AdminTasks(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: from, To: to, Capability: "video"}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(videoRows) != 2 {
		t.Fatalf("capability video should match 2 tasks, got %d", len(videoRows))
	}

	failedRows, _, err := repo.AdminTasks(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: from, To: to}, Status: []string{string(model.TaskStatusFailed)}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(failedRows) != 1 || failedRows[0].ID != "t2" {
		t.Fatalf("status filter failed should return t2, got %+v", failedRows)
	}

	keywordRows, _, err := repo.AdminTasks(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: from, To: to}, Keyword: "bob", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(keywordRows) != 3 {
		t.Fatalf("keyword bob should match bob's 3 tasks, got %d", len(keywordRows))
	}

	// 时间窗之外的任务不应计入。
	outsideRows, total, err := repo.AdminTasks(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: now.Add(-2 * time.Hour), To: now.Add(-90 * time.Minute)}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(outsideRows) != 2 {
		t.Fatalf("narrow window should return 2 tasks, got total=%d rows=%d", total, len(outsideRows))
	}
}

func TestAdminTaskUserStatsCountsSuccessAndFailurePerUser(t *testing.T) {
	repo := newAdminTaskMonitorTestDB(t)
	now := seedAdminTaskMonitor(t, repo)
	from, to := adminTaskRange(now)

	stats, err := repo.AdminTaskUserStats(AdminTaskFilter{AnalyticsFilter: AnalyticsFilter{From: from, To: to}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 users, got %d", len(stats))
	}
	byUser := map[string]AdminTaskUserStat{}
	for _, stat := range stats {
		byUser[stat.UserID] = stat
	}
	if byUser["u1"].Succeeded != 1 || byUser["u1"].Failed != 1 || byUser["u1"].Total != 2 {
		t.Fatalf("alice stats wrong: %+v", byUser["u1"])
	}
	if byUser["u2"].Succeeded != 1 || byUser["u2"].Failed != 0 || byUser["u2"].Total != 3 {
		t.Fatalf("bob stats wrong: %+v", byUser["u2"])
	}
	if byUser["u1"].Username != "alice" || byUser["u1"].DisplayName != "Alice" {
		t.Fatalf("user stats should carry identity, got %+v", byUser["u1"])
	}
}

func TestAdminTaskByIDReturnsNilWhenMissing(t *testing.T) {
	repo := newAdminTaskMonitorTestDB(t)
	seedAdminTaskMonitor(t, repo)

	row, err := repo.AdminTaskByID("missing")
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Fatalf("expected nil row for unknown task, got %+v", row)
	}

	found, err := repo.AdminTaskByID("t1")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.ID != "t1" || found.UserUsername != "alice" {
		t.Fatalf("unexpected row: %+v", found)
	}
}
