package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

func newBatchVideoMaterialService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := newSQLiteTestDB(t)
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(db), t.TempDir())
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	return svc, db
}

func createBatchVideoMaterialResources(t *testing.T, svc *Service, ids ...string) []BatchVideoMaterialInput {
	t.Helper()
	materials := make([]BatchVideoMaterialInput, 0, len(ids))
	for _, id := range ids {
		resource := &model.Resource{
			ID: id, UserID: "user", Kind: "image", Status: model.ResourceStatusReady,
			Provider: "local", ObjectKey: "users/user/image/" + id + ".png",
		}
		if err := svc.repo.CreateResource(resource); err != nil {
			t.Fatal(err)
		}
		materials = append(materials, BatchVideoMaterialInput{ResourceID: id, Kind: "image", Title: id})
	}
	return materials
}

func createBatchVideoMaterialProject(t *testing.T, svc *Service, name string, materials []BatchVideoMaterialInput) *BatchVideoProjectDetail {
	t.Helper()
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:        name,
		ProductName: name,
		Settings:    BatchVideoProjectSettings{StandardCreativeCount: 5},
		Materials:   materials,
	})
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func resourceExists(t *testing.T, svc *Service, resourceID string) bool {
	t.Helper()
	if _, err := svc.repo.ResourceForUser("user", resourceID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return false
		}
		t.Fatal(err)
	}
	return true
}

