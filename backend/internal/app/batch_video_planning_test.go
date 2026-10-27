package app

import (
	"encoding/json"
	"strings"
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestDecodeBatchVideoJSONAcceptsFencedOutput(t *testing.T) {
	text := "这是结果：\n```json\n{\"creatives\":[{\"title\":\"A\"}]}\n```\n请查收"
	var response batchVideoPlanningResponse
	if err := decodeBatchVideoJSON(text, &response); err != nil {
		t.Fatalf("decodeBatchVideoJSON() error = %v", err)
	}
	if len(response.Creatives) != 1 || response.Creatives[0].Title != "A" {
		t.Fatalf("unexpected creatives: %#v", response.Creatives)
	}
}

func TestDecodeBatchVideoJSONRejectsMissingObject(t *testing.T) {
	var response batchVideoPlanningResponse
	if err := decodeBatchVideoJSON("没有 JSON", &response); err == nil {
		t.Fatal("decodeBatchVideoJSON() expected error")
	}
}

func TestBatchVideoMatchReferenceAssetPrefersTagHits(t *testing.T) {
	assets := []model.BatchVideoReferenceAsset{
		{ID: "a", Title: "桌面开箱", Description: "白色背景", TagsJSON: `["桌面","开箱"]`},
		{ID: "b", Title: "户外运动", Description: "跑步场景", TagsJSON: `["户外","运动"]`},
	}
	matched := batchVideoMatchReferenceAsset(assets, []string{"户外", "夜景"}, 0)
	if matched == nil || matched.ID != "b" {
		t.Fatalf("batchVideoMatchReferenceAsset() = %#v, want b", matched)
	}
}

func TestBatchVideoMatchReferenceAssetReturnsNilWithoutTagHits(t *testing.T) {
	assets := []model.BatchVideoReferenceAsset{{ID: "a"}, {ID: "b"}}
	if matched := batchVideoMatchReferenceAsset(assets, nil, 3); matched != nil {
		t.Fatalf("batchVideoMatchReferenceAsset() = %#v, want nil", matched)
	}
	if matched := batchVideoMatchReferenceAsset(assets, []string{"不存在"}, 0); matched != nil {
		t.Fatalf("batchVideoMatchReferenceAsset() with low score = %#v, want nil", matched)
	}
}

func TestBatchVideoMatchSellingPointBySubstring(t *testing.T) {
	points := []model.BatchVideoSellingPoint{{ID: "p1", CanonicalText: "续航 14 天"}, {ID: "p2", CanonicalText: "IP68 防水"}}
	if id := batchVideoMatchSellingPoint(points, "主打续航 14 天，适合出差"); id != "p1" {
		t.Fatalf("batchVideoMatchSellingPoint() = %q, want p1", id)
	}
	if id := batchVideoMatchSellingPoint(points, "完全不相关"); id != "" {
		t.Fatalf("batchVideoMatchSellingPoint() = %q, want empty", id)
	}
}

func validBatchVideoPlanningItem() batchVideoPlanningCreative {
	return batchVideoPlanningCreative{
		Title:        "露营续航方案",
		SellingPoint: "续航 14 天",
		Hook:         "出门两周还要带充电器吗",
		Copywriting:  "这款露营灯一次充电可以续航十四天，短途旅行不用带充电器。",
		VideoPrompt:  "镜头从夜景推向产品，再特写电量显示。",
		ImagePrompt:  "露营桌面上的户外灯，夜景，真实产品摄影。",
		TemplateID:   "snapshot_template",
		SceneTags:    []string{"露营"},
		ImageRequirement: &batchVideoImageRequirement{
			Scene:             "露营桌面",
			Angle:             "低角度近景",
			Style:             "真实商业摄影",
			AspectRatio:       "4:5",
			RequiredElements:  []string{"户外灯"},
			ForbiddenElements: []string{"错误 logo"},
		},
		DurationSeconds: 12,
		AspectRatio:     "4:5",
	}
}

func newBatchVideoPlanningFixture(t *testing.T, count int) (*Service, *model.BatchVideoCreativeRound, []model.BatchVideoSellingPoint) {
	t.Helper()
	svc := newBatchVideoConsistencyService(t)
	project := &model.BatchVideoProject{
		ID: "planning-project", UserID: "planning-user", Name: "户外灯", ProductName: "户外灯",
		Stage: "standard_planning", Status: "running",
		SettingsJSON: batchVideoMustJSON(BatchVideoProjectSettings{DurationSeconds: 12, AspectRatio: "4:5"}),
	}
	if err := svc.repo.CreateBatchVideoProject(project); err != nil {
		t.Fatal(err)
	}
	points := []model.BatchVideoSellingPoint{{ID: "point-1", ProjectID: project.ID, UserID: project.UserID, CanonicalText: "续航 14 天", Status: "active"}}
	if err := svc.repo.CreateBatchVideoSellingPoints(points); err != nil {
		t.Fatal(err)
	}
	round := &model.BatchVideoCreativeRound{
		ID: "planning-round", ProjectID: project.ID, UserID: project.UserID, Mode: "standard",
		RequestedCount: count, TemplateSnapshotJSON: `["snapshot_template"]`, Status: "generating",
	}
	if err := svc.repo.CreateBatchVideoCreativeRound(round); err != nil {
		t.Fatal(err)
	}
	return svc, round, points
}

func batchVideoPlanningTask(t *testing.T, items []batchVideoPlanningCreative) *model.Task {
	t.Helper()
	response, err := json.Marshal(batchVideoPlanningResponse{Creatives: items})
	if err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(map[string]string{"text": string(response)})
	if err != nil {
		t.Fatal(err)
	}
	return &model.Task{ID: newID(), ResultJSON: string(result)}
}

func assertBatchVideoPlanningRejected(t *testing.T, svc *Service, round *model.BatchVideoCreativeRound, task *model.Task, contains string) {
	t.Helper()
	err := svc.applyBatchVideoPlanning(round, task)
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("applyBatchVideoPlanning() error = %v, want containing %q", err, contains)
	}
	count, countErr := svc.repo.CountBatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: round.ProjectID})
	if countErr != nil {
		t.Fatal(countErr)
	}
	if count != 0 {
		t.Fatalf("rejected batch persisted %d creatives", count)
	}
}

