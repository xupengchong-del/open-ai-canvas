package app

import (
	"encoding/json"
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