func TestCancelBatchVideoAnalysisBeforeWorkerStarts(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "queued-analysis", materials)
	task := &model.Task{
		ID: "queued-analysis-task", UserID: "user", ProjectID: detail.Project.ID,
		Type: "text", Status: model.TaskStatusQueued, Stage: "排队中",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateBatchVideoProject("user", detail.Project.ID, map[string]any{
		"analysis_status": "queued", "analysis_task_id": task.ID, "stage": "analyzing", "status": "running",
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := svc.CancelBatchVideoAnalysis(context.Background(), "user", detail.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Project.AnalysisStatus != "cancelled" || updated.Project.Stage != "draft" {
		t.Fatalf("project after queued cancel = %#v", updated.Project)
	}
	var stored model.Task
	if err := db.First(&stored, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.TaskStatusCancelled || stored.CancellationSource != model.TaskCancellationParentCancelled {
		t.Fatalf("task after queued cancel = %#v", stored)
	}
	if len(updated.MaterialBatches) != 1 {
		t.Fatal("queued analysis cancel removed materials")
	}
}

func TestBatchVideoSubmittedJobsWaitForStartOrSchedule(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	now := time.Now()
	project := &model.BatchVideoProject{ID: "start-project", UserID: "user", Name: "start", Stage: "standard_validating", Status: "submitted"}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	jobs := []model.BatchVideoGenerationJob{
		{ID: "start-high", ProjectID: project.ID, CreativeID: "creative-1", UserID: "user", ModelID: "high", Status: "submitted", ModelPriority: 90, CreatedAt: now},
		{ID: "start-low", ProjectID: project.ID, CreativeID: "creative-2", UserID: "user", ModelID: "low", Status: "submitted", ModelPriority: 10, CreatedAt: now.Add(time.Second)},
	}
	if err := svc.repo.CreateBatchVideoJobs(jobs); err != nil {
		t.Fatal(err)
	}
	if count, err := svc.StartBatchVideoGeneration("user", project.ID, nil); err != nil || count != 2 {
		t.Fatalf("start count=%d err=%v", count, err)
	}
	stored, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || stored.Status != "running" {
		t.Fatalf("project after start = %#v err=%v", stored, err)
	}
	waiting, err := svc.repo.WaitingBatchVideoJobs(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting) != 2 || waiting[0].ID != "start-high" || waiting[1].ID != "start-low" {
		t.Fatalf("waiting jobs order = %#v, want high priority first", waiting)
	}

	scheduled := &model.BatchVideoProject{ID: "schedule-project", UserID: "user", Name: "schedule", Stage: "standard_validating", Status: "submitted"}
	if err := svc.repo.CreateBatchVideoProject(scheduled); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "schedule-job", ProjectID: scheduled.ID, CreativeID: "creative-3", UserID: "user", Status: "submitted", ModelPriority: 50}}); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if count, err := svc.StartBatchVideoGeneration("user", scheduled.ID, &future); err != nil || count != 1 {
		t.Fatalf("schedule count=%d err=%v", count, err)
	}
	stored, err = svc.repo.BatchVideoProjectForUser("user", scheduled.ID)
	if err != nil || stored.ScheduledStartAt == nil || stored.Status != "submitted" {
		t.Fatalf("project after schedule = %#v err=%v", stored, err)
	}
	if err := svc.releaseScheduledBatchVideoGeneration(); err != nil {
		t.Fatal(err)
	}
	job, err := svc.repo.BatchVideoJobForUser("user", "schedule-job")
	if err != nil || job.Status != "submitted" {
		t.Fatalf("future scheduled job = %#v err=%v", job, err)
	}
	past := time.Now().Add(-time.Minute).UTC()
	if err := svc.repo.UpdateBatchVideoProject("user", scheduled.ID, map[string]any{"scheduled_start_at": past}); err != nil {
		t.Fatal(err)
	}
	if err := svc.releaseScheduledBatchVideoGeneration(); err != nil {
		t.Fatal(err)
	}
	job, err = svc.repo.BatchVideoJobForUser("user", "schedule-job")
	if err != nil || job.Status != "waiting" {
		t.Fatalf("due scheduled job = %#v err=%v", job, err)
	}
	stored, err = svc.repo.BatchVideoProjectForUser("user", scheduled.ID)
	if err != nil || stored.Status != "running" || stored.ScheduledStartAt != nil {
		t.Fatalf("project after due release = %#v err=%v", stored, err)
	}
}

func TestBatchVideoPausedProjectDoesNotStartOrReleaseSchedule(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	project := &model.BatchVideoProject{ID: "paused-project", UserID: "user", Name: "paused", Stage: "standard_validating", Status: "paused"}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "paused-job", ProjectID: project.ID, CreativeID: "creative-1", UserID: "user", Status: "submitted"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartBatchVideoGeneration("user", project.ID, nil); err == nil || !strings.Contains(err.Error(), "暂停") {
		t.Fatalf("start paused project error = %v", err)
	}
	past := time.Now().Add(-time.Minute).UTC()
	if err := svc.repo.UpdateBatchVideoProject("user", project.ID, map[string]any{"scheduled_start_at": past}); err != nil {
		t.Fatal(err)
	}
	if err := svc.releaseScheduledBatchVideoGeneration(); err != nil {
		t.Fatal(err)
	}
	job, err := svc.repo.BatchVideoJobForUser("user", "paused-job")
	if err != nil || job.Status != "submitted" {
		t.Fatalf("paused job after due = %#v err=%v", job, err)
	}
	stored, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || stored.ScheduledStartAt == nil {
		t.Fatalf("paused project should keep schedule = %#v err=%v", stored, err)
	}
	if err := svc.ResumeBatchVideoProject("user", project.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.releaseScheduledBatchVideoGeneration(); err != nil {
		t.Fatal(err)
	}
	job, err = svc.repo.BatchVideoJobForUser("user", "paused-job")
	if err != nil || job.Status != "waiting" {
		t.Fatalf("resumed job after due = %#v err=%v", job, err)
	}
}

func TestBatchVideoScheduledReleaseWithoutSubmittedJobsDoesNotMarkRunning(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	project := &model.BatchVideoProject{ID: "empty-schedule-project", UserID: "user", Name: "empty", Stage: "standard_validating", Status: "ready"}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateBatchVideoProject("user", project.ID, map[string]any{"scheduled_start_at": time.Now().Add(-time.Minute).UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := svc.releaseScheduledBatchVideoGeneration(); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || stored.Status != "ready" || stored.ScheduledStartAt != nil {
		t.Fatalf("project after empty scheduled release = %#v err=%v", stored, err)
	}
}

func TestDiscardBatchVideoUploadsKeepsReferencedResources(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	createBatchVideoMaterialResources(t, svc, "discard-orphan", "discard-referenced")
	createBatchVideoMaterialProject(t, svc, "discard-referenced", []BatchVideoMaterialInput{{ResourceID: "discard-referenced", Kind: "image", Title: "kept"}})

	count, err := svc.DiscardBatchVideoUploads("user", []string{"discard-orphan", "discard-referenced", "discard-orphan"})
	if err != nil || count != 2 {
		t.Fatalf("discard count=%d err=%v", count, err)
	}
	if resourceExists(t, svc, "discard-orphan") {
		t.Fatal("unreferenced upload was not discarded")
	}
	if !resourceExists(t, svc, "discard-referenced") {
		t.Fatal("referenced material upload was discarded")
	}
	if _, err := svc.DiscardBatchVideoUploads("other-user", []string{"discard-referenced"}); err != nil {
		t.Fatal(err)
	}
	if !resourceExists(t, svc, "discard-referenced") {
		t.Fatal("cross-user discard removed another user's resource")
	}
}

func TestBatchVideoSubmittedJobsBlockMaterialChanges(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "submitted-img")
	detail := createBatchVideoMaterialProject(t, svc, "submitted-material", materials)
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "submitted-job", ProjectID: detail.Project.ID, CreativeID: "creative-1", UserID: "user", Status: "submitted"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteBatchVideoMaterial("user", detail.Project.ID, detail.MaterialBatches[0].ID, "submitted-img"); err == nil || !strings.Contains(err.Error(), "已提交") {
		t.Fatalf("delete material with submitted jobs error = %v", err)
	}
}

func TestCancelPendingBatchVideoJobsIncludesSubmitted(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	project := &model.BatchVideoProject{ID: "cancel-project", UserID: "user", Name: "cancel", Stage: "standard_validating", Status: "submitted"}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{
		{ID: "cancel-submitted", ProjectID: project.ID, CreativeID: "creative-1", UserID: "user", Status: "submitted"},
		{ID: "cancel-waiting", ProjectID: project.ID, CreativeID: "creative-2", UserID: "user", Status: "waiting"},
	}); err != nil {
		t.Fatal(err)
	}
	count, err := svc.CancelPendingBatchVideoJobs("user", project.ID)
	if err != nil || count != 2 {
		t.Fatalf("cancel count=%d err=%v", count, err)
	}
	for _, id := range []string{"cancel-submitted", "cancel-waiting"} {
		job, err := svc.repo.BatchVideoJobForUser("user", id)
		if err != nil || job.Status != "cancelled" {
			t.Fatalf("job %s after cancel = %#v err=%v", id, job, err)
		}
	}
	stored, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || stored.Status != "ready" {
		t.Fatalf("project after cancel = %#v err=%v", stored, err)
	}
}

func TestListBatchVideoJobsIncludesTaskProgress(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "job-progress", materials)
	task := &model.Task{
		ID: "running-video-task", UserID: "user", ProjectID: detail.Project.ID,
		Type: "canvas_video", Status: model.TaskStatusRunning, Stage: "上游生成中", Progress: 61,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	job := &model.BatchVideoGenerationJob{
		ID: "progress-job", ProjectID: detail.Project.ID, UserID: "user",
		Status: "running", CurrentTaskID: task.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{*job}); err != nil {
		t.Fatal(err)
	}
	jobs, err := svc.ListBatchVideoJobs("user", detail.Project.ID, "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Progress != 61 || jobs[0].Stage != "上游生成中" {
		t.Fatalf("job progress view = %#v", jobs)
	}
}

func TestDeleteBatchVideoProjectIgnoresFinishedTaskReferences(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "history-task", materials)
	task := &model.Task{
		ID: "finished-analysis", UserID: "user", ProjectID: detail.Project.ID,
		Type: "canvas_text", Status: model.TaskStatusSucceeded, Stage: "完成",
		InputJSON: `{"referenceImages":[{"storageKey":"resource:img-1"}],"metadata":{"batchVideoProjectId":"` + detail.Project.ID + `"}}`,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteBatchVideoProject("user", detail.Project.ID); err != nil {
		t.Fatal(err)
	}
	if resourceExists(t, svc, "img-1") {
		t.Fatal("finished task history kept the deleted project's material")
	}
	var stored model.Task
	if err := db.First(&stored, "id = ?", task.ID).Error; err != nil {
		t.Fatal("finished task history should be retained for billing")
	}
}

func TestDeleteBatchVideoProjectBlocksRunningAnalysisTaskEvenIfStatusStale(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "stale-analysis", materials)
	task := &model.Task{
		ID: "stale-analysis-task", UserID: "user", ProjectID: detail.Project.ID,
		Type: "canvas_text", Status: model.TaskStatusRunning, Stage: "分析中",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateBatchVideoProject("user", detail.Project.ID, map[string]any{
		"analysis_status": "failed", "analysis_task_id": task.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBatchVideoProject("user", detail.Project.ID); err == nil || !strings.Contains(err.Error(), "进行中") {
		t.Fatalf("DeleteBatchVideoProject() error = %v, want running task rejection", err)
	}
	if _, err := svc.repo.BatchVideoProjectForUser("user", detail.Project.ID); err != nil {
		t.Fatal("project disappeared after rejected delete")
	}
}

func TestDeleteBatchVideoProjectKeepsResourceReferencedByFinishedGenericTask(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "generic-history", materials)
	task := &model.Task{
		ID: "finished-canvas-task", UserID: "user",
		Type: "canvas_image", Status: model.TaskStatusSucceeded, Stage: "完成",
		InputJSON: `{"referenceImages":[{"storageKey":"resource:img-1"}]}`,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteBatchVideoProject("user", detail.Project.ID); err != nil {
		t.Fatal(err)
	}
	if !resourceExists(t, svc, "img-1") {
		t.Fatal("resource that is the only reference of a finished generic task was deleted")
	}
}

func TestDeleteBatchVideoProjectKeepsResourceUsedByRunningTask(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "running-task", materials)
	task := &model.Task{
		ID: "running-canvas-task", UserID: "user",
		Type: "canvas_text", Status: model.TaskStatusRunning, Stage: "生成中",
		InputJSON: `{"referenceImages":[{"storageKey":"resource:img-1"}]}`,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteBatchVideoProject("user", detail.Project.ID); err != nil {
		t.Fatal(err)
	}
	if !resourceExists(t, svc, "img-1") {
		t.Fatal("resource used by a running task was deleted")
	}
}

func TestDeleteBatchVideoProjectBlocksReferenceGeneration(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "generating-reference", materials)
	asset := &model.BatchVideoReferenceAsset{
		ID: "generating-asset", ProjectID: detail.Project.ID, UserID: "user",
		ResourceID: "generating-resource", Title: "补充参考图", TagsJSON: "[]",
		Status: "generating", Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := svc.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteBatchVideoProject("user", detail.Project.ID); err == nil || !strings.Contains(err.Error(), "参考图生成") {
		t.Fatalf("DeleteBatchVideoProject() error = %v, want reference generation rejection", err)
	}
	if _, err := svc.repo.BatchVideoProjectForUser("user", detail.Project.ID); err != nil {
		t.Fatal("project disappeared after rejected delete")
	}
}

func TestDeleteBatchVideoMaterialRemovesOnlyDetachedResource(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1", "img-2")
	detail := createBatchVideoMaterialProject(t, svc, "delete-one", materials)
	batchID := detail.MaterialBatches[0].ID

	updated, err := svc.DeleteBatchVideoMaterial("user", detail.Project.ID, batchID, "img-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.MaterialBatches) != 1 || len(updated.MaterialBatches[0].Materials) != 1 || updated.MaterialBatches[0].Materials[0].ResourceID != "img-2" {
		t.Fatalf("materials after delete = %#v", updated.MaterialBatches)
	}
	if resourceExists(t, svc, "img-1") {
		t.Fatal("deleted material resource row still exists")
	}
	if !resourceExists(t, svc, "img-2") {
		t.Fatal("unrelated material resource was deleted")
	}
	if len(updated.ReferenceAssets) != 1 || updated.ReferenceAssets[0].ResourceID != "img-2" {
		t.Fatalf("reference assets after delete = %#v", updated.ReferenceAssets)
	}
	if updated.Project.AnalysisStatus != "stale" {
		t.Fatalf("analysis status = %q, want stale", updated.Project.AnalysisStatus)
	}
	var job model.ResourceDeletionJob
	if err := db.Where("resource_id = ?", "img-1").First(&job).Error; err != nil {
		t.Fatalf("physical deletion job was not enqueued: %v", err)
	}
	if job.ObjectKey != "users/user/image/img-1.png" || job.Provider != "local" {
		t.Fatalf("deletion job did not freeze storage location: %#v", job)
	}
}

func TestDeleteBatchVideoMaterialKeepsResourceUsedByAnotherProject(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "shared-image")
	first := createBatchVideoMaterialProject(t, svc, "first", materials)
	second := createBatchVideoMaterialProject(t, svc, "second", materials)

	if err := svc.DeleteBatchVideoProject("user", first.Project.ID); err != nil {
		t.Fatal(err)
	}
	if !resourceExists(t, svc, "shared-image") {
		t.Fatal("resource still used by another project was deleted")
	}

	if err := svc.DeleteBatchVideoProject("user", second.Project.ID); err != nil {
		t.Fatal(err)
	}
	if resourceExists(t, svc, "shared-image") {
		t.Fatal("resource row remained after the last project was deleted")
	}
}

func TestClearBatchVideoMaterialsRemovesBatchesAssetsAndResources(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1", "img-2")
	detail := createBatchVideoMaterialProject(t, svc, "clear-all", materials)
	before := detail.Project.ReferencePoolVersion

	updated, err := svc.ClearBatchVideoMaterials("user", detail.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.MaterialBatches) != 0 || len(updated.ReferenceAssets) != 0 {
		t.Fatalf("batches/assets after clear = %d/%d", len(updated.MaterialBatches), len(updated.ReferenceAssets))
	}
	for _, id := range []string{"img-1", "img-2"} {
		if resourceExists(t, svc, id) {
			t.Fatalf("resource %s survived clear", id)
		}
	}
	if updated.Project.AnalysisStatus != "stale" {
		t.Fatalf("analysis status = %q, want stale", updated.Project.AnalysisStatus)
	}
	if updated.Project.ReferencePoolVersion != before+1 {
		t.Fatalf("reference pool version = %d, want %d", updated.Project.ReferencePoolVersion, before+1)
	}
}

func TestBatchVideoMaterialChangesBlockedByStaleRunningAnalysisTask(t *testing.T) {
	svc, db := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "stale-material-analysis", materials)
	task := &model.Task{
		ID: "stale-material-task", UserID: "user", ProjectID: detail.Project.ID,
		Type: "canvas_text", Status: model.TaskStatusRunning, Stage: "分析中",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.UpdateBatchVideoProject("user", detail.Project.ID, map[string]any{
		"analysis_status": "failed", "analysis_task_id": task.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteBatchVideoMaterial("user", detail.Project.ID, detail.MaterialBatches[0].ID, "img-1"); err == nil || !strings.Contains(err.Error(), "分析进行中") {
		t.Fatalf("delete error = %v, want running analysis rejection", err)
	}
}

func TestBatchVideoMaterialChangesBlockedWhileAnalysisRuns(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "busy", materials)
	if err := svc.repo.UpdateBatchVideoProject("user", detail.Project.ID, map[string]any{"analysis_status": "running"}); err != nil {
		t.Fatal(err)
	}
	batchID := detail.MaterialBatches[0].ID
	if _, err := svc.DeleteBatchVideoMaterial("user", detail.Project.ID, batchID, "img-1"); err == nil || !strings.Contains(err.Error(), "分析进行中") {
		t.Fatalf("delete during analysis error = %v", err)
	}
	if _, err := svc.ClearBatchVideoMaterials("user", detail.Project.ID); err == nil || !strings.Contains(err.Error(), "分析进行中") {
		t.Fatalf("clear during analysis error = %v", err)
	}
	if !resourceExists(t, svc, "img-1") {
		t.Fatal("blocked clear removed the resource")
	}
}

func TestDeleteBatchVideoMaterialBatchKeepsSharedResource(t *testing.T) {
	svc, _ := newBatchVideoMaterialService(t)
	materials := createBatchVideoMaterialResources(t, svc, "img-1")
	detail := createBatchVideoMaterialProject(t, svc, "two-batches", materials)
	if _, err := svc.AddBatchVideoMaterialBatch("user", detail.Project.ID, "second", materials, nil); err != nil {
		t.Fatal(err)
	}
	batchID := detail.MaterialBatches[0].ID
	if _, err := svc.DeleteBatchVideoMaterialBatch("user", detail.Project.ID, batchID); err != nil {
		t.Fatal(err)
	}
	if !resourceExists(t, svc, "img-1") {
		t.Fatal("resource used by the remaining batch was deleted")
	}
	batches, err := svc.repo.BatchVideoMaterialBatches(detail.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("batches after delete = %d, want 1", len(batches))
	}
	if _, err := svc.ClearBatchVideoMaterials("user", detail.Project.ID); err != nil {
		t.Fatal(err)
	}
	if resourceExists(t, svc, "img-1") {
		t.Fatal("resource remained after the last batch was cleared")
	}
}