func TestBatchVideoApplyPlanningPersistsStructuredRequirementAndMissingMatch(t *testing.T) {
	svc, round, _ := newBatchVideoPlanningFixture(t, 1)
	asset := &model.BatchVideoReferenceAsset{ID: "unrelated", ProjectID: round.ProjectID, UserID: round.UserID, Title: "办公室", TagsJSON: `["办公"]`, Status: "approved", Version: 2}
	if err := svc.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
		t.Fatal(err)
	}
	item := validBatchVideoPlanningItem()
	if err := svc.applyBatchVideoPlanning(round, batchVideoPlanningTask(t, []batchVideoPlanningCreative{item})); err != nil {
		t.Fatal(err)
	}
	creatives, err := svc.repo.BatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: round.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	if len(creatives) != 1 {
		t.Fatalf("creatives = %d, want 1", len(creatives))
	}
	creative := creatives[0]
	if creative.SellingPointID != "point-1" || creative.TemplateID != "snapshot_template" || creative.MatchStatus != "missing" || creative.MatchedReferenceAssetID != "" {
		t.Fatalf("unexpected creative: %#v", creative)
	}
	var requirement batchVideoImageRequirement
	if err := json.Unmarshal([]byte(creative.ImageRequirementJSON), &requirement); err != nil {
		t.Fatal(err)
	}
	if requirement.Scene != "露营桌面" || requirement.Angle != "低角度近景" || requirement.Style != "真实商业摄影" || requirement.AspectRatio != "4:5" || len(requirement.RequiredElements) != 1 || len(requirement.ForbiddenElements) != 1 {
		t.Fatalf("unexpected image requirement: %#v", requirement)
	}
}

