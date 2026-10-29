package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newBatchVideoConsistencyService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+newID()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.Resource{},
		&model.SystemSetting{},
		&model.ChannelModel{},
		&model.ModelChannel{},
		&model.Task{},
		&model.TaskLog{},
		&model.Result{},
		&model.TaskTextDelta{},
		&model.ApiCallLog{},
		&model.Asset{},
		&model.CanvasProject{},
		&model.BatchVideoProject{},
		&model.BatchVideoMaterialBatch{},
		&model.BatchVideoSellingPoint{},
		&model.BatchVideoReferenceAsset{},
		&model.BatchVideoCreativeReference{},
		&model.BatchVideoCreativeRound{},
		&model.BatchVideoCreative{},
		&model.BatchVideoGenerationJob{},
		&model.BatchVideoAttempt{},
		&model.BatchVideoStandardValidation{},
	); err != nil {
		t.Fatal(err)
	}
	return &Service{repo: repository.New(db)}
}

func saveBatchVideoConsistencySettings(t *testing.T, svc *Service, settings BatchVideoSettings) {
	t.Helper()
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: batchVideoSettingsKey, ValueJSON: string(encoded)}); err != nil {
		t.Fatal(err)
	}
}

func createBatchVideoStandardFixture(t *testing.T, svc *Service) (*model.BatchVideoProject, *model.BatchVideoCreativeRound, []model.BatchVideoCreative) {
	t.Helper()
	settings := defaultBatchVideoSettings()
	settings.StandardCreativeCount = 1
	settings.StandardMinSuccess = 1
	saveBatchVideoConsistencySettings(t, svc, settings)
	projectSettingsJSON := batchVideoMustJSON(BatchVideoProjectSettings{StandardCreativeCount: 2, DurationSeconds: 8, AspectRatio: "9:16"})
	project := &model.BatchVideoProject{ID: "project", UserID: "user", Name: "Product", Stage: "standard_validating", Status: "ready", SettingsJSON: projectSettingsJSON, CurrentProfileVersion: 3, ReferencePoolVersion: 7}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	batch := &model.BatchVideoMaterialBatch{ID: "batch-1", ProjectID: project.ID, UserID: project.UserID, BatchNo: 1, Status: "ready"}
	if err := svc.repo.CreateBatchVideoMaterialBatch(batch); err != nil {
		t.Fatal(err)
	}
	round := &model.BatchVideoCreativeRound{ID: "round-standard", ProjectID: project.ID, UserID: project.UserID, RoundNo: 1, Mode: "standard", RequestedCount: 2, TemplateSnapshotJSON: `["pain_point","comparison"]`, MaterialBatchIDsJSON: `["batch-1"]`, ProfileVersion: 3, Status: "reviewing"}
	if err := svc.repo.CreateBatchVideoCreativeRound(round); err != nil {
		t.Fatal(err)
	}
	creatives := []model.BatchVideoCreative{
		{ID: "creative-1", ProjectID: project.ID, RoundID: round.ID, UserID: project.UserID, Seq: 1, TemplateID: "pain_point", ReviewStatus: "approved", FinalStatus: "approved", MatchedReferenceAssetID: "asset-1"},
		{ID: "creative-2", ProjectID: project.ID, RoundID: round.ID, UserID: project.UserID, Seq: 2, TemplateID: "comparison", ReviewStatus: "approved", FinalStatus: "approved", MatchedReferenceAssetID: "asset-1"},
	}
	if err := svc.repo.CreateBatchVideoCreatives(creatives); err != nil {
		t.Fatal(err)
	}
	jobs := []model.BatchVideoGenerationJob{
		{ID: "job-1", ProjectID: project.ID, CreativeID: creatives[0].ID, UserID: project.UserID, ModelID: "model-a", Status: "succeeded"},
		{ID: "job-2", ProjectID: project.ID, CreativeID: creatives[1].ID, UserID: project.UserID, ModelID: "model-b", Status: "failed"},
		{ID: "job-mass", ProjectID: project.ID, CreativeID: "mass-creative", UserID: project.UserID, ModelID: "model-mass", Status: "succeeded"},
	}
	if err := svc.repo.CreateBatchVideoJobs(jobs); err != nil {
		t.Fatal(err)
	}
	return project, round, creatives
}

func TestBatchVideoValidationRequiresEveryStandardCreativeAndUsesProjectCount(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project, _, creatives := createBatchVideoStandardFixture(t, svc)

	state, err := svc.batchVideoValidationState(project.UserID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Passed {
		t.Fatal("validation passed before every standard creative succeeded")
	}
	if state.StandardCreativeCount != 2 {
		t.Fatalf("standard creative count = %d, want 2", state.StandardCreativeCount)
	}
	if len(state.ModelIDs) != 1 || state.ModelIDs[0] != "model-a" {
		t.Fatalf("validated models = %#v, want only model-a", state.ModelIDs)
	}
	if len(state.TemplateIDs) != 1 || state.TemplateIDs[0] != "pain_point" {
		t.Fatalf("validated templates = %#v, want only pain_point", state.TemplateIDs)
	}
	for _, check := range state.Checks {
		if check.Key == "creative_count" && !check.Passed {
			t.Fatal("project-level standard count was replaced by the global threshold")
		}
		if check.Key == "success" && check.Required != 2 {
			t.Fatalf("success requirement = %#v, want every standard creative", check.Required)
		}
	}

	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "job-3", ProjectID: project.ID, CreativeID: creatives[1].ID, UserID: project.UserID, ModelID: "model-b", Status: "succeeded"}}); err != nil {
		t.Fatal(err)
	}
	state, err = svc.batchVideoValidationState(project.UserID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Passed {
		t.Fatalf("validation should pass after all standard creatives succeed: %#v", state.Checks)
	}
	if len(state.ModelIDs) != 2 || len(state.TemplateIDs) != 2 {
		t.Fatalf("validated snapshots are incomplete: models=%#v templates=%#v", state.ModelIDs, state.TemplateIDs)
	}
}

