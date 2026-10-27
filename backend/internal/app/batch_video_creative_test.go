package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func createBatchVideoCreativeEditFixture(t *testing.T, svc *Service) (*model.BatchVideoProject, *model.BatchVideoCreative) {
	t.Helper()
	project := &model.BatchVideoProject{ID: "edit-project", UserID: "user", Name: "product"}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	creative := &model.BatchVideoCreative{
		ID: "edit-creative", ProjectID: project.ID, UserID: project.UserID, RoundID: "round",
		Title: "old", Hook: "old hook", Copywriting: "old copy", VideoPrompt: "old video", ImagePrompt: "old image",
		ImageRequirementJSON: `{"scene":"户外","angle":"近景","style":"写实","requiredElements":["露营灯"]}`,
		ReviewStatus:         "approved", FinalStatus: "approved",
	}
	if err := svc.repo.CreateBatchVideoCreatives([]model.BatchVideoCreative{*creative}); err != nil {
		t.Fatal(err)
	}
	return project, creative
}

func TestBatchVideoUpdateCreativeResetsReviewAndRejectsSubmittedCreative(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project, creative := createBatchVideoCreativeEditFixture(t, svc)
	updated, err := svc.UpdateBatchVideoCreativeContent(project.UserID, project.ID, creative.ID, UpdateBatchVideoCreativeInput{
		Title: "new", Hook: "new hook", Copywriting: "new copy", VideoPrompt: "new video", ImagePrompt: "new image",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "new" || updated.ReviewStatus != "pending" || updated.FinalStatus != "pending" || updated.DuplicateHash == "" {
		t.Fatalf("unexpected updated creative: %#v", updated)
	}
	if err := svc.repo.CreateBatchVideoJobs([]model.BatchVideoGenerationJob{{ID: "edit-job", ProjectID: project.ID, CreativeID: creative.ID, UserID: project.UserID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateBatchVideoCreativeContent(project.UserID, project.ID, creative.ID, UpdateBatchVideoCreativeInput{
		Title: "again", Hook: "hook", Copywriting: "copy", VideoPrompt: "video", ImagePrompt: "image",
	}); err == nil {
		t.Fatal("submitted creative was edited")
	}
	if _, err := svc.UpdateBatchVideoCreativeContent("other", project.ID, creative.ID, UpdateBatchVideoCreativeInput{}); err == nil {
		t.Fatal("foreign project was accepted")
	}
}

func TestBatchVideoRematchCreativeUsesProgramScoreAndAllowsMissing(t *testing.T) {
	svc := newBatchVideoConsistencyService(t)
	project, creative := createBatchVideoCreativeEditFixture(t, svc)
	asset := &model.BatchVideoReferenceAsset{ID: "matching-asset", ProjectID: project.ID, UserID: project.UserID, Title: "户外露营灯", TagsJSON: `["近景","写实"]`, Status: "approved"}
	if err := svc.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
		t.Fatal(err)
	}
	matched, err := svc.RematchBatchVideoCreative(project.UserID, project.ID, creative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if matched.MatchedReferenceAssetID != asset.ID || matched.MatchStatus != "matched" || matched.MatchMethod != "program" || matched.ReviewStatus != "pending" {
		t.Fatalf("unexpected match: %#v", matched)
	}
	asset.Title = "办公室"
	asset.TagsJSON = `["办公"]`
	if err := svc.repo.SaveBatchVideoReferenceAsset(asset); err != nil {
		t.Fatal(err)
	}
	missing, err := svc.RematchBatchVideoCreative(project.UserID, project.ID, creative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if missing.MatchedReferenceAssetID != "" || missing.MatchStatus != "missing" {
		t.Fatalf("unrelated asset was forced into creative: %#v", missing)
	}
}