func TestBatchVideoApplyPlanningRequiresExactCount(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		items int
	}{
		{name: "too few", count: 2, items: 1},
		{name: "too many", count: 1, items: 2},
		{name: "empty", count: 1, items: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, round, _ := newBatchVideoPlanningFixture(t, test.count)
			items := make([]batchVideoPlanningCreative, test.items)
			for index := range items {
				items[index] = validBatchVideoPlanningItem()
				items[index].Title = strings.Repeat("不同", index+1)
				items[index].Hook += strings.Repeat("变体", index)
				items[index].Copywriting += strings.Repeat("变体", index)
			}
			assertBatchVideoPlanningRejected(t, svc, round, batchVideoPlanningTask(t, items), "数量必须精确等于")
		})
	}
}

func TestBatchVideoApplyPlanningRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name     string
		contains string
		mutate   func(*batchVideoPlanningCreative)
	}{
		{name: "required title", contains: "title", mutate: func(item *batchVideoPlanningCreative) { item.Title = " " }},
		{name: "required selling point", contains: "sellingPoint", mutate: func(item *batchVideoPlanningCreative) { item.SellingPoint = "" }},
		{name: "required hook", contains: "hook", mutate: func(item *batchVideoPlanningCreative) { item.Hook = "" }},
		{name: "required copywriting", contains: "copywriting", mutate: func(item *batchVideoPlanningCreative) { item.Copywriting = "" }},
		{name: "required video prompt", contains: "videoPrompt", mutate: func(item *batchVideoPlanningCreative) { item.VideoPrompt = "" }},
		{name: "required image prompt", contains: "imagePrompt", mutate: func(item *batchVideoPlanningCreative) { item.ImagePrompt = "" }},
		{name: "unknown selling point", contains: "无法匹配项目已有卖点", mutate: func(item *batchVideoPlanningCreative) { item.SellingPoint = "永久续航" }},
		{name: "template outside snapshot", contains: "模板快照", mutate: func(item *batchVideoPlanningCreative) { item.TemplateID = "pain_point" }},
		{name: "duration differs", contains: "时长必须为 12", mutate: func(item *batchVideoPlanningCreative) { item.DurationSeconds = 8 }},
		{name: "ratio differs", contains: "比例必须为 4:5", mutate: func(item *batchVideoPlanningCreative) { item.AspectRatio = "9:16" }},
		{name: "missing requirement", contains: "imageRequirement", mutate: func(item *batchVideoPlanningCreative) { item.ImageRequirement = nil }},
		{name: "requirement ratio differs", contains: "aspectRatio 必须为 4:5", mutate: func(item *batchVideoPlanningCreative) { item.ImageRequirement.AspectRatio = "9:16" }},
		{name: "required elements missing", contains: "requiredElements", mutate: func(item *batchVideoPlanningCreative) { item.ImageRequirement.RequiredElements = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, round, _ := newBatchVideoPlanningFixture(t, 1)
			item := validBatchVideoPlanningItem()
			test.mutate(&item)
			assertBatchVideoPlanningRejected(t, svc, round, batchVideoPlanningTask(t, []batchVideoPlanningCreative{item}), test.contains)
		})
	}
}

func TestBatchVideoApplyPlanningRejectsSameBatchDuplicateDespiteTitleChange(t *testing.T) {
	svc, round, _ := newBatchVideoPlanningFixture(t, 2)
	first := validBatchVideoPlanningItem()
	second := validBatchVideoPlanningItem()
	second.Title = "完全不同的标题"
	assertBatchVideoPlanningRejected(t, svc, round, batchVideoPlanningTask(t, []batchVideoPlanningCreative{first, second}), "完全重复")
}

func TestBatchVideoApplyPlanningRejectsProjectHistoryDuplicateDespiteTitleChange(t *testing.T) {
	svc, round, _ := newBatchVideoPlanningFixture(t, 1)
	first := validBatchVideoPlanningItem()
	task := batchVideoPlanningTask(t, []batchVideoPlanningCreative{first})
	if err := svc.applyBatchVideoPlanning(round, task); err != nil {
		t.Fatal(err)
	}
	secondRound := &model.BatchVideoCreativeRound{
		ID: "history-round", ProjectID: round.ProjectID, UserID: round.UserID, Mode: "standard",
		RequestedCount: 1, TemplateSnapshotJSON: round.TemplateSnapshotJSON, Status: "generating",
	}
	if err := svc.repo.CreateBatchVideoCreativeRound(secondRound); err != nil {
		t.Fatal(err)
	}
	second := validBatchVideoPlanningItem()
	second.Title = "只改标题也不能规避"
	err := svc.applyBatchVideoPlanning(secondRound, batchVideoPlanningTask(t, []batchVideoPlanningCreative{second}))
	if err == nil || !strings.Contains(err.Error(), "完全重复") {
		t.Fatalf("history duplicate error = %v", err)
	}
	count, err := svc.repo.CountBatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: round.ProjectID})
	if err != nil || count != 1 {
		t.Fatalf("history duplicate persisted creatives: count=%d err=%v", count, err)
	}
}