func TestBatchVideoUnlockStoresRealValidationSnapshot(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project, round, creatives := createBatchVideoStandardFixture(t, svc)
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "job-3", ProjectID: project.ID, CreativeID: creatives[1].ID, UserID: project.UserID, ModelID: "model-b", Status: "succeeded"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UnlockBatchVideoMass(project.UserID, project.ID); err != nil {
		t.Fatal(err)
	}
	validation, err := svc.repo.LatestBatchVideoStandardValidation(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if validation.StandardRoundID != round.ID || validation.ReferencePoolVersion != 7 || validation.ProfileVersion != 3 {
		t.Fatalf("unexpected validation snapshot: %#v", validation)
	}
	if validation.MaterialBatchIDsJSON != `["batch-1"]` {
		t.Fatalf("material snapshot = %s, want real batch ids", validation.MaterialBatchIDsJSON)
	}
}

func TestBatchVideoMassSnapshotRejectsModelTemplateAndVersionDrift(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project, _, creatives := createBatchVideoStandardFixture(t, svc)
	now := time.Now()
	project.MassUnlockedAt = &now
	if err := svc.repo.SaveBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	validation := &model.BatchVideoStandardValidation{
		ID: "validation", ProjectID: project.ID, UserID: project.UserID, Status: "passed",
		ValidatedTemplateIDsJSON: `["pain_point","comparison"]`, ValidatedModelIDsJSON: `["model-a","model-b"]`,
		ReferencePoolVersion: 7, MaterialBatchIDsJSON: `["batch-1"]`, ProfileVersion: 3,
	}
	if err := svc.repo.CreateBatchVideoStandardValidation(validation); err != nil {
		t.Fatal(err)
	}
	models := []BatchVideoSubmitModel{{ModelID: "model-a", ChannelModelID: "model-a"}}
	if err := svc.validateBatchVideoMassSubmission(project, creatives, models); err != nil {
		t.Fatalf("matching snapshot rejected: %v", err)
	}
	if err := svc.validateBatchVideoMassSubmission(project, creatives, []BatchVideoSubmitModel{{ModelID: "model-new", ChannelModelID: "model-new"}}); err == nil || !strings.Contains(err.Error(), "模型") {
		t.Fatalf("unvalidated model error = %v", err)
	}
	project.CurrentProfileVersion = 4
	if err := svc.validateBatchVideoMassSubmission(project, creatives, models); err == nil || !strings.Contains(err.Error(), "产品档案") {
		t.Fatalf("profile drift error = %v", err)
	}
}

func TestBatchVideoReviewRequiresRejectReasonAndFilteredTotal(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project, _, creatives := createBatchVideoStandardFixture(t, svc)
	if _, err := svc.ReviewBatchVideoCreative(project.UserID, project.ID, creatives[0].ID, "rejected", "  "); err == nil {
		t.Fatal("rejection without a reason was accepted")
	}
	updated, err := svc.ReviewBatchVideoCreatives(project.UserID, project.ID, []string{creatives[0].ID, creatives[1].ID}, "rejected", "文案不符合产品")
	if err != nil {
		t.Fatal(err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	items, total, err := svc.ListBatchVideoCreatives(project.UserID, project.ID, "", "rejected", "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || total != 2 {
		t.Fatalf("filtered page len=%d total=%d, want 1 and 2", len(items), total)
	}
	for _, creative := range items {
		if creative.FinalStatus != "rejected" {
			t.Fatalf("final status = %q, want rejected", creative.FinalStatus)
		}
	}
}

func TestBatchVideoDeleteProjectCascadesOwnedRecords(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:        "delete-project",
		ProductName: "delete product",
		Settings:    BatchVideoProjectSettings{StandardCreativeCount: 5},
		Documents:   []BatchVideoDocumentInput{{Name: "卖点", Text: "续航 14 天"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projectID := detail.Project.ID
	round := &model.BatchVideoCreativeRound{ID: "delete-round", ProjectID: projectID, UserID: "user", RoundNo: 1, Mode: "standard", RequestedCount: 1, Status: "reviewing"}
	if err := svc.repo.CreateBatchVideoCreativeRound(round); err != nil {
		t.Fatal(err)
	}
	creative := model.BatchVideoCreative{ID: "delete-creative", ProjectID: projectID, RoundID: round.ID, UserID: "user", Seq: 1, Title: "title", ReviewStatus: "pending", FinalStatus: "pending"}
	if err := svc.repo.CreateBatchVideoCreatives([]model.BatchVideoCreative{creative}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoCreativeReferences([]model.BatchVideoCreativeReference{{ID: "delete-ref", CreativeID: creative.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "delete-job", ProjectID: projectID, CreativeID: creative.ID, UserID: "user", Status: "failed"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoAttempt(&model.BatchVideoAttempt{ID: "delete-attempt", ProjectID: projectID, CreativeID: creative.ID, JobID: "delete-job", UserID: "user", AttemptNo: 0, Status: "failed"}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteBatchVideoProject("user", projectID); err != nil {
		t.Fatalf("DeleteBatchVideoProject() error = %v", err)
	}
	if _, err := svc.repo.BatchVideoProjectForUser("user", projectID); err == nil {
		t.Fatal("project still exists after delete")
	}
	if batches, _ := svc.repo.BatchVideoMaterialBatches(projectID); len(batches) != 0 {
		t.Fatalf("material batches = %d after delete", len(batches))
	}
	if points, _ := svc.repo.BatchVideoSellingPoints(projectID); len(points) != 0 {
		t.Fatalf("selling points = %d after delete", len(points))
	}
	if rounds, _ := svc.repo.BatchVideoCreativeRounds(projectID); len(rounds) != 0 {
		t.Fatalf("rounds = %d after delete", len(rounds))
	}
	if creatives, _ := svc.repo.BatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: projectID}); len(creatives) != 0 {
		t.Fatalf("creatives = %d after delete", len(creatives))
	}
	if refs, _ := svc.repo.BatchVideoCreativeReferences([]string{creative.ID}); len(refs) != 0 {
		t.Fatalf("creative references = %d after delete", len(refs))
	}
	if jobs, _ := svc.repo.BatchVideoJobs(projectID, "", 0, 0); len(jobs) != 0 {
		t.Fatalf("jobs = %d after delete", len(jobs))
	}
	if attempts, _ := svc.repo.BatchVideoAttempts("delete-job"); len(attempts) != 0 {
		t.Fatalf("attempts = %d after delete", len(attempts))
	}
}

func TestSubmitBatchVideoGenerationWaitsForStartEndToEnd(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: "feature_availability", ValueJSON: `{"creditsEnabled":false}`}); err != nil {
		t.Fatal(err)
	}
	channelModel := batchVideoPriorityTestModel(t, "submit-model", "submit-video", 80)
	if err := svc.repo.SaveChannelModel(&channelModel); err != nil {
		t.Fatal(err)
	}
	resource := &model.Resource{ID: "submit-resource", UserID: "user", Kind: "image", Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "users/user/image/submit.png"}
	if err := svc.repo.CreateResource(resource); err != nil {
		t.Fatal(err)
	}
	project := &model.BatchVideoProject{ID: "submit-project", UserID: "user", Name: "submit", Stage: "standard_validating", Status: "ready", SettingsJSON: batchVideoMustJSON(BatchVideoProjectSettings{StandardCreativeCount: 5, DurationSeconds: 8, AspectRatio: "9:16"})}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	asset := &model.BatchVideoReferenceAsset{ID: "submit-asset", ProjectID: project.ID, UserID: "user", ResourceID: resource.ID, Title: "主图", Status: "approved", Version: 1}
	if err := svc.repo.SaveBatchVideoReferenceAsset(asset); err != nil {
		t.Fatal(err)
	}
	creative := model.BatchVideoCreative{ID: "submit-creative", ProjectID: project.ID, UserID: "user", Seq: 1, Title: "创意", VideoPrompt: "展示产品", DurationSeconds: 8, AspectRatio: "9:16", MatchedReferenceAssetID: asset.ID, MatchStatus: "matched", ReviewStatus: "approved", FinalStatus: "approved"}
	if err := svc.repo.CreateBatchVideoCreatives([]model.BatchVideoCreative{creative}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoCreativeReferences([]model.BatchVideoCreativeReference{{ID: "submit-ref", CreativeID: creative.ID, ReferenceAssetID: asset.ID, AssetVersion: 1, Role: "reference"}}); err != nil {
		t.Fatal(err)
	}

	count, err := svc.SubmitBatchVideoGeneration("user", project.ID, BatchVideoSubmitInput{
		CreativeIDs: []string{creative.ID}, GenerationMode: "standard", Mode: "single",
		Models: []BatchVideoSubmitModel{{ChannelModelID: channelModel.ID}},
	})
	if err != nil || count != 1 {
		t.Fatalf("submit count=%d err=%v", count, err)
	}
	storedProject, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || storedProject.Status != "submitted" {
		t.Fatalf("project after submit = %#v err=%v", storedProject, err)
	}
	jobs, err := svc.repo.BatchVideoJobs(project.ID, "", 0, 0)
	if err != nil || len(jobs) != 1 || jobs[0].Status != "submitted" || jobs[0].ModelPriority != 80 {
		t.Fatalf("jobs after submit = %#v err=%v", jobs, err)
	}
	if err := svc.ProcessBatchVideoQueue(context.Background()); err != nil {
		t.Fatal(err)
	}
	jobs, err = svc.repo.BatchVideoJobs(project.ID, "", 0, 0)
	if err != nil || len(jobs) != 1 || jobs[0].Status != "submitted" {
		t.Fatalf("submitted job was dispatched before start: %#v err=%v", jobs, err)
	}
	if _, err := svc.StartBatchVideoGeneration("user", project.ID, nil); err != nil {
		t.Fatal(err)
	}
	jobs, err = svc.repo.BatchVideoJobs(project.ID, "", 0, 0)
	if err != nil || len(jobs) != 1 || jobs[0].Status != "waiting" {
		t.Fatalf("jobs after start = %#v err=%v", jobs, err)
	}
}

func TestBatchVideoDeleteProjectAllowsSubmittedJobs(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	project := &model.BatchVideoProject{ID: "submitted-delete-project", UserID: "user", Name: "submitted-delete", Stage: "standard_validating", Status: "submitted"}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "submitted-delete-job", ProjectID: project.ID, CreativeID: "creative-1", UserID: "user", Status: "submitted"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBatchVideoProject("user", project.ID); err != nil {
		t.Fatalf("delete project with only submitted jobs = %v", err)
	}
	if jobs, err := svc.repo.BatchVideoJobs(project.ID, "", 0, 0); err != nil || len(jobs) != 0 {
		t.Fatalf("jobs after delete = %#v err=%v", jobs, err)
	}
}

func TestBatchVideoDeleteProjectBlocksActiveWork(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{Name: "active-project", Settings: BatchVideoProjectSettings{StandardCreativeCount: 5}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "active-job", ProjectID: detail.Project.ID, UserID: "user", Status: "waiting"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBatchVideoProject("user", detail.Project.ID); err == nil || !strings.Contains(err.Error(), "进行中的") {
		t.Fatalf("DeleteBatchVideoProject() error = %v, want active-work rejection", err)
	}
	if _, err := svc.repo.BatchVideoProjectForUser("user", detail.Project.ID); err != nil {
		t.Fatalf("project disappeared after rejected delete: %v", err)
	}
}

func TestBatchVideoDeleteProjectRejectsForeignUser(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{Name: "foreign-project", Settings: BatchVideoProjectSettings{StandardCreativeCount: 5}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteBatchVideoProject("other-user", detail.Project.ID); err == nil {
		t.Fatal("foreign user deleted the project")
	}
	if _, err := svc.repo.BatchVideoProjectForUser("user", detail.Project.ID); err != nil {
		t.Fatalf("project missing after foreign delete attempt: %v", err)
	}
}

func TestBatchVideoCreateAcceptsNameOnlyProject(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:     "name-only",
		Settings: BatchVideoProjectSettings{StandardCreativeCount: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Project.ProductName != "" {
		t.Fatalf("product name = %q, want empty", detail.Project.ProductName)
	}
	if len(detail.MaterialBatches) != 0 {
		t.Fatalf("material batches = %d, want none for a name-only project", len(detail.MaterialBatches))
	}
	if len(detail.SellingPoints) != 0 {
		t.Fatalf("selling points = %d, want none for a name-only project", len(detail.SellingPoints))
	}
	if detail.Project.Name != "name-only" {
		t.Fatalf("project name = %q", detail.Project.Name)
	}
}

func TestBatchVideoCreateAcceptsDocumentOnlyProject(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:        "document-only",
		ProductName: "document product",
		Settings:    BatchVideoProjectSettings{StandardCreativeCount: 5},
		Documents:   []BatchVideoDocumentInput{{Name: "手动卖点", Text: "续航 14 天"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.MaterialBatches) != 1 || len(detail.MaterialBatches[0].Documents) != 1 {
		t.Fatalf("document batch was not persisted: %#v", detail.MaterialBatches)
	}
	if _, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name: "empty-document", ProductName: "document product",
		Settings:  BatchVideoProjectSettings{StandardCreativeCount: 5},
		Documents: []BatchVideoDocumentInput{{Name: "empty", Text: "  "}},
	}); err == nil {
		t.Fatal("blank document was accepted")
	}
}

func TestBatchVideoCreateRejectsForeignOrPendingMaterialBeforePersisting(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	for _, resource := range []model.Resource{
		{ID: "foreign", UserID: "other", Kind: "image", Status: model.ResourceStatusReady},
		{ID: "pending", UserID: "user", Kind: "image", Status: model.ResourceStatusPending},
	} {
		if err := svc.repo.CreateResource(&resource); err != nil {
			t.Fatal(err)
		}
		_, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
			Name:        "project-" + resource.ID,
			ProductName: "product-" + resource.ID,
			Settings:    BatchVideoProjectSettings{StandardCreativeCount: 5},
			Materials:   []BatchVideoMaterialInput{{ResourceID: resource.ID, Kind: "image"}},
		})
		if err == nil {
			t.Fatalf("resource %s was accepted", resource.ID)
		}
	}
	projects, err := svc.repo.BatchVideoProjects("user", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("invalid material persisted %d projects", len(projects))
	}
}

func createBatchVideoAnalysisModels(t *testing.T, svc *Service, models ...model.ChannelModel) {
	t.Helper()
	for index := range models {
		if err := svc.repo.SaveChannelModel(&models[index]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBatchVideoAnalysisAutoSelectsHighestPriorityVisionModel(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	createBatchVideoAnalysisModels(t, svc,
		model.ChannelModel{ID: "text-plain", ModelKey: "plain-text", Capability: "text", Protocol: "openai", Enabled: true, SortOrder: 1, CapabilityConfigJSON: `{"version":1,"priority":100,"text":{"references":{"promptMaxChars":100000}}}`},
		model.ChannelModel{ID: "text-vision-low", ModelKey: "vision-low", Capability: "text", Protocol: "openai", Enabled: true, SortOrder: 2, CapabilityConfigJSON: `{"version":1,"priority":60,"text":{"references":{"promptMaxChars":100000,"maxImages":2}}}`},
		model.ChannelModel{ID: "text-vision", ModelKey: "vision-text", Capability: "text", Protocol: "openai", Enabled: true, SortOrder: 3, CapabilityConfigJSON: `{"version":1,"priority":51,"text":{"references":{"promptMaxChars":100000,"maxImages":4}}}`},
	)
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:     "auto-model",
		Settings: BatchVideoProjectSettings{StandardCreativeCount: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 旧项目可能残留固定的策划模型；新逻辑必须忽略它，按后台优先级重新挑选。
	stale := `{"durationSeconds":8,"aspectRatio":"9:16","standardCreativeCount":5,"planningChannelModelId":"text-plain"}`
	if err := svc.repo.UpdateBatchVideoProject("user", detail.Project.ID, map[string]any{"settings_json": stale}); err != nil {
		t.Fatal(err)
	}
	selected, capability, err := svc.autoBatchVideoPlanningModel(true)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "text-vision-low" || capability.Text == nil || capability.Text.References.MaxImages != 2 {
		t.Fatalf("auto-selected model = %s, want highest-priority vision model text-vision-low", selected.ID)
	}
	updated, err := svc.repo.BatchVideoProjectForUser("user", detail.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SettingsJSON != stale {
		t.Fatalf("settings rewritten after auto selection: %s", updated.SettingsJSON)
	}
	// 后台新增更高优先级模型后，下一次任务必须立即改用新模型。
	createBatchVideoAnalysisModels(t, svc,
		model.ChannelModel{ID: "text-vision-new", ModelKey: "vision-new", Capability: "text", Protocol: "openai", Enabled: true, SortOrder: 9, CapabilityConfigJSON: `{"version":1,"priority":200,"text":{"references":{"promptMaxChars":100000,"maxImages":6}}}`},
	)
	selected, _, err = svc.autoBatchVideoPlanningModel(true)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "text-vision-new" {
		t.Fatalf("newly added model not picked, selected %s", selected.ID)
	}
}

func TestBatchVideoAnalysisAutoSelectionRequiresVisionWhenImages(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	createBatchVideoAnalysisModels(t, svc,
		model.ChannelModel{ID: "text-plain", ModelKey: "plain-text", Capability: "text", Protocol: "openai", Enabled: true, CapabilityConfigJSON: `{"version":1,"text":{"references":{"promptMaxChars":100000}}}`},
	)
	if _, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:     "vision-required",
		Settings: BatchVideoProjectSettings{StandardCreativeCount: 5},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.autoBatchVideoPlanningModel(true); err == nil || !strings.Contains(err.Error(), "视觉") {
		t.Fatalf("image analysis without a vision model error = %v, want vision requirement", err)
	}
	selected, _, err := svc.autoBatchVideoPlanningModel(false)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "text-plain" {
		t.Fatalf("text-only analysis selected %s, want text-plain", selected.ID)
	}
	settings := projectSettings(&model.BatchVideoProject{SettingsJSON: `{"durationSeconds":8,"aspectRatio":"9:16","standardCreativeCount":5,"planningChannelModelId":"text-plain"}`})
	encoded := batchVideoMustJSON(settings)
	if strings.Contains(encoded, "planningChannelModelId") || settings.StandardCreativeCount != 5 {
		t.Fatalf("settings still expose a fixed planning model: %s", encoded)
	}
}

func batchVideoPriorityTestModel(t *testing.T, id, modelKey string, priority int) model.ChannelModel {
	t.Helper()
	profile := DefaultModelCapabilityConfigForModel("newapi-channel-2", modelKey).Video
	profile.BatchGeneration = VideoBatchGenerationConfig{Enabled: true, MaxItems: 20, Priority: priority}
	normalized, err := NormalizeModelCapabilityConfig("video", "newapi-channel-2", &ModelCapabilityConfig{Version: 1, Video: profile})
	if err != nil {
		t.Fatal(err)
	}
	return model.ChannelModel{
		ID: id, ChannelID: "channel", ModelKey: modelKey, Capability: "video",
		Protocol: "newapi-channel-2", Enabled: true, CapabilityConfigJSON: batchVideoMustJSON(normalized),
	}
}

func TestBatchVideoModelsOrderByPriorityDescending(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	for _, item := range []model.ChannelModel{
		batchVideoPriorityTestModel(t, "model-low", "low-video", 10),
		batchVideoPriorityTestModel(t, "model-high", "high-video", 90),
		batchVideoPriorityTestModel(t, "model-default", "default-video", 0),
	} {
		if err := svc.repo.SaveChannelModel(&item); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := svc.prepareBatchVideoModels([]BatchVideoSubmitModel{
		{ChannelModelID: "model-low"}, {ChannelModelID: "model-high"}, {ChannelModelID: "model-default"},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 3 {
		t.Fatalf("prepared models = %d, want 3", len(prepared))
	}
	want := []struct {
		id       string
		priority int
	}{{"model-high", 90}, {"model-default", 50}, {"model-low", 10}}
	for index, item := range want {
		if prepared[index].ModelID != item.id || prepared[index].Priority != item.priority {
			t.Fatalf("prepared[%d] = %s/%d, want %s/%d", index, prepared[index].ModelID, prepared[index].Priority, item.id, item.priority)
		}
	}
	options, err := svc.ListBatchVideoModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 3 || options[0].ChannelModelID != "model-high" || options[0].Priority != 90 {
		t.Fatalf("model options = %#v, want high priority first", options)
	}
	job := newBatchVideoJob(&model.BatchVideoProject{ID: "project", UserID: "user"}, model.BatchVideoCreative{ID: "creative"}, "single", BatchVideoTaskTemplate{
		Candidates: []BatchVideoTaskCandidate{{ModelID: "model-high", Priority: 90}},
	})
	if job.Status != "submitted" || job.ModelPriority != 90 {
		t.Fatalf("new job = %#v, want submitted with priority 90", job)
	}
}

func TestSortBatchVideoMaterialsByNumericImageTitle(t *testing.T) {
	materials := []BatchVideoMaterialInput{
		{ResourceID: "img-10", Kind: "image", Title: "10"},
		{ResourceID: "img-2", Kind: "image", Title: "2"},
		{ResourceID: "video-1", Kind: "video", Title: "1"},
		{ResourceID: "cover", Kind: "image", Title: "封面"},
		{ResourceID: "img-1", Kind: "image", Title: "01"},
	}
	sortBatchVideoMaterials(materials)
	if materials[0].Title != "01" || materials[1].Title != "2" || materials[4].Title != "10" {
		t.Fatalf("numeric image order = %q, %q, %q, want 01, 2, 10", materials[0].Title, materials[1].Title, materials[4].Title)
	}
	if materials[2].Kind != "video" || materials[3].Title != "封面" {
		t.Fatalf("non-numeric order changed: %#v", materials)
	}
	unchanged := []BatchVideoMaterialInput{{Kind: "image", Title: "b"}, {Kind: "image", Title: "a"}}
	sortBatchVideoMaterials(unchanged)
	if unchanged[0].Title != "b" || unchanged[1].Title != "a" {
		t.Fatalf("non-numeric titles were reordered: %#v", unchanged)
	}
}

func TestAddBatchVideoMaterialBatchDefaultsLabelAndSortsNumericTitles(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:     "batch-label",
		Settings: BatchVideoProjectSettings{StandardCreativeCount: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"img-10", "img-2"} {
		if err := svc.repo.CreateResource(&model.Resource{ID: id, UserID: "user", Kind: "image", Status: model.ResourceStatusReady}); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := svc.AddBatchVideoMaterialBatch("user", detail.Project.ID, "  ", []BatchVideoMaterialInput{
		{ResourceID: "img-10", Kind: "image", Title: "10"},
		{ResourceID: "img-2", Kind: "image", Title: "2"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(batch.Label, time.Now().Format("2006-01-02")) || !strings.Contains(batch.Label, "素材批次 1") {
		t.Fatalf("default batch label = %q, want today's date and batch number", batch.Label)
	}
	if len(batch.Materials) != 2 || batch.Materials[0].Title != "2" || batch.Materials[1].Title != "10" {
		t.Fatalf("stored material order = %#v, want numeric ascending", batch.Materials)
	}
}

func TestBatchVideoMaterialsDedupeRepeatedResource(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	if err := svc.repo.CreateResource(&model.Resource{ID: "img-1", UserID: "user", Kind: "image", Status: model.ResourceStatusReady}); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:     "dedupe-materials",
		Settings: BatchVideoProjectSettings{StandardCreativeCount: 5},
		Materials: []BatchVideoMaterialInput{
			{ResourceID: "img-1", Kind: "image", Title: "1"},
			{ResourceID: "img-1", Kind: "image", Title: "1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.MaterialBatches) != 1 || len(detail.MaterialBatches[0].Materials) != 1 {
		t.Fatalf("duplicate materials were persisted: %#v", detail.MaterialBatches)
	}
	if len(detail.ReferenceAssets) != 1 {
		t.Fatalf("duplicate reference assets = %d, want 1", len(detail.ReferenceAssets))
	}
	batch, err := svc.AddBatchVideoMaterialBatch("user", detail.Project.ID, "dup", []BatchVideoMaterialInput{
		{ResourceID: "img-1", Kind: "image", Title: "1"},
		{ResourceID: "img-1", Kind: "image", Title: "1"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Materials) != 1 {
		t.Fatalf("duplicate materials in added batch = %d, want 1", len(batch.Materials))
	}
}

func TestBatchVideoApplyAnalysisWritesProfilePointsAndImageDescriptions(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project := &model.BatchVideoProject{
		ID: "analysis-project", UserID: "user", Name: "watch",
		Stage: "analyzing", Status: "running", AnalysisStatus: "running",
		SettingsJSON: batchVideoMustJSON(BatchVideoProjectSettings{StandardCreativeCount: 5}), CurrentProfileVersion: 1, ReferencePoolVersion: 1,
	}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	asset := &model.BatchVideoReferenceAsset{
		ID: "asset-image", ProjectID: project.ID, UserID: project.UserID,
		ResourceID: "resource-image", Title: "正面图", TagsJSON: "[]", Status: "approved", Version: 1,
	}
	if err := svc.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
		t.Fatal(err)
	}
	result := `{"productSummary":"轻薄智能手表，续航 14 天。","sellingPoints":["续航 14 天","支持血氧监测"],"imageDescriptions":[{"resourceId":"resource-image","description":"白底正面手表图","tags":["白底","正面"]}],"complianceNotes":["避免使用医疗诊断表述"]}`
	task := &model.Task{ID: "analysis-task", ResultJSON: batchVideoMustJSON(map[string]any{"text": "```json\n" + result + "\n```"})}
	if err := svc.applyBatchVideoAnalysis(project, task); err != nil {
		t.Fatal(err)
	}
	points, err := svc.repo.BatchVideoSellingPoints(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("selling points = %d, want 2", len(points))
	}
	assets, err := svc.repo.BatchVideoReferenceAssets(project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Description != "白底正面手表图" || !strings.Contains(assets[0].TagsJSON, "白底") {
		t.Fatalf("image description was not applied: %#v", assets)
	}
	updated, err := svc.repo.BatchVideoProjectForUser(project.UserID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AnalysisStatus != "succeeded" || updated.Stage != "draft" || updated.CurrentProfileVersion != 2 {
		t.Fatalf("project after analysis = %#v", updated)
	}
	if !strings.Contains(updated.ProductProfileJSON, "续航 14 天") || !strings.Contains(updated.ProductProfileJSON, "避免使用医疗诊断表述") {
		t.Fatalf("product profile missing model output: %s", updated.ProductProfileJSON)
	}
}

func TestBatchVideoAnalysisRunsImagesInParallelAndStoresEachResult(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: "feature_availability", ValueJSON: `{"creditsEnabled":false}`}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Create(&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Name: "test", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	channelModel := model.ChannelModel{
		ID: "analysis-vision", ChannelID: "channel", ModelKey: "vision-text", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, Enabled: true, SortOrder: 1,
		BillingMode: "fixed_request", UnitPriceMicrocredits: 1, PriceConfigured: true, PriceVersion: 1,
		CapabilityConfigJSON: `{"version":1,"text":{"references":{"promptMaxChars":100000,"maxImages":4}}}`,
	}
	if err := svc.repo.SaveChannelModel(&channelModel); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"analysis-image-1", "analysis-image-2"} {
		if err := svc.repo.CreateResource(&model.Resource{ID: id, UserID: "user", Kind: "image", Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "users/user/image/" + id + ".png"}); err != nil {
			t.Fatal(err)
		}
	}
	project := &model.BatchVideoProject{
		ID: "per-image-project", UserID: "user", Name: "并行分析",
		Stage: "draft", Status: "ready", CurrentProfileVersion: 1, ReferencePoolVersion: 1,
		SettingsJSON: batchVideoMustJSON(BatchVideoProjectSettings{StandardCreativeCount: 5}),
	}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	for index, resourceID := range []string{"analysis-image-1", "analysis-image-2"} {
		asset := &model.BatchVideoReferenceAsset{
			ID:        newID(),
			ProjectID: project.ID, UserID: "user", ResourceID: resourceID,
			Title:    []string{"正面图", "侧面图"}[index],
			TagsJSON: "[]", Source: "upload", Status: "approved", Version: 1,
		}
		if err := svc.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
			t.Fatal(err)
		}
	}
	materials := []BatchVideoMaterialInput{
		{ResourceID: "analysis-image-1", Kind: "image", Title: "正面图"},
		{ResourceID: "analysis-image-2", Kind: "image", Title: "侧面图"},
	}
	started, err := svc.startBatchVideoAnalysis("user", project, materials, nil, batchVideoAnalysisScopeAll)
	if err != nil {
		t.Fatal(err)
	}
	if started.AnalysisStatus != "queued" {
		t.Fatalf("project after start = %#v", started)
	}
	assets, err := svc.repo.BatchVideoReferenceAssets(project.ID, "")
	if err != nil || len(assets) != 2 {
		t.Fatalf("assets = %#v err=%v", assets, err)
	}
	tasks := map[string]*model.Task{}
	for index := range assets {
		if assets[index].AnalysisStatus != "queued" || assets[index].AnalysisTaskID == "" {
			t.Fatalf("asset %s was not queued in parallel: %#v", assets[index].Title, assets[index])
		}
		task, err := svc.repo.TaskForUser("user", assets[index].AnalysisTaskID)
		if err != nil {
			t.Fatal(err)
		}
		var input struct {
			ReferenceImages []map[string]any `json:"referenceImages"`
		}
		if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
			t.Fatal(err)
		}
		if len(input.ReferenceImages) != 1 {
			t.Fatalf("analysis request for %s has %d images, want exactly 1", assets[index].Title, len(input.ReferenceImages))
		}
		tasks[assets[index].ID] = task
	}
	if len(tasks) != 2 {
		t.Fatalf("parallel analysis created %d tasks, want 2", len(tasks))
	}
	// 第一张完成时第二张仍在运行，项目保持分析中。
	firstTask := tasks[assets[0].ID]
	firstTask.Status = model.TaskStatusSucceeded
	firstTask.ResultJSON = batchVideoMustJSON(map[string]any{"text": `{"description":"正面白底图","tags":["白底","正面"],"sellingPoints":["续航 14 天"],"productSummary":"智能手表","complianceNotes":["避免医疗表述"]}`})
	if err := svc.repo.SaveTaskCompletion(firstTask, model.TaskStatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.reconcileBatchVideoAnalysis(); err != nil {
		t.Fatal(err)
	}
	firstAsset, err := svc.repo.BatchVideoReferenceAssetForUser("user", assets[0].ID)
	if err != nil || firstAsset.AnalysisStatus != "succeeded" || firstAsset.Description != "正面白底图" || !strings.Contains(firstAsset.AnalysisResultJSON, "续航 14 天") {
		t.Fatalf("first asset analysis = %#v err=%v", firstAsset, err)
	}
	mid, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mid.AnalysisStatus != "running" {
		t.Fatalf("project after first parallel result = %#v, want running", mid)
	}
	secondAsset, err := svc.repo.BatchVideoReferenceAssetForUser("user", assets[1].ID)
	if err != nil || secondAsset.AnalysisTaskID != tasks[assets[1].ID].ID || secondAsset.AnalysisStatus != "queued" {
		t.Fatalf("second asset should stay in flight: %#v err=%v", secondAsset, err)
	}
	secondTask := tasks[assets[1].ID]
	secondTask.Status = model.TaskStatusSucceeded
	secondTask.ResultJSON = batchVideoMustJSON(map[string]any{"text": `{"description":"侧面场景图","tags":["侧面","场景"],"sellingPoints":["续航 14 天","支持血氧监测"],"productSummary":"轻薄智能手表","complianceNotes":[]}`})
	if err := svc.repo.SaveTaskCompletion(secondTask, model.TaskStatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.reconcileBatchVideoAnalysis(); err != nil {
		t.Fatal(err)
	}
	finished, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.AnalysisStatus != "succeeded" || finished.AnalysisCurrentAssetID != "" || finished.Stage != "draft" {
		t.Fatalf("project after parallel analysis = %#v", finished)
	}
	assets, err = svc.repo.BatchVideoReferenceAssets(project.ID, "")
	if err != nil || len(assets) != 2 {
		t.Fatalf("assets = %#v err=%v", assets, err)
	}
	for _, asset := range assets {
		if asset.AnalysisStatus != "succeeded" || asset.AnalyzedAt == nil || asset.AnalysisResultJSON == "" {
			t.Fatalf("asset result was not persisted: %#v", asset)
		}
	}
	points, err := svc.repo.BatchVideoSellingPoints(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("selling points = %d, want deduplicated 2: %#v", len(points), points)
	}
	if !strings.Contains(finished.ProductProfileJSON, "轻薄智能手表") || !strings.Contains(finished.ProductProfileJSON, "避免医疗表述") {
		t.Fatalf("profile did not merge image results: %s", finished.ProductProfileJSON)
	}
}

func TestBatchVideoIncrementalAnalysisSkipsSucceededImages(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: "feature_availability", ValueJSON: `{"creditsEnabled":false}`}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Create(&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Name: "test", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveChannelModel(&model.ChannelModel{
		ID: "analysis-vision", ChannelID: "channel", ModelKey: "vision-text", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, Enabled: true, SortOrder: 1,
		BillingMode: "fixed_request", UnitPriceMicrocredits: 1, PriceConfigured: true, PriceVersion: 1,
		CapabilityConfigJSON: `{"version":1,"text":{"references":{"promptMaxChars":100000,"maxImages":4}}}`,
	}); err != nil {
		t.Fatal(err)
	}
	resources := []string{"incr-image-1", "incr-image-2", "incr-image-3"}
	for _, id := range resources {
		if err := svc.repo.CreateResource(&model.Resource{ID: id, UserID: "user", Kind: "image", Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "users/user/image/" + id + ".png"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CreateBatchVideoProject("user", CreateBatchVideoProjectInput{
		Name:      "incremental",
		Settings:  BatchVideoProjectSettings{StandardCreativeCount: 5},
		Materials: []BatchVideoMaterialInput{{ResourceID: resources[0], Kind: "image", Title: "01"}, {ResourceID: resources[1], Kind: "image", Title: "02"}, {ResourceID: resources[2], Kind: "image", Title: "03"}},
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.ListBatchVideoProjects("user", "", 1, 10)
	if err != nil || len(detail) != 1 {
		t.Fatalf("projects = %#v err=%v", detail, err)
	}
	projectID := detail[0].Project.ID
	assets, err := svc.repo.BatchVideoReferenceAssets(projectID, "")
	if err != nil || len(assets) != 3 {
		t.Fatalf("assets = %#v err=%v", assets, err)
	}
	// 第一张已成功、第二张失败、第三张待分析。
	assets[0].AnalysisStatus = "succeeded"
	assets[0].Description = "已分析的正面图"
	assets[0].AnalysisResultJSON = `{"description":"已分析的正面图"}`
	assets[0].AnalysisTaskID = "done-task"
	assets[1].AnalysisStatus = "failed"
	assets[1].AnalysisError = "上游 HTTP 502"
	for index := range assets {
		if err := svc.repo.SaveBatchVideoReferenceAsset(&assets[index]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.AnalyzeBatchVideoProject("user", projectID, "unknown"); err == nil {
		t.Fatal("invalid analysis scope was accepted")
	}
	started, err := svc.AnalyzeBatchVideoProject("user", projectID, batchVideoAnalysisScopePending)
	if err != nil {
		t.Fatal(err)
	}
	if started.Project.AnalysisStatus != "queued" {
		t.Fatalf("project after incremental analysis = %#v", started.Project)
	}
	assets, err = svc.repo.BatchVideoReferenceAssets(projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	if assets[0].AnalysisStatus != "succeeded" || assets[0].Description != "已分析的正面图" || assets[0].AnalysisTaskID != "done-task" {
		t.Fatalf("incremental analysis restarted a succeeded image: %#v", assets[0])
	}
	if assets[1].AnalysisStatus != "queued" || assets[1].AnalysisTaskID == "" || assets[1].AnalysisTaskID == "done-task" {
		t.Fatalf("failed image was not retried: %#v", assets[1])
	}
	if assets[2].AnalysisStatus != "queued" || assets[2].AnalysisTaskID == "" {
		t.Fatalf("pending image was not started: %#v", assets[2])
	}
	if _, err := svc.CancelBatchVideoAnalysis(context.Background(), "user", projectID); err != nil {
		t.Fatal(err)
	}
	rewound, err := svc.AnalyzeBatchVideoProject("user", projectID, batchVideoAnalysisScopeAll)
	if err != nil {
		t.Fatal(err)
	}
	if rewound.Project.AnalysisStatus != "queued" {
		t.Fatalf("project after full re-analysis = %#v", rewound.Project)
	}
	assets, err = svc.repo.BatchVideoReferenceAssets(projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	if assets[0].AnalysisStatus != "queued" || assets[0].AnalysisTaskID == "done-task" {
		t.Fatalf("all scope did not restart the succeeded image: %#v", assets[0])
	}
}

func TestBatchVideoAnalysisFailureWaitsForDecisionAndContinues(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	settings := defaultBatchVideoSettings()
	settings.AnalysisConcurrency = 2
	saveBatchVideoConsistencySettings(t, svc, settings)
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: "feature_availability", ValueJSON: `{"creditsEnabled":false}`}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Create(&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Name: "test", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	channelModel := model.ChannelModel{
		ID: "analysis-vision", ChannelID: "channel", ModelKey: "vision-text", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, Enabled: true, SortOrder: 1,
		BillingMode: "fixed_request", UnitPriceMicrocredits: 1, PriceConfigured: true, PriceVersion: 1,
		CapabilityConfigJSON: `{"version":1,"text":{"references":{"promptMaxChars":100000,"maxImages":4}}}`,
	}
	if err := svc.repo.SaveChannelModel(&channelModel); err != nil {
		t.Fatal(err)
	}
	resources := []string{"decision-image-1", "decision-image-2", "decision-image-3"}
	for _, id := range resources {
		if err := svc.repo.CreateResource(&model.Resource{ID: id, UserID: "user", Kind: "image", Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "users/user/image/" + id + ".png"}); err != nil {
			t.Fatal(err)
		}
	}
	project := &model.BatchVideoProject{
		ID: "decision-project", UserID: "user", Name: "失败决策",
		Stage: "draft", Status: "ready", CurrentProfileVersion: 1, ReferencePoolVersion: 1,
		SettingsJSON: batchVideoMustJSON(BatchVideoProjectSettings{StandardCreativeCount: 5}),
	}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	titles := []string{"正面图", "侧面图", "细节图"}
	for index, resourceID := range resources {
		asset := &model.BatchVideoReferenceAsset{
			ID:        newID(),
			ProjectID: project.ID, UserID: "user", ResourceID: resourceID,
			Title:    titles[index],
			TagsJSON: "[]", Source: "upload", Status: "approved", Version: 1,
		}
		if err := svc.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
			t.Fatal(err)
		}
	}
	materials := []BatchVideoMaterialInput{
		{ResourceID: resources[0], Kind: "image", Title: titles[0]},
		{ResourceID: resources[1], Kind: "image", Title: titles[1]},
		{ResourceID: resources[2], Kind: "image", Title: titles[2]},
	}
	if _, err := svc.startBatchVideoAnalysis("user", project, materials, nil, batchVideoAnalysisScopeAll); err != nil {
		t.Fatal(err)
	}
	assets, err := svc.repo.BatchVideoReferenceAssets(project.ID, "")
	if err != nil || len(assets) != 3 {
		t.Fatalf("assets = %#v err=%v", assets, err)
	}
	inFlight, pending := 0, 0
	for _, asset := range assets {
		switch asset.AnalysisStatus {
		case "queued", "running":
			inFlight++
		case "pending":
			pending++
		}
	}
	if inFlight != 2 || pending != 1 {
		t.Fatalf("parallel start = %d in flight, %d pending, want 2 and 1", inFlight, pending)
	}
	firstTask, err := svc.repo.TaskForUser("user", assets[0].AnalysisTaskID)
	if err != nil {
		t.Fatal(err)
	}
	firstTask.Status = model.TaskStatusFailed
	firstTask.Error = `Post "https://d1api.xin/v1/chat/completions": net/http: TLS handshake timeout`
	if err := svc.repo.SaveTaskCompletion(firstTask, model.TaskStatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.reconcileBatchVideoAnalysis(); err != nil {
		t.Fatal(err)
	}
	paused, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.AnalysisStatus != batchVideoAnalysisAwaitingDecision || paused.AnalysisCurrentAssetID != assets[0].ID {
		t.Fatalf("project after failure = %#v, want awaiting decision on failed asset", paused)
	}
	if !strings.Contains(paused.AnalysisError, "TLS 握手超时") || !strings.Contains(paused.AnalysisError, "第 1 张") {
		t.Fatalf("analysis error = %q, want image index and TLS reason", paused.AnalysisError)
	}
	failedAsset, err := svc.repo.BatchVideoReferenceAssetForUser("user", assets[0].ID)
	if err != nil || failedAsset.AnalysisStatus != "failed" || !strings.Contains(failedAsset.AnalysisError, "TLS handshake timeout") {
		t.Fatalf("failed asset = %#v err=%v", failedAsset, err)
	}
	// 等待确认期间不补位：第二张继续跑，第三张仍待分析。
	secondTask, err := svc.repo.TaskForUser("user", assets[1].AnalysisTaskID)
	if err != nil {
		t.Fatal(err)
	}
	secondTask.Status = model.TaskStatusSucceeded
	secondTask.ResultJSON = batchVideoMustJSON(map[string]any{"text": `{"description":"侧面场景图","tags":["侧面","场景"],"sellingPoints":["容量 500ml"],"productSummary":"保温杯","complianceNotes":[]}`})
	if err := svc.repo.SaveTaskCompletion(secondTask, model.TaskStatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.reconcileBatchVideoAnalysis(); err != nil {
		t.Fatal(err)
	}
	pausedAgain, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || pausedAgain.AnalysisStatus != batchVideoAnalysisAwaitingDecision {
		t.Fatalf("project after in-flight success = %#v err=%v", pausedAgain, err)
	}
	thirdAsset, err := svc.repo.BatchVideoReferenceAssetForUser("user", assets[2].ID)
	if err != nil || thirdAsset.AnalysisStatus != "pending" || thirdAsset.AnalysisTaskID != "" {
		t.Fatalf("third asset should wait for decision: %#v err=%v", thirdAsset, err)
	}
	continued, err := svc.ContinueBatchVideoAnalysis("user", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Project.AnalysisStatus == batchVideoAnalysisAwaitingDecision {
		t.Fatalf("project after continue = %#v, want running", continued.Project)
	}
	thirdAsset, err = svc.repo.BatchVideoReferenceAssetForUser("user", assets[2].ID)
	if err != nil || thirdAsset.AnalysisTaskID == "" || thirdAsset.AnalysisStatus != "queued" {
		t.Fatalf("third asset not dispatched after continue: %#v err=%v", thirdAsset, err)
	}
	thirdTask, err := svc.repo.TaskForUser("user", thirdAsset.AnalysisTaskID)
	if err != nil {
		t.Fatal(err)
	}
	thirdTask.Status = model.TaskStatusFailed
	thirdTask.Error = "上游 HTTP 502"
	if err := svc.repo.SaveTaskCompletion(thirdTask, model.TaskStatusQueued, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.reconcileBatchVideoAnalysis(); err != nil {
		t.Fatal(err)
	}
	pausedThird, err := svc.repo.BatchVideoProjectForUser("user", project.ID)
	if err != nil || pausedThird.AnalysisStatus != batchVideoAnalysisAwaitingDecision {
		t.Fatalf("project after third failure = %#v err=%v", pausedThird, err)
	}
	if !strings.Contains(pausedThird.AnalysisError, "第 3 张") {
		t.Fatalf("third failure message = %q", pausedThird.AnalysisError)
	}
	cancelled, err := svc.CancelBatchVideoAnalysis(context.Background(), "user", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Project.AnalysisStatus != "cancelled" || cancelled.Project.AnalysisCurrentAssetID != "" {
		t.Fatalf("project after cancel = %#v, want cancelled", cancelled.Project)
	}
	thirdAsset, err = svc.repo.BatchVideoReferenceAssetForUser("user", assets[2].ID)
	if err != nil || thirdAsset.AnalysisStatus != "failed" {
		t.Fatalf("third failed asset = %#v err=%v, want failure preserved", thirdAsset, err)
	}
}

func TestBatchVideoAnalysisConcurrencySettingDefaultsAndClamps(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{name: "unset", value: 0, want: 10},
		{name: "negative", value: -3, want: 10},
		{name: "explicit", value: 7, want: 7},
		{name: "too large", value: 200, want: 50},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := normalizeBatchVideoSettings(BatchVideoSettings{AnalysisConcurrency: test.value})
			if settings.AnalysisConcurrency != test.want {
				t.Fatalf("analysis concurrency = %d, want %d", settings.AnalysisConcurrency, test.want)
			}
		})
	}
}

func TestBatchVideoAnalysisRespectsAccountActiveTaskLimit(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	saveBatchVideoConsistencySettings(t, svc, defaultBatchVideoSettings())
	if err := svc.repo.SaveSystemSetting(&model.SystemSetting{Key: "feature_availability", ValueJSON: `{"creditsEnabled":false}`}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.Create(&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Name: "test", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.SaveChannelModel(&model.ChannelModel{
		ID: "analysis-vision", ChannelID: "channel", ModelKey: "vision-text", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, Enabled: true, SortOrder: 1,
		BillingMode: "fixed_request", UnitPriceMicrocredits: 1, PriceConfigured: true, PriceVersion: 1,
		CapabilityConfigJSON: `{"version":1,"text":{"references":{"promptMaxChars":100000,"maxImages":4}}}`,
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"slot-image-1", "slot-image-2", "slot-image-3"} {
		if err := svc.repo.CreateResource(&model.Resource{ID: id, UserID: "user", Kind: "image", Status: model.ResourceStatusReady, Provider: "local", ObjectKey: "users/user/image/" + id + ".png"}); err != nil {
			t.Fatal(err)
		}
	}
	// 账号活动任务上限默认 5，先占掉 3 个，只剩 2 个可用槽位。
	for index := 0; index < 3; index++ {
		if err := svc.repo.Create(&model.Task{ID: fmt.Sprintf("busy-task-%d", index), UserID: "user", Type: "text", Status: model.TaskStatusQueued}); err != nil {
			t.Fatal(err)
		}
	}
	project := &model.BatchVideoProject{
		ID: "slot-project", UserID: "user", Name: "槽位受限",
		Stage: "draft", Status: "ready", CurrentProfileVersion: 1, ReferencePoolVersion: 1,
		SettingsJSON: batchVideoMustJSON(BatchVideoProjectSettings{StandardCreativeCount: 5}),
	}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	for index, resourceID := range []string{"slot-image-1", "slot-image-2", "slot-image-3"} {
		if err := svc.repo.CreateBatchVideoReferenceAsset(&model.BatchVideoReferenceAsset{
			ID: newID(), ProjectID: project.ID, UserID: "user", ResourceID: resourceID,
			Title: fmt.Sprintf("%02d", index+1), TagsJSON: "[]", Source: "upload", Status: "approved", Version: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	materials := []BatchVideoMaterialInput{
		{ResourceID: "slot-image-1", Kind: "image", Title: "01"},
		{ResourceID: "slot-image-2", Kind: "image", Title: "02"},
		{ResourceID: "slot-image-3", Kind: "image", Title: "03"},
	}
	started, err := svc.startBatchVideoAnalysis("user", project, materials, nil, batchVideoAnalysisScopePending)
	if err != nil {
		t.Fatal(err)
	}
	if started.AnalysisStatus == "failed" {
		t.Fatalf("analysis failed instead of waiting for slots: %#v", started)
	}
	assets, err := svc.repo.BatchVideoReferenceAssets(project.ID, "")
	if err != nil || len(assets) != 3 {
		t.Fatalf("assets = %#v err=%v", assets, err)
	}
	inFlight, waiting := 0, 0
	for _, asset := range assets {
		switch asset.AnalysisStatus {
		case "queued", "running":
			inFlight++
		case "pending":
			waiting++
		}
	}
	if inFlight != 2 || waiting != 1 {
		t.Fatalf("slot-limited start = %d in flight, %d pending, want 2 and 1", inFlight, waiting)
	}
	// 槽位占满后再次显式分析要给出可操作提示，而不是静默排队。
	for index := 3; index < 5; index++ {
		if err := svc.repo.Create(&model.Task{ID: fmt.Sprintf("busy-task-%d", index), UserID: "user", Type: "text", Status: model.TaskStatusQueued}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CancelBatchVideoAnalysis(context.Background(), "user", project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.startBatchVideoAnalysis("user", project, materials, nil, batchVideoAnalysisScopePending); err == nil || !strings.Contains(err.Error(), "账号同时最多") {
		t.Fatalf("full-slot start error = %v, want account task limit hint", err)
	}
}