func TestBatchVideoApplyPlanningRejectsRepeatedCopywritingAcrossDifferentCombination(t *testing.T) {
	svc, round, _ := newBatchVideoPlanningFixture(t, 1)
	first := validBatchVideoPlanningItem()
	if err := svc.applyBatchVideoPlanning(round, batchVideoPlanningTask(t, []batchVideoPlanningCreative{first})); err != nil {
		t.Fatal(err)
	}
	secondRound := &model.BatchVideoCreativeRound{
		ID: "copy-history-round", ProjectID: round.ProjectID, UserID: round.UserID, Mode: "standard",
		RequestedCount: 1, TemplateSnapshotJSON: round.TemplateSnapshotJSON, Status: "generating",
	}
	if err := svc.repo.CreateBatchVideoCreativeRound(secondRound); err != nil {
		t.Fatal(err)
	}
	second := validBatchVideoPlanningItem()
	second.Title = "新标题"
	second.Hook = "新的开场"
	second.ImageRequirement.Scene = "不同场景"
	second.ImageRequirement.Angle = "俯拍"
	second.ImageRequirement.Style = "自然纪实"
	err := svc.applyBatchVideoPlanning(secondRound, batchVideoPlanningTask(t, []batchVideoPlanningCreative{second}))
	if err == nil || !strings.Contains(err.Error(), "完全重复") {
		t.Fatalf("repeated copywriting error = %v", err)
	}
}

func TestBatchVideoApplyPlanningAllowsOrdinarySimilarity(t *testing.T) {
	svc, round, _ := newBatchVideoPlanningFixture(t, 2)
	first := validBatchVideoPlanningItem()
	second := validBatchVideoPlanningItem()
	second.Title = "另一个卖点表达"
	second.Hook = "露营时最担心什么"
	second.Copywriting = "同样是续航卖点，但这条使用不同的正文和开场。"
	if err := svc.applyBatchVideoPlanning(round, batchVideoPlanningTask(t, []batchVideoPlanningCreative{first, second})); err != nil {
		t.Fatalf("ordinary similarity was rejected: %v", err)
	}
}

func TestBatchVideoPlanningRepairMarkerIsPersistentAndFinal(t *testing.T) {
	svc, round, _ := newBatchVideoPlanningFixture(t, 1)
	result, err := json.Marshal(map[string]string{"text": "仍不是 JSON"})
	if err != nil {
		t.Fatal(err)
	}
	task := &model.Task{
		ID: "repair-task", ResultJSON: string(result),
		InputJSON: `{"metadata":{"batchVideoPlanningRepair":true}}`,
	}
	if !batchVideoTaskIsPlanningRepair(task) {
		t.Fatal("repair task metadata was not recognized")
	}
	assertBatchVideoPlanningRejected(t, svc, round, task, "修复后仍不是有效 JSON")
	if batchVideoTaskIsPlanningRepair(&model.Task{InputJSON: `{}`}) {
		t.Fatal("ordinary planning task recognized as repair")
	}
}

func TestPrepareBatchVideoModelsRejectsUnconfiguredModel(t *testing.T) {
	// 没有仓储时仍应拒绝空模型标识，确保前端隐藏不是授权边界。
	svc := &Service{}
	_, err := svc.prepareBatchVideoModels([]BatchVideoSubmitModel{{}}, 1)
	if err == nil {
		t.Fatal("prepareBatchVideoModels() expected error for empty channel model id")
	}
}
