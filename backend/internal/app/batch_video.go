package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

const (
	batchVideoSettingsKey          = "batch_video_settings"
	defaultBatchVideoModelPriority = DefaultModelPriority
	// 默认同时分析 10 张；上限防止单项目一次性打满渠道并发。
	defaultBatchVideoAnalysisConcurrency = 10
	maxBatchVideoAnalysisConcurrency     = 50
)

func batchVideoModelPriority(value int) int {
	if value <= 0 {
		return defaultBatchVideoModelPriority
	}
	return value
}

type BatchVideoSettings struct {
	StandardCreativeCount    int     `json:"standardCreativeCount"`
	StandardMinSuccess       int     `json:"standardMinSuccess"`
	StandardSuccessRate      float64 `json:"standardSuccessRate"`
	MaxStandardCreatives     int     `json:"maxStandardCreatives"`
	MaxMassCreatives         int     `json:"maxMassCreatives"`
	ReferencePoolStandard    int     `json:"referencePoolStandard"`
	ReferencePoolMass        int     `json:"referencePoolMass"`
	MaxNewReferencesPerRound int     `json:"maxNewReferencesPerRound"`
	MaxAttemptsPerModel      int     `json:"maxAttemptsPerModel"`
	DefaultSubmissionMode    string  `json:"defaultSubmissionMode"`
	SamplingRate             float64 `json:"samplingRate"`
	// AnalysisConcurrency 是逐张素材分析同时运行的任务数；0 或非法值回落到默认值。
	AnalysisConcurrency int `json:"analysisConcurrency"`
}

func defaultBatchVideoSettings() BatchVideoSettings {
	return BatchVideoSettings{
		StandardCreativeCount:    10,
		StandardMinSuccess:       1,
		StandardSuccessRate:      0.8,
		MaxStandardCreatives:     20,
		MaxMassCreatives:         1000,
		ReferencePoolStandard:    20,
		ReferencePoolMass:        50,
		MaxNewReferencesPerRound: 20,
		MaxAttemptsPerModel:      2,
		DefaultSubmissionMode:    "single",
		SamplingRate:             0.1,
		AnalysisConcurrency:      defaultBatchVideoAnalysisConcurrency,
	}
}

func (s *Service) BatchVideoSettings() (BatchVideoSettings, error) {
	setting, err := s.repo.SystemSettingOptional(batchVideoSettingsKey)
	if err != nil {
		return BatchVideoSettings{}, err
	}
	value := defaultBatchVideoSettings()
	if setting == nil || strings.TrimSpace(setting.ValueJSON) == "" {
		return value, nil
	}
	if err := json.Unmarshal([]byte(setting.ValueJSON), &value); err != nil {
		return BatchVideoSettings{}, fmt.Errorf("解析批量视频设置失败：%w", err)
	}
	return normalizeBatchVideoSettings(value), nil
}

func (s *Service) AdminBatchVideoSettings(actor *model.User) (BatchVideoSettings, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return BatchVideoSettings{}, err
	}
	return s.BatchVideoSettings()
}

func normalizeBatchVideoSettings(value BatchVideoSettings) BatchVideoSettings {
	defaults := defaultBatchVideoSettings()
	if value.StandardCreativeCount < 5 || value.StandardCreativeCount > 20 {
		value.StandardCreativeCount = defaults.StandardCreativeCount
	}
	if value.StandardMinSuccess <= 0 {
		value.StandardMinSuccess = defaults.StandardMinSuccess
	}
	if value.StandardSuccessRate <= 0 || value.StandardSuccessRate > 1 {
		value.StandardSuccessRate = defaults.StandardSuccessRate
	}
	if value.MaxStandardCreatives < 5 || value.MaxStandardCreatives > 20 {
		value.MaxStandardCreatives = defaults.MaxStandardCreatives
	}
	if value.MaxMassCreatives <= 0 || value.MaxMassCreatives > 1000 {
		value.MaxMassCreatives = defaults.MaxMassCreatives
	}
	if value.ReferencePoolStandard <= 0 {
		value.ReferencePoolStandard = defaults.ReferencePoolStandard
	}
	if value.ReferencePoolMass <= 0 {
		value.ReferencePoolMass = defaults.ReferencePoolMass
	}
	if value.MaxNewReferencesPerRound <= 0 {
		value.MaxNewReferencesPerRound = defaults.MaxNewReferencesPerRound
	}
	if value.AnalysisConcurrency <= 0 {
		value.AnalysisConcurrency = defaults.AnalysisConcurrency
	}
	if value.AnalysisConcurrency > maxBatchVideoAnalysisConcurrency {
		value.AnalysisConcurrency = maxBatchVideoAnalysisConcurrency
	}
	if value.MaxAttemptsPerModel <= 0 {
		value.MaxAttemptsPerModel = defaults.MaxAttemptsPerModel
	}
	if value.DefaultSubmissionMode != "multi" {
		value.DefaultSubmissionMode = "single"
	}
	if value.SamplingRate <= 0 || value.SamplingRate > 1 {
		value.SamplingRate = defaults.SamplingRate
	}
	return value
}

func (s *Service) UpdateBatchVideoSettings(actor *model.User, value BatchVideoSettings) (BatchVideoSettings, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return BatchVideoSettings{}, err
	}
	value = normalizeBatchVideoSettings(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return BatchVideoSettings{}, err
	}
	setting := &model.SystemSetting{Key: batchVideoSettingsKey, ValueJSON: string(encoded), UpdatedBy: actor.ID}
	if err := s.repo.SaveSystemSetting(setting); err != nil {
		return BatchVideoSettings{}, err
	}
	return value, nil
}

type BatchVideoProjectSettings struct {
	DurationSeconds       int      `json:"durationSeconds"`
	AspectRatio           string   `json:"aspectRatio"`
	StandardCreativeCount int      `json:"standardCreativeCount"`
	Templates             []string `json:"templates,omitempty"`
}

type BatchVideoMaterialInput struct {
	ResourceID  string   `json:"resourceId"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type BatchVideoDocumentInput struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type BatchVideoSellingPointInput struct {
	Text     string `json:"text"`
	Evidence string `json:"evidence"`
	Source   string `json:"source"`
}

type CreateBatchVideoProjectInput struct {
	Name          string                        `json:"name"`
	ProductName   string                        `json:"productName"`
	Settings      BatchVideoProjectSettings     `json:"settings"`
	Materials     []BatchVideoMaterialInput     `json:"materials"`
	Documents     []BatchVideoDocumentInput     `json:"documents"`
	SellingPoints []BatchVideoSellingPointInput `json:"sellingPoints"`
}

type batchVideoMaterialJSON struct {
	Materials []BatchVideoMaterialInput `json:"materials"`
	Documents []BatchVideoDocumentInput `json:"documents"`
}

func (s *Service) validateBatchVideoInputs(userID string, materials []BatchVideoMaterialInput, documents []BatchVideoDocumentInput) error {
	if len(materials) > 100 {
		return BadAuthRequest("单个素材批次最多 100 个文件")
	}
	for _, material := range materials {
		if strings.TrimSpace(material.ResourceID) == "" {
			return BadAuthRequest("素材缺少资源 ID")
		}
		if material.Kind != "image" && material.Kind != "video" {
			return BadAuthRequest("批量视频素材只支持图片或视频")
		}
		resource, err := s.repo.ResourceForUser(userID, material.ResourceID)
		if err != nil || resource.Status != model.ResourceStatusReady || resource.Kind != material.Kind {
			return BadAuthRequest("素材不存在、尚未就绪或类型不匹配")
		}
	}
	totalRunes := 0
	for _, document := range documents {
		size := utf8.RuneCountInString(strings.TrimSpace(document.Text))
		if size == 0 || size > 100000 {
			return BadAuthRequest("单篇卖点文档必须为 1–100000 字")
		}
		totalRunes += size
		if totalRunes > 200000 {
			return BadAuthRequest("单个素材批次的文档总长度不能超过 200000 字")
		}
	}
	return nil
}

type BatchVideoProjectDetail struct {
	Project         model.BatchVideoProject          `json:"project"`
	Settings        BatchVideoProjectSettings        `json:"settings"`
	AnalysisTask    *BatchVideoAnalysisTask          `json:"analysisTask,omitempty"`
	MaterialBatches []BatchVideoMaterialBatchView    `json:"materialBatches"`
	SellingPoints   []model.BatchVideoSellingPoint   `json:"sellingPoints"`
	ReferenceAssets []model.BatchVideoReferenceAsset `json:"referenceAssets"`
	Rounds          []model.BatchVideoCreativeRound  `json:"rounds"`
	Creatives       []model.BatchVideoCreative       `json:"creatives"`
	Jobs            []model.BatchVideoGenerationJob  `json:"jobs"`
	Counts          BatchVideoProjectCounts          `json:"counts"`
	Validation      BatchVideoValidationState        `json:"validation"`
}

// BatchVideoAnalysisTask 只暴露用户可见的分析任务进度，不把任务正文或上游标识带给前端。
type BatchVideoAnalysisTask struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Progress   int    `json:"progress"`
	Error      string `json:"error"`
	AssetID    string `json:"assetId,omitempty"`
	AssetTitle string `json:"assetTitle,omitempty"`
	Total      int    `json:"total"`
	Done       int    `json:"done"`
	Failed     int    `json:"failed"`
	// Running 是当前并行分析中的图片数量，便于前端显示并发进度。
	Running     int        `json:"running"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type BatchVideoProjectCounts struct {
	CreativeTotal  int64 `json:"creativeTotal"`
	Approved       int64 `json:"approved"`
	PendingReview  int64 `json:"pendingReview"`
	Rejected       int64 `json:"rejected"`
	JobsTotal      int64 `json:"jobsTotal"`
	Submitted      int64 `json:"submitted"`
	Waiting        int64 `json:"waiting"`
	Running        int64 `json:"running"`
	Succeeded      int64 `json:"succeeded"`
	Failed         int64 `json:"failed"`
	Cancelled      int64 `json:"cancelled"`
	Attempts       int64 `json:"attempts"`
	RetrySucceeded int64 `json:"retrySucceeded"`
	RetryFailed    int64 `json:"retryFailed"`
}

type BatchVideoMaterialBatchView struct {
	model.BatchVideoMaterialBatch
	Materials []BatchVideoMaterialInput `json:"materials"`
	Documents []BatchVideoDocumentInput `json:"documents"`
}

func (s *Service) CreateBatchVideoProject(userID string, input CreateBatchVideoProjectInput) (*BatchVideoProjectDetail, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, BadAuthRequest("请填写项目名称")
	}
	hasSource := len(input.Materials) > 0 || len(input.Documents) > 0
	if hasSource {
		if err := s.validateBatchVideoInputs(userID, input.Materials, input.Documents); err != nil {
			return nil, err
		}
		input.Materials = uniqueBatchVideoMaterials(input.Materials)
		sortBatchVideoMaterials(input.Materials)
	}
	settings := input.Settings
	if settings.DurationSeconds <= 0 {
		settings.DurationSeconds = 8
	}
	if settings.AspectRatio == "" {
		settings.AspectRatio = "9:16"
	}
	adminSettings, err := s.BatchVideoSettings()
	if err != nil {
		return nil, err
	}
	uniqueImages := map[string]bool{}
	for _, material := range input.Materials {
		if material.Kind == "image" && material.ResourceID != "" {
			uniqueImages[material.ResourceID] = true
		}
	}
	if len(uniqueImages) > adminSettings.ReferencePoolStandard {
		return nil, BadAuthRequest(fmt.Sprintf("标准阶段最多保留 %d 张参考图，当前提交了 %d 张；请先精简素材", adminSettings.ReferencePoolStandard, len(uniqueImages)))
	}
	if settings.StandardCreativeCount <= 0 {
		settings.StandardCreativeCount = adminSettings.StandardCreativeCount
	}
	if settings.StandardCreativeCount < 5 || settings.StandardCreativeCount > 20 {
		return nil, BadAuthRequest("标准验收创意数量必须为 5–20 条")
	}
	project := &model.BatchVideoProject{
		ID:                    newID(),
		UserID:                userID,
		Name:                  name,
		ProductName:           strings.TrimSpace(input.ProductName),
		Stage:                 "draft",
		Status:                "draft",
		CurrentProfileVersion: 1,
		ReferencePoolVersion:  1,
	}
	settingsJSON, _ := json.Marshal(settings)
	project.SettingsJSON = string(settingsJSON)
	if err := s.repo.CreateBatchVideoProject(project); err != nil {
		return nil, err
	}
	batchID := ""
	if hasSource {
		batch, err := s.createMaterialBatch(userID, project.ID, 1, "首批素材", input.Materials, input.Documents, "draft")
		if err != nil {
			return nil, err
		}
		batchID = batch.ID
	}
	if batchID != "" || len(input.SellingPoints) > 0 {
		if err := s.createSellingPoints(project, batchID, input.SellingPoints, input.Materials, input.Documents); err != nil {
			return nil, err
		}
	}
	if err := s.createReferenceAssetsFromMaterials(project, input.Materials); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, project.ID)
}

func (s *Service) createMaterialBatch(userID, projectID string, batchNo int, label string, materials []BatchVideoMaterialInput, documents []BatchVideoDocumentInput, status string) (*model.BatchVideoMaterialBatch, error) {
	payload, _ := json.Marshal(batchVideoMaterialJSON{Materials: materials, Documents: documents})
	batch := &model.BatchVideoMaterialBatch{
		ID:           newID(),
		ProjectID:    projectID,
		UserID:       userID,
		BatchNo:      batchNo,
		Label:        label,
		MaterialJSON: string(payload),
		Status:       status,
	}
	if err := s.repo.CreateBatchVideoMaterialBatch(batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func (s *Service) createSellingPoints(project *model.BatchVideoProject, batchID string, input []BatchVideoSellingPointInput, materials []BatchVideoMaterialInput, documents []BatchVideoDocumentInput) error {
	points := make([]model.BatchVideoSellingPoint, 0, len(input))
	if len(input) > 0 {
		for _, point := range input {
			text := strings.TrimSpace(point.Text)
			if text == "" {
				continue
			}
			source := point.Source
			if source == "" {
				source = "manual"
			}
			evidence, _ := json.Marshal(map[string]string{"text": point.Evidence})
			points = append(points, model.BatchVideoSellingPoint{
				ID:               newID(),
				ProjectID:        project.ID,
				UserID:           project.UserID,
				CanonicalText:    text,
				EvidenceJSON:     string(evidence),
				Confidence:       1,
				Source:           source,
				Status:           "active",
				FirstSeenBatchID: batchID,
			})
		}
	}
	if len(points) == 0 {
		for _, text := range deriveSellingPoints(materials, documents) {
			points = append(points, model.BatchVideoSellingPoint{
				ID:               newID(),
				ProjectID:        project.ID,
				UserID:           project.UserID,
				CanonicalText:    text,
				EvidenceJSON:     `{"text":"从素材或文档提取"}`,
				Confidence:       0.6,
				Source:           "material",
				Status:           "active",
				FirstSeenBatchID: batchID,
			})
		}
	}
	if len(points) == 0 {
		text := strings.TrimSpace(project.ProductName)
		if text == "" {
			text = project.Name
		}
		points = append(points, model.BatchVideoSellingPoint{
			ID:            newID(),
			ProjectID:     project.ID,
			UserID:        project.UserID,
			CanonicalText: text + "核心卖点",
			EvidenceJSON:  `{"text":"默认卖点，请在产品档案中补充"}`,
			Confidence:    0.3,
			Source:        "fallback",
			Status:        "active",
		})
	}
	return s.repo.CreateBatchVideoSellingPoints(points)
}

func deriveSellingPoints(materials []BatchVideoMaterialInput, documents []BatchVideoDocumentInput) []string {
	seen := map[string]bool{}
	result := []string{}
	add := func(text string) {
		text = strings.TrimSpace(strings.TrimLeft(text, "-*•·0123456789.、) "))
		if len([]rune(text)) < 4 || len([]rune(text)) > 120 || seen[text] {
			return
		}
		seen[text] = true
		result = append(result, text)
	}
	for _, document := range documents {
		for _, line := range strings.Split(document.Text, "\n") {
			add(line)
			if len(result) >= 20 {
				return result
			}
		}
	}
	for _, material := range materials {
		add(material.Description)
		if len(result) >= 20 {
			return result
		}
	}
	return result
}

func (s *Service) createReferenceAssetsFromMaterials(project *model.BatchVideoProject, materials []BatchVideoMaterialInput) error {
	for _, material := range materials {
		if material.Kind != "image" || material.ResourceID == "" {
			continue
		}
		resource, err := s.repo.ResourceForUser(project.UserID, material.ResourceID)
		if err != nil || resource.Status != model.ResourceStatusReady || resource.Kind != "image" {
			return BadAuthRequest("素材参考图不存在或尚未就绪")
		}
		hash := ""
		if resource.ContentHash != nil {
			hash = *resource.ContentHash
		}
		if hash != "" {
			if existing, err := s.repo.BatchVideoReferenceAssetByHash(project.ID, hash); err == nil && existing != nil {
				continue
			}
		}
		title := strings.TrimSpace(material.Title)
		if title == "" {
			title = "素材参考图"
		}
		tags, _ := json.Marshal(material.Tags)
		asset := &model.BatchVideoReferenceAsset{
			ID:          newID(),
			ProjectID:   project.ID,
			UserID:      project.UserID,
			ResourceID:  projectResourceID(resource),
			ContentHash: hash,
			Title:       title,
			Description: material.Description,
			ImagePrompt: material.Description,
			TagsJSON:    string(tags),
			Source:      "upload",
			Status:      "approved",
			Version:     1,
		}
		if err := s.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
			return err
		}
	}
	return nil
}

func projectResourceID(resource *model.Resource) string {
	if resource == nil {
		return ""
	}
	return resource.ID
}

// batchVideoReferencePoolUsage 只统计尚未进入参考图库的图片；内容哈希优先，其次资源 ID。
func (s *Service) batchVideoReferencePoolUsage(project *model.BatchVideoProject, materials []BatchVideoMaterialInput) (newCount int, total int, err error) {
	assets, err := s.repo.BatchVideoReferenceAssets(project.ID, "")
	if err != nil {
		return 0, 0, err
	}
	existing := map[string]bool{}
	for _, asset := range assets {
		if asset.ContentHash != "" {
			existing["h:"+asset.ContentHash] = true
		}
		if asset.ResourceID != "" {
			existing["r:"+asset.ResourceID] = true
		}
	}
	added := 0
	for _, material := range materials {
		if material.Kind != "image" || material.ResourceID == "" {
			continue
		}
		resource, err := s.repo.ResourceForUser(project.UserID, material.ResourceID)
		if err != nil || resource.Status != model.ResourceStatusReady || resource.Kind != "image" {
			return 0, 0, BadAuthRequest("素材参考图不存在或尚未就绪")
		}
		key := "r:" + resource.ID
		if resource.ContentHash != nil && *resource.ContentHash != "" {
			key = "h:" + *resource.ContentHash
		}
		if existing[key] {
			continue
		}
		existing[key] = true
		added++
	}
	return added, len(assets) + added, nil
}

// uniqueBatchVideoMaterials 去掉同一批次里重复选择的资源，避免删除和 React 列表只处理第一条。
func uniqueBatchVideoMaterials(materials []BatchVideoMaterialInput) []BatchVideoMaterialInput {
	seen := make(map[string]bool, len(materials))
	result := make([]BatchVideoMaterialInput, 0, len(materials))
	for _, material := range materials {
		key := material.Kind + "\x00" + material.ResourceID
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, material)
	}
	return result
}

// sortBatchVideoMaterials 将纯数字命名的图片按数值升序重排（1、2、10 而不是 1、10、2）；
// 只替换数字命名占用的位置，非数字命名和视频保持原有相对顺序。
func sortBatchVideoMaterials(materials []BatchVideoMaterialInput) {
	indexes := make([]int, 0, len(materials))
	for index := range materials {
		if materials[index].Kind != "image" {
			continue
		}
		if _, ok := batchVideoNumericTitle(materials[index].Title); ok {
			indexes = append(indexes, index)
		}
	}
	if len(indexes) < 2 {
		return
	}
	ordered := make([]BatchVideoMaterialInput, len(indexes))
	for position, index := range indexes {
		ordered[position] = materials[index]
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, _ := batchVideoNumericTitle(ordered[i].Title)
		right, _ := batchVideoNumericTitle(ordered[j].Title)
		return left < right
	})
	for position, index := range indexes {
		materials[index] = ordered[position]
	}
}

func batchVideoNumericTitle(title string) (int, bool) {
	value := strings.TrimSpace(title)
	if value == "" {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return number, true
}

func (s *Service) AddBatchVideoMaterialBatch(userID, projectID string, label string, materials []BatchVideoMaterialInput, documents []BatchVideoDocumentInput) (*BatchVideoMaterialBatchView, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if len(materials) == 0 && len(documents) == 0 {
		return nil, BadAuthRequest("请上传素材或文档")
	}
	if err := s.validateBatchVideoInputs(userID, materials, documents); err != nil {
		return nil, err
	}
	materials = uniqueBatchVideoMaterials(materials)
	next, err := s.repo.NextBatchVideoMaterialBatchNo(project.ID)
	if err != nil {
		return nil, err
	}
	adminSettings, err := s.BatchVideoSettings()
	if err != nil {
		return nil, err
	}
	newRefs, totalRefs, err := s.batchVideoReferencePoolUsage(project, materials)
	if err != nil {
		return nil, err
	}
	poolLimit := adminSettings.ReferencePoolStandard
	if project.MassUnlockedAt != nil {
		poolLimit = adminSettings.ReferencePoolMass
	}
	if newRefs > adminSettings.MaxNewReferencesPerRound {
		return nil, BadAuthRequest(fmt.Sprintf("单轮最多新增 %d 张参考图，当前新增 %d 张", adminSettings.MaxNewReferencesPerRound, newRefs))
	}
	if totalRefs > poolLimit {
		return nil, BadAuthRequest(fmt.Sprintf("当前阶段参考图池最多 %d 张，新增后会达到 %d 张", poolLimit, totalRefs))
	}
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		label = trimmed
	} else {
		label = fmt.Sprintf("%s 素材批次 %d", time.Now().Format("2006-01-02"), next)
	}
	sortBatchVideoMaterials(materials)
	batch, err := s.createMaterialBatch(userID, project.ID, next, label, materials, documents, "ready")
	if err != nil {
		return nil, err
	}
	if err := s.createReferenceAssetsFromMaterials(project, materials); err != nil {
		return nil, err
	}
	if newRefs > 0 {
		if err := s.repo.UpdateBatchVideoProject(userID, project.ID, map[string]any{"reference_pool_version": project.ReferencePoolVersion + 1}); err != nil {
			return nil, err
		}
	}
	return &BatchVideoMaterialBatchView{
		BatchVideoMaterialBatch: *batch,
		Materials:               materials,
		Documents:               documents,
	}, nil
}

func (s *Service) AnalyzeBatchVideoProject(userID, projectID, scope string) (*BatchVideoProjectDetail, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if batchVideoAnalysisActive(project.AnalysisStatus) {
		return s.BatchVideoProjectDetail(userID, projectID)
	}
	batches, err := s.repo.BatchVideoMaterialBatches(projectID)
	if err != nil {
		return nil, err
	}
	materials := []BatchVideoMaterialInput{}
	documents := []BatchVideoDocumentInput{}
	for _, batch := range batches {
		var payload batchVideoMaterialJSON
		if err := json.Unmarshal([]byte(batch.MaterialJSON), &payload); err == nil {
			materials = append(materials, payload.Materials...)
			documents = append(documents, payload.Documents...)
		}
	}
	analysisScope, err := normalizeBatchVideoAnalysisScope(scope)
	if err != nil {
		return nil, err
	}
	if _, err := s.startBatchVideoAnalysis(userID, project, materials, documents, analysisScope); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

// ContinueBatchVideoAnalysis 在单张失败后跳过该图片，继续补位分析剩余图片。
func (s *Service) ContinueBatchVideoAnalysis(userID, projectID string) (*BatchVideoProjectDetail, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if project.AnalysisStatus != batchVideoAnalysisAwaitingDecision {
		return nil, BadAuthRequest("当前没有等待确认的分析失败")
	}
	if err := s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{
		"analysis_status":           "running",
		"analysis_error":            "",
		"analysis_current_asset_id": "",
		"stage":                     "analyzing",
		"status":                    "running",
	}); err != nil {
		return nil, err
	}
	project.AnalysisStatus = "running"
	materials, documents := s.batchVideoAnalysisInputs(projectID)
	if err := s.dispatchBatchVideoAssetAnalysis(project, materials, documents); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

// cancelBatchVideoAssetAnalysisTasks 取消并行分析中所有在途任务并标记对应素材；
// 纯文档项目取消项目自身任务。返回实际取消的任务数。
func (s *Service) cancelBatchVideoAssetAnalysisTasks(ctx context.Context, userID string, project *model.BatchVideoProject) (int, error) {
	uploads, err := s.batchVideoAnalysisAssets(project.ID)
	if err != nil {
		return 0, err
	}
	cancelled := 0
	cancelledTasks := map[string]bool{}
	for index := range uploads {
		asset := &uploads[index]
		if asset.AnalysisStatus != "queued" && asset.AnalysisStatus != "running" {
			continue
		}
		if strings.TrimSpace(asset.AnalysisTaskID) != "" {
			_, _ = s.taskLifecycle().cancelTaskWithIntent(ctx, userID, asset.AnalysisTaskID, model.TaskCancellationIntent{
				Source:  model.TaskCancellationParentCancelled,
				ActorID: userID,
			})
			cancelledTasks[asset.AnalysisTaskID] = true
		}
		if err := s.updateBatchVideoAssetAnalysis(userID, asset.ID, map[string]any{
			"analysis_status": "cancelled",
			"analysis_error":  "分析已中止，可重新分析",
		}); err != nil {
			return cancelled, err
		}
		cancelled++
	}
	// 项目主任务用于纯文档分析和旧状态兜底；图片项目已按素材任务取消，避免重复取消。
	if taskID := strings.TrimSpace(project.AnalysisTaskID); taskID != "" && !cancelledTasks[taskID] {
		if _, err := s.taskLifecycle().cancelTaskWithIntent(ctx, userID, taskID, model.TaskCancellationIntent{
			Source:  model.TaskCancellationParentCancelled,
			ActorID: userID,
		}); err == nil {
			cancelled++
		}
	}
	return cancelled, nil
}

// CancelBatchVideoAnalysis 中止当前分析任务。并行分析会取消所有在途图片任务。
func (s *Service) CancelBatchVideoAnalysis(ctx context.Context, userID, projectID string) (*BatchVideoProjectDetail, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	awaiting := project.AnalysisStatus == batchVideoAnalysisAwaitingDecision
	if !awaiting && project.AnalysisStatus != "queued" && project.AnalysisStatus != "running" {
		return nil, BadAuthRequest("当前没有可中止的分析任务")
	}
	if _, err := s.cancelBatchVideoAssetAnalysisTasks(ctx, userID, project); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{
		"analysis_status":           "cancelled",
		"analysis_error":            "分析已中止，可重新分析",
		"analysis_current_asset_id": "",
		"stage":                     "draft",
		"status":                    "ready",
	}); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

// CancelBatchVideoPlanning 中止所有进行中的策划轮次；已完成的轮次和创意保留。
func (s *Service) CancelBatchVideoPlanning(ctx context.Context, userID, projectID string) (*BatchVideoProjectDetail, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return nil, err
	}
	rounds, err := s.repo.BatchVideoCreativeRounds(projectID)
	if err != nil {
		return nil, err
	}
	cancelled := 0
	for _, round := range rounds {
		if round.Status != "queued" && round.Status != "generating" && round.Status != "running" {
			continue
		}
		if strings.TrimSpace(round.TaskID) != "" {
			_, _ = s.taskLifecycle().cancelTaskWithIntent(ctx, userID, round.TaskID, model.TaskCancellationIntent{
				Source:  model.TaskCancellationParentCancelled,
				ActorID: userID,
			})
		}
		if err := s.repo.UpdateBatchVideoCreativeRound(userID, round.ID, map[string]any{
			"status": "failed",
			"error":  "策划已中止，可重新生成",
		}); err != nil {
			return nil, err
		}
		cancelled++
	}
	if cancelled == 0 {
		return nil, BadAuthRequest("当前没有进行中的策划任务")
	}
	if err := s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{"stage": "draft", "status": "ready"}); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

func (s *Service) UpdateBatchVideoProjectSettings(userID, projectID string, input BatchVideoProjectSettings) (*BatchVideoProjectDetail, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if input.DurationSeconds <= 0 {
		input.DurationSeconds = 8
	}
	if input.DurationSeconds > 60 {
		return nil, BadAuthRequest("视频时长不能超过 60 秒")
	}
	if strings.TrimSpace(input.AspectRatio) == "" {
		input.AspectRatio = "9:16"
	}
	adminSettings, err := s.BatchVideoSettings()
	if err != nil {
		return nil, err
	}
	if input.StandardCreativeCount <= 0 {
		input.StandardCreativeCount = adminSettings.StandardCreativeCount
	}
	if input.StandardCreativeCount < 5 || input.StandardCreativeCount > 20 {
		return nil, BadAuthRequest("标准验收创意数量必须为 5–20 条")
	}
	if input.StandardCreativeCount > adminSettings.MaxStandardCreatives {
		return nil, BadAuthRequest(fmt.Sprintf("标准轮创意数量不能超过 %d", adminSettings.MaxStandardCreatives))
	}
	encoded, _ := json.Marshal(input)
	data := map[string]any{"settings_json": string(encoded)}
	if project.AnalysisStatus != "queued" && project.AnalysisStatus != "running" {
		data["analysis_status"] = "stale"
		data["analysis_current_asset_id"] = ""
	}
	if err := s.repo.UpdateBatchVideoProject(userID, projectID, data); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

func (s *Service) BatchVideoProjectDetail(userID, projectID string) (*BatchVideoProjectDetail, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	settings := BatchVideoProjectSettings{}
	_ = json.Unmarshal([]byte(project.SettingsJSON), &settings)
	batches, _ := s.repo.BatchVideoMaterialBatches(projectID)
	batchViews := make([]BatchVideoMaterialBatchView, 0, len(batches))
	for _, batch := range batches {
		var payload batchVideoMaterialJSON
		_ = json.Unmarshal([]byte(batch.MaterialJSON), &payload)
		materials := payload.Materials
		if materials == nil {
			materials = []BatchVideoMaterialInput{}
		}
		for index := range materials {
			if materials[index].Tags == nil {
				materials[index].Tags = []string{}
			}
		}
		documents := payload.Documents
		if documents == nil {
			documents = []BatchVideoDocumentInput{}
		}
		batchViews = append(batchViews, BatchVideoMaterialBatchView{BatchVideoMaterialBatch: batch, Materials: materials, Documents: documents})
	}
	points, _ := s.repo.BatchVideoSellingPoints(projectID)
	assets, _ := s.repo.BatchVideoReferenceAssets(projectID, "")
	rounds, _ := s.repo.BatchVideoCreativeRounds(projectID)
	creatives, _ := s.repo.BatchVideoCreatives(repositoryBatchVideoFilter(projectID, 100, 0))
	jobs, _ := s.repo.BatchVideoJobs(projectID, "", 100, 0)
	counts, _ := s.repo.BatchVideoCounts(projectID)
	validation, _ := s.batchVideoValidationState(userID, projectID)
	detail := &BatchVideoProjectDetail{
		Project:         *project,
		Settings:        settings,
		MaterialBatches: batchViews,
		SellingPoints:   points,
		ReferenceAssets: assets,
		Rounds:          rounds,
		Creatives:       creatives,
		Jobs:            jobs,
		Counts:          BatchVideoProjectCounts(counts),
		Validation:      validation,
	}
	if project.AnalysisTaskID != "" {
		if task, err := s.repo.TaskForUser(userID, project.AnalysisTaskID); err == nil {
			total, done, failed, running := 0, 0, 0, 0
			assetTitle := ""
			runningAssetID := ""
			for index := range assets {
				if assets[index].Source == "generated" || strings.TrimSpace(assets[index].ResourceID) == "" {
					continue
				}
				total++
				switch assets[index].AnalysisStatus {
				case "succeeded":
					done++
				case "failed":
					done++
					failed++
				case "queued", "running":
					running++
					if runningAssetID == "" {
						runningAssetID = assets[index].ID
						assetTitle = assets[index].Title
					}
				}
				if runningAssetID == "" && assets[index].ID == project.AnalysisCurrentAssetID {
					assetTitle = assets[index].Title
				}
			}
			status := string(task.Status)
			switch project.AnalysisStatus {
			case batchVideoAnalysisAwaitingDecision:
				status = batchVideoAnalysisAwaitingDecision
			case "queued", "running":
				status = project.AnalysisStatus
			}
			progress := task.Progress
			if total > 0 {
				progress = done * 100 / total
			}
			errorText := task.Error
			if project.AnalysisStatus == batchVideoAnalysisAwaitingDecision {
				errorText = project.AnalysisError
			}
			assetID := runningAssetID
			if assetID == "" {
				assetID = project.AnalysisCurrentAssetID
			}
			detail.AnalysisTask = &BatchVideoAnalysisTask{
				ID:          task.ID,
				Status:      status,
				Stage:       task.Stage,
				Progress:    progress,
				Error:       errorText,
				AssetID:     assetID,
				AssetTitle:  assetTitle,
				Total:       total,
				Done:        done,
				Failed:      failed,
				Running:     running,
				CreatedAt:   task.CreatedAt,
				StartedAt:   task.StartedAt,
				CompletedAt: task.CompletedAt,
			}
		}
	}
	return detail, nil
}

func repositoryBatchVideoFilter(projectID string, limit, offset int) repository.BatchVideoCreativeFilter {
	return repository.BatchVideoCreativeFilter{ProjectID: projectID, Limit: limit, Offset: offset}
}

func (s *Service) ListBatchVideoCreatives(userID, projectID, roundID, reviewStatus, matchStatus string, page, pageSize int) ([]model.BatchVideoCreative, int64, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return nil, 0, err
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	filter := repository.BatchVideoCreativeFilter{
		ProjectID:    projectID,
		RoundID:      roundID,
		ReviewStatus: reviewStatus,
		MatchStatus:  matchStatus,
		Limit:        pageSize,
		Offset:       (page - 1) * pageSize,
	}
	creatives, err := s.repo.BatchVideoCreatives(filter)
	if err != nil {
		return nil, 0, err
	}
	filter.Limit = 0
	filter.Offset = 0
	total, err := s.repo.CountBatchVideoCreatives(filter)
	return creatives, total, err
}

type BatchVideoGenerationJobView struct {
	model.BatchVideoGenerationJob
	Progress int    `json:"progress"`
	Stage    string `json:"stage"`
}

func (s *Service) ListBatchVideoJobs(userID, projectID, status string, page, pageSize int) ([]BatchVideoGenerationJobView, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return nil, err
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	if page <= 0 {
		page = 1
	}
	jobs, err := s.repo.BatchVideoJobs(projectID, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if job.CurrentTaskID != "" {
			taskIDs = append(taskIDs, job.CurrentTaskID)
		}
	}
	tasks, err := s.repo.TasksForUserIDs(userID, taskIDs)
	if err != nil {
		return nil, err
	}
	tasksByID := make(map[string]model.Task, len(tasks))
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	views := make([]BatchVideoGenerationJobView, 0, len(jobs))
	for _, job := range jobs {
		view := BatchVideoGenerationJobView{BatchVideoGenerationJob: job}
		if task, ok := tasksByID[job.CurrentTaskID]; ok {
			view.Progress = task.Progress
			view.Stage = task.Stage
		}
		views = append(views, view)
	}
	return views, nil
}

type GenerateBatchVideoCreativesInput struct {
	Count     int      `json:"count"`
	Mode      string   `json:"mode"`
	Templates []string `json:"templates"`
}

type BatchVideoCreativeRoundResult struct {
	Round      model.BatchVideoCreativeRound `json:"round"`
	Rounds     int                           `json:"rounds"`
	RoundIDs   []string                      `json:"roundIds"`
	Created    int                           `json:"created"`
	Mode       string                        `json:"mode"`
	MatchCount int                           `json:"matchCount"`
}

func (s *Service) GenerateBatchVideoCreatives(userID, projectID string, input GenerateBatchVideoCreativesInput) (*BatchVideoCreativeRoundResult, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if input.Count <= 0 {
		return nil, BadAuthRequest("请填写创意数量")
	}
	settings, err := s.BatchVideoSettings()
	if err != nil {
		return nil, err
	}
	mode := input.Mode
	if mode == "" {
		mode = "standard"
	}
	if mode != "standard" && mode != "mass" {
		return nil, BadAuthRequest("创意轮次只能是 standard 或 mass")
	}
	if mode == "mass" && project.MassUnlockedAt == nil {
		return nil, BadAuthRequest("项目尚未解锁量产")
	}
	limit := settings.MaxStandardCreatives
	if mode == "mass" {
		limit = settings.MaxMassCreatives
	}
	if mode == "standard" && input.Count < 5 {
		return nil, BadAuthRequest("标准轮至少生成 5 条创意")
	}
	if input.Count > limit {
		return nil, BadAuthRequest(fmt.Sprintf("本模式单轮最多生成 %d 条创意", limit))
	}
	existingCreativeCount, err := s.repo.CountBatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	if existingCreativeCount+int64(input.Count) > int64(settings.MaxMassCreatives) {
		return nil, BadAuthRequest(fmt.Sprintf("单个项目累计最多 %d 条创意", settings.MaxMassCreatives))
	}
	points, err := s.repo.BatchVideoSellingPoints(projectID)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, BadAuthRequest("项目还没有卖点，请先分析素材或填写卖点")
	}
	assets, _ := s.repo.BatchVideoReferenceAssets(projectID, "approved")
	roundNo, err := s.repo.NextBatchVideoRoundNo(projectID)
	if err != nil {
		return nil, err
	}
	templates := input.Templates
	if len(templates) == 0 {
		templates = []string{"pain_point", "comparison", "unboxing", "scene", "voiceover"}
	}
	batches, err := s.repo.BatchVideoMaterialBatches(projectID)
	if err != nil {
		return nil, err
	}
	batchIDs := make([]string, 0, len(batches))
	for _, batch := range batches {
		batchIDs = append(batchIDs, batch.ID)
	}
	if mode == "mass" {
		snapshotCreatives := make([]model.BatchVideoCreative, len(templates))
		for index, templateID := range templates {
			snapshotCreatives[index].TemplateID = templateID
		}
		if err := s.validateBatchVideoMassSubmission(project, snapshotCreatives, nil); err != nil {
			return nil, err
		}
	}
	// 单次文本生成无法稳定产出上千条 JSON；按固定小块拆成多个策划轮次，
	// 每轮独立计费、独立重试，前端按轮次汇总创意。
	const planningChunkSize = 50
	remaining := input.Count
	rounds := make([]*model.BatchVideoCreativeRound, 0, (input.Count+planningChunkSize-1)/planningChunkSize)
	roundIDs := make([]string, 0, cap(rounds))
	for index := 0; remaining > 0; index++ {
		chunk := planningChunkSize
		if remaining < chunk {
			chunk = remaining
		}
		remaining -= chunk
		round := &model.BatchVideoCreativeRound{
			ID:                   newID(),
			ProjectID:            projectID,
			UserID:               userID,
			RoundNo:              roundNo + index,
			Mode:                 mode,
			RequestedCount:       chunk,
			TemplateSnapshotJSON: batchVideoMustJSON(templates),
			MaterialBatchIDsJSON: batchVideoMustJSON(batchIDs),
			ProfileVersion:       project.CurrentProfileVersion,
			Status:               "queued",
		}
		if err := s.repo.CreateBatchVideoCreativeRound(round); err != nil {
			return nil, err
		}
		roundInput := input
		roundInput.Count = chunk
		if err := s.startBatchVideoPlanning(userID, project, roundInput, round, points, assets, templates); err != nil {
			_ = s.repo.UpdateBatchVideoCreativeRound(userID, round.ID, map[string]any{"status": "failed", "error": err.Error()})
			return nil, err
		}
		rounds = append(rounds, round)
		roundIDs = append(roundIDs, round.ID)
	}
	stage := "standard_planning"
	if mode == "mass" {
		stage = "mass_planning"
	}
	_ = s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{"stage": stage, "status": "running"})
	result := BatchVideoCreativeRoundResult{Rounds: len(rounds), RoundIDs: roundIDs, Created: 0, Mode: mode, MatchCount: 0}
	if len(rounds) > 0 {
		result.Round = *rounds[0]
	}
	return &result, nil
}

func projectSettings(project *model.BatchVideoProject) BatchVideoProjectSettings {
	settings := BatchVideoProjectSettings{}
	_ = json.Unmarshal([]byte(project.SettingsJSON), &settings)
	if settings.DurationSeconds <= 0 {
		settings.DurationSeconds = 8
	}
	if settings.AspectRatio == "" {
		settings.AspectRatio = "9:16"
	}
	return settings
}

func buildTemplateCreative(round *model.BatchVideoCreativeRound, project *model.BatchVideoProject, point model.BatchVideoSellingPoint, templateID string, index, duration int, ratio string) *model.BatchVideoCreative {
	product := strings.TrimSpace(project.ProductName)
	if product == "" {
		product = project.Name
	}
	hook, copywriting, videoPrompt, imagePrompt := templateCopy(templateID, product, point.CanonicalText, index)
	requirement := map[string]any{
		"scene":            "产品使用场景",
		"angle":            "正面展示",
		"style":            "干净商业摄影",
		"aspectRatio":      ratio,
		"requiredElements": []string{product},
	}
	return &model.BatchVideoCreative{
		ID:                   newID(),
		ProjectID:            project.ID,
		RoundID:              round.ID,
		UserID:               project.UserID,
		Seq:                  index + 1,
		TemplateID:           templateID,
		SellingPointID:       point.ID,
		Title:                fmt.Sprintf("%s｜%s", point.CanonicalText, templateLabel(templateID)),
		Hook:                 hook,
		Copywriting:          copywriting,
		VideoPrompt:          videoPrompt,
		ImageRequirementJSON: batchVideoMustJSON(requirement),
		ImagePrompt:          imagePrompt,
		DurationSeconds:      duration,
		AspectRatio:          ratio,
		ReviewStatus:         "pending",
		FinalStatus:          "pending",
		SourceSnapshotJSON:   batchVideoMustJSON(map[string]any{"generator": "template", "profileVersion": project.CurrentProfileVersion}),
	}
}

func templateLabel(templateID string) string {
	switch templateID {
	case "comparison":
		return "对比型"
	case "unboxing":
		return "开箱型"
	case "scene":
		return "场景型"
	case "voiceover":
		return "口播型"
	default:
		return "痛点型"
	}
}

func templateCopy(templateID, product, sellingPoint string, index int) (string, string, string, string) {
	switch templateID {
	case "comparison":
		return fmt.Sprintf("别再为%s踩坑了", sellingPoint),
			fmt.Sprintf("同样是一个%s，为什么差别这么大？关键就在%s。", product, sellingPoint),
			fmt.Sprintf("左右对比展示%s使用前后差异，镜头从细节推到全景，突出%s。", product, sellingPoint),
			fmt.Sprintf("%s产品对比图，左右分屏，干净背景，突出%s，商业摄影，高清", product, sellingPoint)
	case "unboxing":
		return "开箱那一刻就知道值不值",
			fmt.Sprintf("今天开箱%s，第一眼最打动我的是%s。", product, sellingPoint),
			fmt.Sprintf("第一视角开箱%s，手部动作自然，镜头跟随包装打开，特写%s。", product, sellingPoint),
			fmt.Sprintf("%s开箱场景，桌面、自然光、包装完整，特写产品细节，真实质感", product)
	case "scene":
		return "这个场景真的离不开它",
			fmt.Sprintf("在日常生活里，%s让%s这件事变得简单。", product, sellingPoint),
			fmt.Sprintf("生活场景中使用%s，人物自然互动，镜头缓慢环绕，突出%s。", product, sellingPoint),
			fmt.Sprintf("%s真实生活使用场景，自然光，人物自然，产品清晰可见，生活方式摄影", product)
	case "voiceover":
		return "先说结论：值得",
			fmt.Sprintf("如果你在意%s，%s可以直接看这一条。", sellingPoint, product),
			fmt.Sprintf("正面口播介绍%s，切产品特写和使用画面，节奏紧凑，突出%s。", product, sellingPoint),
			fmt.Sprintf("%s产品正面展示图，简洁背景，主体居中，适合作为口播视频参考图", product)
	default:
		return "你是不是也遇到过这个问题",
			fmt.Sprintf("很多人买%s都会忽略一点：%s。", product, sellingPoint),
			fmt.Sprintf("用痛点开场，展示%s如何解决%s，镜头从人物表情切到产品细节。", product, sellingPoint),
			fmt.Sprintf("%s产品痛点场景参考图，人物表情自然，产品位于画面中心，商业短视频质感", product)
	}
}

func creativeDuplicateHash(creative *model.BatchVideoCreative) string {
	normalized := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(creative.Title)),
		strings.ToLower(strings.TrimSpace(creative.Hook)),
		strings.ToLower(strings.TrimSpace(creative.Copywriting)),
		strings.ToLower(strings.TrimSpace(creative.ImagePrompt)),
	}, "|")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func batchVideoMustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (s *Service) ReviewBatchVideoCreative(userID, projectID, creativeID, status, note string) (*model.BatchVideoCreative, error) {
	if status != "approved" && status != "rejected" {
		return nil, BadAuthRequest("审核状态只能是 approved 或 rejected")
	}
	if status == "rejected" && strings.TrimSpace(note) == "" {
		return nil, BadAuthRequest("驳回创意时必须填写原因")
	}
	creative, err := s.repo.BatchVideoCreativeForUser(userID, creativeID)
	if err != nil || creative.ProjectID != projectID {
		return nil, BadAuthRequest("创意不存在")
	}
	if status == "approved" && creative.MatchedReferenceAssetID == "" {
		return nil, BadAuthRequest("该创意还没有参考图，请先匹配或补充参考图")
	}
	if err := s.repo.UpdateBatchVideoCreative(userID, creativeID, map[string]any{"review_status": status, "review_note": note, "final_status": status}); err != nil {
		return nil, err
	}
	return s.repo.BatchVideoCreativeForUser(userID, creativeID)
}

func (s *Service) ReviewBatchVideoCreatives(userID, projectID string, ids []string, status, note string) (int64, error) {
	if status != "approved" && status != "rejected" {
		return 0, BadAuthRequest("审核状态只能是 approved 或 rejected")
	}
	if len(ids) == 0 {
		return 0, BadAuthRequest("请选择要审核的创意")
	}
	if status == "rejected" && strings.TrimSpace(note) == "" {
		return 0, BadAuthRequest("批量驳回时必须填写原因")
	}
	creatives, err := s.repo.BatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: projectID, Limit: 0})
	if err != nil {
		return 0, err
	}
	byID := make(map[string]model.BatchVideoCreative, len(creatives))
	for _, creative := range creatives {
		byID[creative.ID] = creative
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		creative, ok := byID[id]
		if !ok {
			return 0, BadAuthRequest("所选创意不存在或不属于当前项目")
		}
		if seen[id] {
			return 0, BadAuthRequest("所选创意重复")
		}
		seen[id] = true
		if status == "approved" && creative.MatchedReferenceAssetID == "" {
			return 0, BadAuthRequest("所选创意中有缺少参考图的内容，无法通过")
		}
	}
	return s.repo.UpdateBatchVideoCreativesReview(userID, projectID, ids, status, note)
}

type BatchVideoValidationCheck struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Passed   bool   `json:"passed"`
	Current  any    `json:"current"`
	Required any    `json:"required"`
}

type BatchVideoValidationState struct {
	Passed                bool                                `json:"passed"`
	StandardRoundID       string                              `json:"standardRoundId,omitempty"`
	StandardCreativeCount int                                 `json:"standardCreativeCount"`
	ApprovedCount         int                                 `json:"approvedCount"`
	SuccessCount          int                                 `json:"successCount"`
	SuccessRate           float64                             `json:"successRate"`
	TemplateIDs           []string                            `json:"templateIds"`
	ModelIDs              []string                            `json:"modelIds"`
	Checks                []BatchVideoValidationCheck         `json:"checks"`
	Validation            *model.BatchVideoStandardValidation `json:"validation,omitempty"`
}

func (s *Service) batchVideoValidationState(userID, projectID string) (BatchVideoValidationState, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return BatchVideoValidationState{}, err
	}
	settings, err := s.BatchVideoSettings()
	if err != nil {
		return BatchVideoValidationState{}, err
	}
	rounds, _ := s.repo.BatchVideoCreativeRounds(projectID)
	standardRoundIDs := []string{}
	standardRoundID := ""
	for _, round := range rounds {
		if round.Mode == "standard" {
			standardRoundIDs = append(standardRoundIDs, round.ID)
			standardRoundID = round.ID
		}
	}
	creativeAll, _ := s.repo.BatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: projectID, Limit: 0})
	creatives := []model.BatchVideoCreative{}
	creativeTemplates := map[string]string{}
	for _, creative := range creativeAll {
		for _, roundID := range standardRoundIDs {
			if creative.RoundID == roundID {
				creatives = append(creatives, creative)
				creativeTemplates[creative.ID] = creative.TemplateID
			}
		}
	}
	approved := 0
	for _, creative := range creatives {
		if creative.ReviewStatus == "approved" {
			approved++
		}
	}
	jobs, _ := s.repo.BatchVideoJobs(projectID, "", 0, 0)
	successCreative := map[string]bool{}
	templateSet := map[string]bool{}
	successModelSet := map[string]bool{}
	for _, job := range jobs {
		templateID, standardCreative := creativeTemplates[job.CreativeID]
		if !standardCreative || job.Status != "succeeded" {
			continue
		}
		successCreative[job.CreativeID] = true
		if templateID != "" {
			templateSet[templateID] = true
		}
		if job.ModelID != "" {
			successModelSet[job.ModelID] = true
		}
	}
	// 标准轮声明的模板必须全部产出过创意，否则“所有模板已验证”无法成立。
	declaredTemplates := map[string]bool{}
	for _, round := range rounds {
		if round.Mode != "standard" {
			continue
		}
		var values []string
		if err := json.Unmarshal([]byte(round.TemplateSnapshotJSON), &values); err == nil {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					declaredTemplates[value] = true
				}
			}
		}
	}
	coveredTemplates := 0
	for template := range declaredTemplates {
		if templateSet[template] {
			coveredTemplates++
		}
	}
	success := 0
	for _, creative := range creatives {
		if successCreative[creative.ID] {
			success++
		}
	}
	rate := 0.0
	if len(creatives) > 0 {
		rate = float64(success) / float64(len(creatives))
	}
	approvedRequired := len(creatives)
	requiredCreatives := projectSettings(project).StandardCreativeCount
	if approvedRequired == 0 {
		approvedRequired = requiredCreatives
	}
	checks := []BatchVideoValidationCheck{
		{Key: "creative_count", Label: "标准创意数量", Passed: len(creatives) >= requiredCreatives, Current: len(creatives), Required: requiredCreatives},
		{Key: "approved", Label: "标准创意全部审核通过", Passed: approved >= len(creatives) && len(creatives) > 0, Current: approved, Required: approvedRequired},
		{Key: "success", Label: "每条标准创意至少成功一条视频", Passed: success == len(creatives) && len(creatives) > 0, Current: success, Required: len(creatives)},
		{Key: "success_rate", Label: "标准视频成功率", Passed: rate >= settings.StandardSuccessRate && len(creatives) > 0, Current: rate, Required: settings.StandardSuccessRate},
		{Key: "templates", Label: "标准轮模板均已完成视频验证", Passed: len(declaredTemplates) > 0 && coveredTemplates >= len(declaredTemplates), Current: coveredTemplates, Required: len(declaredTemplates)},
		{Key: "models", Label: "至少一个视频模型验证成功", Passed: len(successModelSet) > 0, Current: len(successModelSet), Required: 1},
	}
	passed := true
	for _, check := range checks {
		if !check.Passed {
			passed = false
		}
	}
	templateIDs := make([]string, 0, len(templateSet))
	for id := range templateSet {
		templateIDs = append(templateIDs, id)
	}
	modelIDs := make([]string, 0, len(successModelSet))
	for id := range successModelSet {
		modelIDs = append(modelIDs, id)
	}
	sort.Strings(templateIDs)
	sort.Strings(modelIDs)
	state := BatchVideoValidationState{
		Passed:                passed,
		StandardRoundID:       standardRoundID,
		StandardCreativeCount: len(creatives),
		ApprovedCount:         approved,
		SuccessCount:          success,
		SuccessRate:           rate,
		TemplateIDs:           templateIDs,
		ModelIDs:              modelIDs,
		Checks:                checks,
	}
	if project.MassUnlockedAt != nil {
		if validation, err := s.repo.LatestBatchVideoStandardValidation(projectID); err == nil {
			state.Validation = validation
		}
	}
	return state, nil
}

func (s *Service) BatchVideoValidation(userID, projectID string) (BatchVideoValidationState, error) {
	return s.batchVideoValidationState(userID, projectID)
}

func (s *Service) UnlockBatchVideoMass(userID, projectID string) (*BatchVideoProjectDetail, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	state, err := s.batchVideoValidationState(userID, projectID)
	if err != nil {
		return nil, err
	}
	if !state.Passed {
		return nil, BadAuthRequest("标准验证还未达标，暂时不能解锁大批量")
	}
	batches, err := s.repo.BatchVideoMaterialBatches(projectID)
	if err != nil {
		return nil, err
	}
	batchIDs := make([]string, 0, len(batches))
	for _, batch := range batches {
		batchIDs = append(batchIDs, batch.ID)
	}
	now := time.Now()
	validation := &model.BatchVideoStandardValidation{
		ID:                       newID(),
		ProjectID:                projectID,
		UserID:                   userID,
		StandardRoundID:          state.StandardRoundID,
		CreativeCount:            state.StandardCreativeCount,
		ApprovedCount:            state.ApprovedCount,
		SuccessCount:             state.SuccessCount,
		SuccessRate:              state.SuccessRate,
		ValidatedTemplateIDsJSON: batchVideoMustJSON(state.TemplateIDs),
		ValidatedModelIDsJSON:    batchVideoMustJSON(state.ModelIDs),
		ReferencePoolVersion:     project.ReferencePoolVersion,
		MaterialBatchIDsJSON:     batchVideoMustJSON(batchIDs),
		ProfileVersion:           project.CurrentProfileVersion,
		Status:                   "passed",
		UnlockedBy:               userID,
		UnlockedAt:               &now,
	}
	if err := s.repo.UnlockBatchVideoMass(validation, now); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

type BatchVideoReferenceAssetInput struct {
	ResourceID  string   `json:"resourceId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ImagePrompt string   `json:"imagePrompt"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

func (s *Service) AddBatchVideoReferenceAssets(userID, projectID string, inputs []BatchVideoReferenceAssetInput) ([]model.BatchVideoReferenceAsset, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, BadAuthRequest("请选择要加入参考图池的图片")
	}
	settings, err := s.BatchVideoSettings()
	if err != nil {
		return nil, err
	}
	existingAssets, err := s.repo.BatchVideoReferenceAssets(projectID, "")
	if err != nil {
		return nil, err
	}
	poolLimit := settings.ReferencePoolStandard
	if project.MassUnlockedAt != nil {
		poolLimit = settings.ReferencePoolMass
	}
	created := 0
	assets := make([]model.BatchVideoReferenceAsset, 0, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input.ResourceID) == "" {
			continue
		}
		resource, err := s.repo.ResourceForUser(userID, input.ResourceID)
		if err != nil || resource.Kind != "image" || resource.Status != model.ResourceStatusReady {
			return nil, BadAuthRequest("参考图资源不存在、尚未就绪或不是图片")
		}
		hash := ""
		if resource.ContentHash != nil {
			hash = *resource.ContentHash
		}
		if hash != "" {
			if existing, err := s.repo.BatchVideoReferenceAssetByHash(project.ID, hash); err == nil && existing != nil {
				assets = append(assets, *existing)
				continue
			}
		}
		if len(existingAssets)+created >= poolLimit {
			return nil, BadAuthRequest(fmt.Sprintf("当前阶段参考图池最多 %d 张", poolLimit))
		}
		if created >= settings.MaxNewReferencesPerRound {
			return nil, BadAuthRequest(fmt.Sprintf("单轮最多新增 %d 张参考图", settings.MaxNewReferencesPerRound))
		}
		status := input.Status
		if status != "draft" && status != "approved" && status != "disabled" {
			status = "approved"
		}
		tags, _ := json.Marshal(input.Tags)
		title := strings.TrimSpace(input.Title)
		if title == "" {
			title = "参考图"
		}
		asset := &model.BatchVideoReferenceAsset{
			ID:          newID(),
			ProjectID:   project.ID,
			UserID:      userID,
			ResourceID:  resource.ID,
			ContentHash: hash,
			Title:       title,
			Description: input.Description,
			ImagePrompt: input.ImagePrompt,
			TagsJSON:    string(tags),
			Source:      "manual",
			Status:      status,
			Version:     1,
		}
		if err := s.repo.CreateBatchVideoReferenceAsset(asset); err != nil {
			return nil, err
		}
		created++
		assets = append(assets, *asset)
	}
	if created > 0 {
		if err := s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{"reference_pool_version": project.ReferencePoolVersion + 1}); err != nil {
			return nil, err
		}
	}
	return assets, nil
}

func (s *Service) ListBatchVideoReferenceAssets(userID, projectID string) ([]model.BatchVideoReferenceAsset, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return nil, err
	}
	return s.repo.BatchVideoReferenceAssets(projectID, "")
}

type BatchVideoProjectSummary struct {
	Project    model.BatchVideoProject   `json:"project"`
	Settings   BatchVideoProjectSettings `json:"settings"`
	Counts     BatchVideoProjectCounts   `json:"counts"`
	Validation BatchVideoValidationState `json:"validation"`
}

func (s *Service) ListBatchVideoProjects(userID, stage string, page, pageSize int) ([]BatchVideoProjectSummary, error) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	projects, err := s.repo.BatchVideoProjects(userID, stage, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, err
	}
	result := make([]BatchVideoProjectSummary, 0, len(projects))
	for _, project := range projects {
		settings := projectSettings(&project)
		counts, _ := s.repo.BatchVideoCounts(project.ID)
		validation, _ := s.batchVideoValidationState(userID, project.ID)
		result = append(result, BatchVideoProjectSummary{
			Project:    project,
			Settings:   settings,
			Counts:     BatchVideoProjectCounts(counts),
			Validation: validation,
		})
	}
	return result, nil
}

type BatchVideoSubmitModel struct {
	ModelID        string         `json:"modelId"`
	Model          string         `json:"model"`
	LogicalModelID string         `json:"logicalModelId"`
	ChannelModelID string         `json:"channelModelId"`
	Priority       int            `json:"-"`
	Config         map[string]any `json:"config"`
}

type BatchVideoSubmitInput struct {
	CreativeIDs    []string                `json:"creativeIds"`
	AllApproved    bool                    `json:"allApproved"`
	GenerationMode string                  `json:"generationMode,omitempty"`
	Mode           string                  `json:"mode"`
	Models         []BatchVideoSubmitModel `json:"models"`
}

func batchVideoStringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result[value] = true
		}
	}
	return result
}

func batchVideoSameStringSet(left, right []string) bool {
	leftSet := batchVideoStringSet(left)
	rightSet := batchVideoStringSet(right)
	if len(leftSet) != len(rightSet) {
		return false
	}
	for value := range leftSet {
		if !rightSet[value] {
			return false
		}
	}
	return true
}

func (s *Service) validateBatchVideoMassSubmission(project *model.BatchVideoProject, creatives []model.BatchVideoCreative, models []BatchVideoSubmitModel) error {
	if project.MassUnlockedAt == nil {
		return BadAuthRequest("项目尚未解锁量产")
	}
	validation, err := s.repo.LatestBatchVideoStandardValidation(project.ID)
	if err != nil || validation.Status != "passed" {
		return BadAuthRequest("缺少有效的标准验收快照，请重新完成验收")
	}
	var validatedTemplates, validatedModels, validatedBatches []string
	if json.Unmarshal([]byte(validation.ValidatedTemplateIDsJSON), &validatedTemplates) != nil ||
		json.Unmarshal([]byte(validation.ValidatedModelIDsJSON), &validatedModels) != nil ||
		json.Unmarshal([]byte(validation.MaterialBatchIDsJSON), &validatedBatches) != nil {
		return BadAuthRequest("标准验收快照无效，请重新完成验收")
	}
	templateSet := batchVideoStringSet(validatedTemplates)
	for _, creative := range creatives {
		if !templateSet[creative.TemplateID] {
			return BadAuthRequest("创意使用了未经验证的模板，请先完成标准复验")
		}
	}
	modelSet := batchVideoStringSet(validatedModels)
	for _, selected := range models {
		modelID := firstNonEmpty(selected.ChannelModelID, selected.ModelID)
		if !modelSet[modelID] {
			return BadAuthRequest("所选视频模型未通过标准验证，请先完成标准复验")
		}
	}
	if validation.ReferencePoolVersion != project.ReferencePoolVersion {
		return BadAuthRequest("参考图池已变化，请重新完成标准验收")
	}
	if validation.ProfileVersion != project.CurrentProfileVersion {
		return BadAuthRequest("产品档案已变化，请重新完成标准验收")
	}
	batches, err := s.repo.BatchVideoMaterialBatches(project.ID)
	if err != nil {
		return err
	}
	batchIDs := make([]string, 0, len(batches))
	for _, batch := range batches {
		batchIDs = append(batchIDs, batch.ID)
	}
	if !batchVideoSameStringSet(validatedBatches, batchIDs) {
		return BadAuthRequest("素材批次已变化，请重新完成标准验收")
	}
	return nil
}

type BatchVideoTaskCandidate struct {
	ModelID        string         `json:"modelId"`
	Model          string         `json:"model"`
	LogicalModelID string         `json:"logicalModelId"`
	ChannelModelID string         `json:"channelModelId"`
	Priority       int            `json:"priority,omitempty"`
	Config         map[string]any `json:"config"`
}

type BatchVideoTaskTemplate struct {
	Candidates      []BatchVideoTaskCandidate `json:"candidates"`
	Prompt          string                    `json:"prompt"`
	ReferenceImages []map[string]any          `json:"referenceImages"`
	DurationSeconds int                       `json:"durationSeconds"`
	AspectRatio     string                    `json:"aspectRatio"`
	Operation       string                    `json:"operation"`
	ProjectID       string                    `json:"projectId"`
	CreativeID      string                    `json:"creativeId"`
}

type batchVideoPreparedSubmission struct {
	project        *model.BatchVideoProject
	jobs           []model.BatchVideoGenerationJob
	generationMode string
}

type BatchVideoSubmissionPreflight struct {
	JobCount              int    `json:"jobCount"`
	TotalMicrocredits     int64  `json:"totalMicrocredits"`
	AvailableMicrocredits int64  `json:"availableMicrocredits"`
	Estimated             bool   `json:"estimated"`
	StoredBytes           int64  `json:"storedBytes"`
	StorageLimitBytes     int64  `json:"storageLimitBytes"`
	GenerationMode        string `json:"generationMode"`
	SubmissionFingerprint string `json:"submissionFingerprint"`
}

func (s *Service) prepareBatchVideoSubmission(userID, projectID string, input BatchVideoSubmitInput) (*batchVideoPreparedSubmission, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	if input.AllApproved {
		approved, err := s.repo.BatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: projectID, ReviewStatus: "approved", Limit: 0})
		if err != nil {
			return nil, err
		}
		input.CreativeIDs = make([]string, 0, len(approved))
		for _, creative := range approved {
			input.CreativeIDs = append(input.CreativeIDs, creative.ID)
		}
	}
	if len(input.CreativeIDs) == 0 {
		return nil, BadAuthRequest("请选择要生成的创意")
	}
	seenCreativeIDs := make(map[string]bool, len(input.CreativeIDs))
	for _, id := range input.CreativeIDs {
		if seenCreativeIDs[id] {
			return nil, BadAuthRequest("所选创意重复")
		}
		seenCreativeIDs[id] = true
	}
	if len(input.Models) == 0 {
		return nil, BadAuthRequest("请选择视频模型")
	}
	models, err := s.prepareBatchVideoModels(input.Models, len(input.CreativeIDs))
	if err != nil {
		return nil, err
	}
	input.Models = models
	mode := input.Mode
	if mode != "multi" {
		mode = "single"
	}
	creatives := make([]model.BatchVideoCreative, 0, len(input.CreativeIDs))
	for _, id := range input.CreativeIDs {
		creative, err := s.repo.BatchVideoCreativeForUser(userID, id)
		if err != nil || creative.ProjectID != projectID {
			return nil, BadAuthRequest("创意不存在")
		}
		if creative.ReviewStatus != "approved" {
			return nil, BadAuthRequest("只有审核通过的创意才能提交生成")
		}
		if creative.MatchedReferenceAssetID == "" {
			return nil, BadAuthRequest("创意缺少参考图")
		}
		creatives = append(creatives, *creative)
	}
	generationMode := input.GenerationMode
	if generationMode == "" {
		generationMode = "standard"
		if project.MassUnlockedAt != nil {
			generationMode = "mass"
		}
	}
	if generationMode != "standard" && generationMode != "mass" {
		return nil, BadAuthRequest("生成阶段只能是 standard 或 mass")
	}
	settings, err := s.BatchVideoSettings()
	if err != nil {
		return nil, err
	}
	if generationMode == "standard" && len(creatives) > settings.MaxStandardCreatives {
		return nil, BadAuthRequest(fmt.Sprintf("标准阶段单次最多提交 %d 条创意", settings.MaxStandardCreatives))
	}
	if generationMode == "mass" {
		if len(creatives) > settings.MaxMassCreatives {
			return nil, BadAuthRequest(fmt.Sprintf("量产单次最多提交 %d 条创意", settings.MaxMassCreatives))
		}
		if err := s.validateBatchVideoMassSubmission(project, creatives, input.Models); err != nil {
			return nil, err
		}
	}
	jobs := []model.BatchVideoGenerationJob{}
	for _, creative := range creatives {
		refs, err := s.repo.BatchVideoCreativeReferences([]string{creative.ID})
		if err != nil {
			return nil, err
		}
		if len(refs) == 0 && creative.MatchedReferenceAssetID != "" {
			refs = []model.BatchVideoCreativeReference{{ReferenceAssetID: creative.MatchedReferenceAssetID, Role: "reference"}}
		}
		refImages := []map[string]any{}
		prompt := creative.VideoPrompt
		for index, ref := range refs {
			asset, err := s.repo.BatchVideoReferenceAssetForUser(userID, ref.ReferenceAssetID)
			if err != nil || asset.ProjectID != projectID || asset.Status != "approved" {
				return nil, BadAuthRequest("创意引用的参考图不存在、未审核或不属于当前项目")
			}
			if ref.AssetVersion > 0 && ref.AssetVersion != asset.Version {
				return nil, BadAuthRequest("创意引用的参考图版本已变化，请重新匹配后再提交")
			}
			resource, err := s.repo.ResourceForUser(userID, asset.ResourceID)
			if err != nil || resource.Status != model.ResourceStatusReady || resource.Kind != "image" {
				return nil, BadAuthRequest("创意引用的参考图资源尚未就绪")
			}
			refImages = append(refImages, map[string]any{
				"storageKey":   "resource:" + asset.ResourceID,
				"name":         asset.Title,
				"type":         "image",
				"role":         "reference",
				"referenceId":  asset.ID,
				"assetVersion": asset.Version,
			})
			prompt += fmt.Sprintf("\n参考图 %d：%s。资产描述：%s。创意用法：%s。", index+1, asset.Title, asset.Description, ref.Caption)
		}
		if len(refImages) == 0 {
			return nil, BadAuthRequest("创意缺少可用参考图")
		}
		operation := "image_to_video"
		if mode == "multi" {
			for _, selected := range input.Models {
				template := BatchVideoTaskTemplate{
					Candidates:      []BatchVideoTaskCandidate{candidateFromSubmit(selected)},
					Prompt:          prompt,
					ReferenceImages: refImages,
					DurationSeconds: creative.DurationSeconds,
					AspectRatio:     creative.AspectRatio,
					Operation:       operation,
					ProjectID:       projectID,
					CreativeID:      creative.ID,
				}
				jobs = append(jobs, newBatchVideoJob(project, creative, mode, template))
			}
			continue
		}
		candidates := make([]BatchVideoTaskCandidate, 0, len(input.Models))
		for _, selected := range input.Models {
			candidates = append(candidates, candidateFromSubmit(selected))
		}
		template := BatchVideoTaskTemplate{
			Candidates:      candidates,
			Prompt:          prompt,
			ReferenceImages: refImages,
			DurationSeconds: creative.DurationSeconds,
			AspectRatio:     creative.AspectRatio,
			Operation:       operation,
			ProjectID:       projectID,
			CreativeID:      creative.ID,
		}
		jobs = append(jobs, newBatchVideoJob(project, creative, mode, template))
	}
	return &batchVideoPreparedSubmission{project: project, jobs: jobs, generationMode: generationMode}, nil
}

func (s *Service) preflightBatchVideoSubmission(userID string, prepared *batchVideoPreparedSubmission) (*BatchVideoSubmissionPreflight, error) {
	result := &BatchVideoSubmissionPreflight{JobCount: len(prepared.jobs), GenerationMode: prepared.generationMode}
	fingerprint := sha256.New()
	for _, job := range prepared.jobs {
		_, _ = fingerprint.Write([]byte(job.CreativeID + "\x00" + job.TaskTemplateJSON + "\n"))
		template := BatchVideoTaskTemplate{}
		if err := json.Unmarshal([]byte(job.TaskTemplateJSON), &template); err != nil || len(template.Candidates) == 0 {
			return nil, BadAuthRequest("生成配置缺失")
		}
		candidate := template.Candidates[0]
		request := batchVideoTaskRequest(template, candidate, job)
		task := &model.Task{ID: job.ID, UserID: userID, Type: request.Type, Operation: request.Operation}
		order, err := s.taskBillingOrder(userID, task, request.Input)
		if err != nil {
			return nil, err
		}
		if order != nil {
			result.TotalMicrocredits += order.AmountMicrocredits
			result.Estimated = result.Estimated || order.BillingMode == "token"
		}
	}
	result.SubmissionFingerprint = hex.EncodeToString(fingerprint.Sum(nil))
	creditsEnabled, err := s.FeatureEnabled(FeatureCredits)
	if err != nil {
		return nil, err
	}
	if creditsEnabled {
		account, err := s.repo.CreditAccount(userID)
		if err == nil {
			result.AvailableMicrocredits = account.AvailableMicrocredits
		}
		if result.AvailableMicrocredits < result.TotalMicrocredits {
			return nil, BadAuthRequest(fmt.Sprintf("本次预计需要 %d 微积分，当前可用 %d 微积分，请充值或减少提交数量", result.TotalMicrocredits, result.AvailableMicrocredits))
		}
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	if err := s.requireStoredFileCapacityForTask(userID, "canvas_video", policy); err != nil {
		return nil, err
	}
	result.StoredBytes, err = s.repo.UserStoredFileBytes(userID)
	if err != nil {
		return nil, err
	}
	result.StorageLimitBytes = gigabytes(policy.Resource.StoredFileGB)
	return result, nil
}

func (s *Service) PreviewBatchVideoGeneration(userID, projectID string, input BatchVideoSubmitInput) (*BatchVideoSubmissionPreflight, error) {
	prepared, err := s.prepareBatchVideoSubmission(userID, projectID, input)
	if err != nil {
		return nil, err
	}
	return s.preflightBatchVideoSubmission(userID, prepared)
}

func (s *Service) SubmitBatchVideoGeneration(userID, projectID string, input BatchVideoSubmitInput) (int, error) {
	prepared, err := s.prepareBatchVideoSubmission(userID, projectID, input)
	if err != nil {
		return 0, err
	}
	if _, err := s.preflightBatchVideoSubmission(userID, prepared); err != nil {
		return 0, err
	}
	if err := s.repo.CreateBatchVideoJobs(prepared.jobs); err != nil {
		return 0, err
	}
	stage := "standard_validating"
	if prepared.generationMode == "mass" {
		stage = "mass_generating"
	}
	// 提交只落库，不进入调度队列；用户点击开始或定时到点后才释放给 dispatcher。
	if err := s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{
		"stage":              stage,
		"status":             "submitted",
		"scheduled_start_at": nil,
	}); err != nil {
		return 0, err
	}
	return len(prepared.jobs), nil
}

// StartBatchVideoGeneration 释放已提交的任务；scheduledAt 为空表示立即开始。
func (s *Service) StartBatchVideoGeneration(userID, projectID string, scheduledAt *time.Time) (int, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return 0, err
	}
	if project.Status == "paused" {
		return 0, BadAuthRequest("项目已暂停，请先继续调度再开始生成")
	}
	pending, err := s.repo.CountSubmittedBatchVideoJobs(userID, projectID)
	if err != nil {
		return 0, err
	}
	if pending == 0 {
		return 0, BadAuthRequest("没有等待开始的生成任务")
	}
	if scheduledAt != nil {
		value := scheduledAt.UTC()
		if !value.After(time.Now()) {
			return 0, BadAuthRequest("定时开始时间必须晚于当前时间")
		}
		_, started, err := s.repo.StartBatchVideoProject(userID, projectID, &value)
		if err != nil {
			return 0, err
		}
		if !started {
			return 0, BadAuthRequest("项目已暂停，请先继续调度再开始生成")
		}
		return int(pending), nil
	}
	released, started, err := s.repo.StartBatchVideoProject(userID, projectID, nil)
	if err != nil {
		return 0, err
	}
	if !started {
		return 0, BadAuthRequest("项目已暂停，请先继续调度再开始生成")
	}
	return int(released), nil
}

func candidateFromSubmit(model BatchVideoSubmitModel) BatchVideoTaskCandidate {
	return BatchVideoTaskCandidate{
		ModelID:        firstNonEmpty(model.ModelID, model.Model),
		Model:          model.Model,
		LogicalModelID: model.LogicalModelID,
		ChannelModelID: model.ChannelModelID,
		Priority:       batchVideoModelPriority(model.Priority),
		Config:         model.Config,
	}
}

// prepareBatchVideoModels 是批量视频的服务端授权边界：模型必须在线、属于视频能力，
// 且管理员在渠道模型能力里显式开启 batchGeneration；协议、渠道和模型 key 一律由服务端记录重建。
func (s *Service) prepareBatchVideoModels(selected []BatchVideoSubmitModel, creativeCount int) ([]BatchVideoSubmitModel, error) {
	prepared := make([]BatchVideoSubmitModel, 0, len(selected))
	seen := map[string]bool{}
	maxItems := 0
	for _, item := range selected {
		channelID := firstNonEmpty(item.ChannelModelID, item.ModelID)
		if channelID == "" {
			return nil, BadAuthRequest("视频模型缺少渠道模型标识")
		}
		if seen[channelID] {
			return nil, BadAuthRequest("视频模型重复选择")
		}
		seen[channelID] = true
		channelModel, err := s.repo.ChannelModel(channelID)
		if err != nil {
			return nil, BadAuthRequest("视频模型不存在或已下线")
		}
		if !channelModel.Enabled {
			return nil, BadAuthRequest("视频模型已停用：" + firstNonEmpty(channelModel.DisplayName, channelModel.ModelKey))
		}
		if normalizeCapability(channelModel.Capability) != "video" {
			return nil, BadAuthRequest("所选模型不是视频模型：" + firstNonEmpty(channelModel.DisplayName, channelModel.ModelKey))
		}
		config, err := normalizedChannelModelCapability(channelModel, s.protocolRegistry())
		if err != nil || config == nil || config.Video == nil {
			return nil, BadAuthRequest("视频模型能力配置无效：" + firstNonEmpty(channelModel.DisplayName, channelModel.ModelKey))
		}
		if !config.Video.BatchGeneration.Enabled {
			return nil, BadAuthRequest("模型未开启批量视频授权：" + firstNonEmpty(channelModel.DisplayName, channelModel.ModelKey))
		}
		limit := config.Video.BatchGeneration.MaxItems
		if limit <= 0 {
			limit = 20
		}
		if maxItems == 0 || limit < maxItems {
			maxItems = limit
		}
		// 批量任务固定走系统渠道口径：渠道 pin 由服务端补齐，前端只表达本轮选择的模型。
		prepared = append(prepared, BatchVideoSubmitModel{
			ModelID:        channelModel.ID,
			Model:          channelModel.ModelKey,
			ChannelModelID: channelModel.ID,
			Priority:       effectiveModelPriority(config),
			Config: map[string]any{
				"channelId": channelModel.ChannelID,
				"model":     channelModel.ModelKey,
			},
		})
	}
	// 单模型模式的候选顺序和调度队列都按优先级从高到低，用户选择顺序不作数。
	sort.SliceStable(prepared, func(i, j int) bool {
		return prepared[i].Priority > prepared[j].Priority
	})
	if creativeCount > maxItems {
		return nil, BadAuthRequest(fmt.Sprintf("所选模型单次最多提交 %d 条创意，请分批提交", maxItems))
	}
	return prepared, nil
}

// BatchVideoModelOption 是批量视频模块可选的模型；只有管理员显式开启授权才会出现。
type BatchVideoModelOption struct {
	ChannelModelID string `json:"channelModelId"`
	ChannelID      string `json:"channelId"`
	Model          string `json:"model"`
	DisplayName    string `json:"displayName"`
	MaxItems       int    `json:"maxItems"`
	Priority       int    `json:"priority"`
	SortOrder      int    `json:"sortOrder"`
}

func (s *Service) ListBatchVideoModels() ([]BatchVideoModelOption, error) {
	models, err := s.repo.EnabledChannelModelsByCapability("video")
	if err != nil {
		return nil, err
	}
	result := make([]BatchVideoModelOption, 0, len(models))
	for index := range models {
		channelModel := &models[index]
		config, err := normalizedChannelModelCapability(channelModel, s.protocolRegistry())
		if err != nil || config == nil || config.Video == nil || !config.Video.BatchGeneration.Enabled {
			continue
		}
		maxItems := config.Video.BatchGeneration.MaxItems
		if maxItems <= 0 {
			maxItems = 20
		}
		result = append(result, BatchVideoModelOption{
			ChannelModelID: channelModel.ID,
			ChannelID:      channelModel.ChannelID,
			Model:          channelModel.ModelKey,
			DisplayName:    firstNonEmpty(channelModel.DisplayName, channelModel.ModelKey),
			MaxItems:       maxItems,
			Priority:       effectiveModelPriority(config),
			SortOrder:      channelModel.SortOrder,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority > result[j].Priority
		}
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, nil
}

func newBatchVideoJob(project *model.BatchVideoProject, creative model.BatchVideoCreative, mode string, template BatchVideoTaskTemplate) model.BatchVideoGenerationJob {
	modelID := ""
	channelModelID := ""
	logicalModelID := ""
	priority := defaultBatchVideoModelPriority
	if len(template.Candidates) > 0 {
		candidate := template.Candidates[0]
		modelID = candidate.ModelID
		channelModelID = candidate.ChannelModelID
		logicalModelID = candidate.LogicalModelID
		priority = batchVideoModelPriority(candidate.Priority)
	}
	return model.BatchVideoGenerationJob{
		ID:               newID(),
		ProjectID:        project.ID,
		CreativeID:       creative.ID,
		UserID:           project.UserID,
		ModelID:          modelID,
		ChannelModelID:   channelModelID,
		LogicalModelID:   logicalModelID,
		Mode:             mode,
		Status:           "submitted",
		ModelPriority:    priority,
		TaskTemplateJSON: batchVideoMustJSON(template),
	}
}

func (s *Service) RetryBatchVideoJob(userID, projectID, jobID string) (*model.BatchVideoGenerationJob, error) {
	job, err := s.repo.BatchVideoJobForUser(userID, jobID)
	if err != nil || job.ProjectID != projectID {
		return nil, BadAuthRequest("生成任务不存在")
	}
	if job.Status != "failed" && job.Status != "cancelled" {
		return nil, BadAuthRequest("只有失败或已取消的生成任务可以重试")
	}
	template := BatchVideoTaskTemplate{}
	if err := json.Unmarshal([]byte(job.TaskTemplateJSON), &template); err != nil || len(template.Candidates) == 0 {
		return nil, BadAuthRequest("生成任务缺少可用模型配置")
	}
	first := template.Candidates[0]
	currentTaskID := ""
	if job.CurrentTaskID != "" {
		task, err := s.repo.TaskForUser(userID, job.CurrentTaskID)
		if err != nil {
			return nil, BadAuthRequest("原生成任务不存在，无法重试")
		}
		if isContentModerationFailure(task.Error) {
			return nil, BadAuthRequest(contentModerationRetryMessage)
		}
		if job.ModelID == first.ModelID {
			currentTaskID = job.CurrentTaskID
		}
	}
	if err := s.repo.UpdateBatchVideoJob(userID, jobID, map[string]any{
		"status":              "waiting",
		"attempts_used":       0,
		"failover_count":      0,
		"retry_round":         job.RetryRound + 1,
		"model_id":            first.ModelID,
		"channel_model_id":    first.ChannelModelID,
		"logical_model_id":    first.LogicalModelID,
		"model_priority":      batchVideoModelPriority(first.Priority),
		"current_task_id":     currentTaskID,
		"success_resource_id": "",
		"last_error":          "",
	}); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateBatchVideoProjectUnlessPaused(userID, projectID, map[string]any{"status": "running"}); err != nil {
		return nil, err
	}
	return s.repo.BatchVideoJobForUser(userID, jobID)
}

func (s *Service) RetryFailedBatchVideoJobs(userID, projectID string) (int, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return 0, err
	}
	jobs, err := s.repo.BatchVideoJobs(projectID, "failed", 0, 0)
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		template := BatchVideoTaskTemplate{}
		if err := json.Unmarshal([]byte(job.TaskTemplateJSON), &template); err != nil || len(template.Candidates) == 0 {
			return 0, BadAuthRequest("失败任务缺少可用模型配置，请单独处理")
		}
		if job.CurrentTaskID != "" {
			task, err := s.repo.TaskForUser(userID, job.CurrentTaskID)
			if err != nil {
				return 0, BadAuthRequest("失败任务的原生成记录不存在")
			}
			if isContentModerationFailure(task.Error) {
				return 0, BadAuthRequest(contentModerationRetryMessage)
			}
		}
	}
	count := 0
	for _, job := range jobs {
		if _, err := s.RetryBatchVideoJob(userID, projectID, job.ID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Service) ListBatchVideoJobAttempts(userID, projectID, jobID string) ([]model.BatchVideoAttempt, error) {
	job, err := s.repo.BatchVideoJobForUser(userID, jobID)
	if err != nil || job.ProjectID != projectID {
		return nil, BadAuthRequest("生成任务不存在")
	}
	return s.repo.BatchVideoAttempts(jobID)
}

func (s *Service) CancelPendingBatchVideoJobs(userID, projectID string) (int64, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return 0, err
	}
	count, err := s.repo.CancelUnstartedBatchVideoJobs(userID, projectID)
	if err != nil {
		return 0, err
	}
	if err := s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{"scheduled_start_at": nil}); err != nil {
		return 0, err
	}
	active, err := s.repo.CountActiveBatchVideoJobs(userID, projectID)
	if err != nil {
		return 0, err
	}
	if active == 0 {
		_ = s.repo.UpdateBatchVideoProjectUnlessPaused(userID, projectID, map[string]any{"status": "ready"})
	}
	return count, nil
}

func (s *Service) PauseBatchVideoProject(userID, projectID string) error {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return err
	}
	return s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{"status": "paused"})
}

func (s *Service) ResumeBatchVideoProject(userID, projectID string) error {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return err
	}
	return s.repo.UpdateBatchVideoProject(userID, projectID, map[string]any{"status": "running"})
}

func (s *Service) BatchVideoStats(userID, projectID string) (BatchVideoProjectCounts, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return BatchVideoProjectCounts{}, err
	}
	counts, err := s.repo.BatchVideoCounts(projectID)
	return BatchVideoProjectCounts(counts), err
}

func (s *Service) DeleteBatchVideoProject(userID, projectID string) error {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return err
	}
	activeJobs, err := s.repo.CountActiveBatchVideoJobs(userID, projectID)
	if err != nil {
		return err
	}
	activeRounds, err := s.repo.CountActiveBatchVideoRounds(userID, projectID)
	if err != nil {
		return err
	}
	analysisRunning := batchVideoAnalysisActive(project.AnalysisStatus)
	if !analysisRunning && project.AnalysisTaskID != "" {
		if task, err := s.repo.TaskForUser(userID, project.AnalysisTaskID); err == nil &&
			(task.Status == model.TaskStatusQueued || task.Status == model.TaskStatusRunning) {
			analysisRunning = true
		}
	}
	generatingAssets, err := s.repo.BatchVideoReferenceAssets(projectID, "generating")
	if err != nil {
		return err
	}
	if activeJobs > 0 || activeRounds > 0 || analysisRunning || len(generatingAssets) > 0 {
		return BadAuthRequest("项目仍有进行中的分析、策划、参考图生成或视频生成任务；请先取消未开始的任务，待运行中的任务结束后再删除")
	}
	// 先收集资源，级联删除业务记录后再清理文件；仍被其他项目引用的资源会被保留。
	resourceIDs := s.batchVideoProjectResourceIDs(projectID)
	if err := s.repo.DeleteBatchVideoProject(userID, projectID); err != nil {
		return err
	}
	if err := s.cleanupBatchVideoResources(userID, resourceIDs); err != nil {
		_ = s.log(userID, "", "warn", "删除项目后清理素材文件失败", err.Error())
	}
	return nil
}

func (s *Service) RunBatchVideoDispatcher(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.ProcessBatchVideoQueue(ctx)
		}
	}
}

func (s *Service) ProcessBatchVideoQueue(ctx context.Context) error {
	_, _ = s.repo.RecoverStaleBatchVideoJobs(time.Now().Add(-2 * time.Minute))
	if err := s.reconcileBatchVideoAnalysis(); err != nil {
		return err
	}
	if err := s.reconcileBatchVideoPlanning(); err != nil {
		return err
	}
	if err := s.reconcileBatchVideoReferenceGeneration(); err != nil {
		return err
	}
	if err := s.releaseScheduledBatchVideoGeneration(); err != nil {
		return err
	}
	if err := s.dispatchWaitingBatchVideoJobs(ctx); err != nil {
		return err
	}
	return s.reconcileBatchVideoJobs(ctx)
}

func (s *Service) releaseScheduledBatchVideoGeneration() error {
	projects, err := s.repo.DueScheduledBatchVideoProjects(time.Now().UTC())
	if err != nil {
		return err
	}
	for _, project := range projects {
		if _, err := s.repo.ReleaseScheduledBatchVideoProject(project.UserID, project.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) dispatchWaitingBatchVideoJobs(ctx context.Context) error {
	jobs, err := s.repo.WaitingBatchVideoJobs(50)
	if err != nil {
		return err
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		project, err := s.repo.BatchVideoProjectForUser(job.UserID, job.ProjectID)
		if err != nil || project.Status == "paused" {
			continue
		}
		active, err := s.repo.ActiveTaskCountForUser(job.UserID)
		if err != nil || active >= int64(policy.Task.ActiveTaskLimit) {
			continue
		}
		claimed, err := s.repo.ClaimBatchVideoJob(job.UserID, job.ID)
		if err != nil || !claimed {
			continue
		}
		template := BatchVideoTaskTemplate{}
		if err := json.Unmarshal([]byte(job.TaskTemplateJSON), &template); err != nil || len(template.Candidates) == 0 {
			_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "failed", "last_error": "生成配置缺失"})
			continue
		}
		candidateIndex := job.FailoverCount
		if candidateIndex < 0 || candidateIndex >= len(template.Candidates) {
			_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "failed", "last_error": "候选模型已全部用尽"})
			continue
		}
		candidate := template.Candidates[candidateIndex]
		var task *model.Task
		if job.CurrentTaskID != "" {
			task, err = s.RetryTask(job.UserID, job.CurrentTaskID)
		} else {
			request := batchVideoTaskRequest(template, candidate, job)
			task, err = s.CreateTask(job.UserID, request)
		}
		if err != nil {
			message := err.Error()
			var appErr *AppError
			if errors.Is(err, repository.ErrActiveTaskLimit) || strings.Contains(message, "同时排队或运行的任务最多") || (errors.As(err, &appErr) && appErr.Retryable) {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "waiting"})
				continue
			}
			if job.CurrentTaskID == "" && GetModelErrorCode(err) == ErrCodeModelRouteUnavailable && candidateIndex+1 < len(template.Candidates) {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{
					"status":           "waiting",
					"current_task_id":  "",
					"failover_count":   candidateIndex + 1,
					"model_id":         template.Candidates[candidateIndex+1].ModelID,
					"channel_model_id": template.Candidates[candidateIndex+1].ChannelModelID,
					"logical_model_id": template.Candidates[candidateIndex+1].LogicalModelID,
					"model_priority":   batchVideoModelPriority(template.Candidates[candidateIndex+1].Priority),
					"last_error":       message,
				})
			} else {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "failed", "last_error": message})
			}
			continue
		}
		startedAt := time.Now()
		switchedFrom := ""
		if candidateIndex > 0 {
			switchedFrom = template.Candidates[candidateIndex-1].ModelID
		}
		attempt := &model.BatchVideoAttempt{
			ID:                  newID(),
			ProjectID:           job.ProjectID,
			CreativeID:          job.CreativeID,
			JobID:               job.ID,
			UserID:              job.UserID,
			RetryRound:          job.RetryRound,
			ModelID:             candidate.ModelID,
			TaskID:              task.ID,
			Status:              "queued",
			SwitchedFromModelID: switchedFrom,
			StartedAt:           &startedAt,
		}
		if err := s.repo.AttachBatchVideoTask(job.UserID, job.ID, attempt, map[string]any{
			"status":           "queued",
			"current_task_id":  task.ID,
			"model_id":         candidate.ModelID,
			"channel_model_id": candidate.ChannelModelID,
			"logical_model_id": candidate.LogicalModelID,
			"attempts_used":    job.AttemptsUsed + 1,
			"last_error":       "",
		}); err != nil {
			_, _ = s.CancelTask(ctx, job.UserID, task.ID)
			_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "failed", "last_error": "生成任务与批次记录关联失败"})
			continue
		}
	}
	return nil
}

func batchVideoTaskRequest(template BatchVideoTaskTemplate, candidate BatchVideoTaskCandidate, job model.BatchVideoGenerationJob) CreateTaskRequest {
	config := map[string]any{}
	for key, value := range candidate.Config {
		config[key] = value
	}
	if template.DurationSeconds > 0 {
		config["videoSeconds"] = fmt.Sprint(template.DurationSeconds)
	}
	if template.AspectRatio != "" {
		config["size"] = template.AspectRatio
	}
	input := map[string]any{
		"mode":            "video",
		"prompt":          template.Prompt,
		"config":          config,
		"referenceImages": template.ReferenceImages,
		"metadata": map[string]any{
			"source":              "batch-video",
			"batchVideoProjectId": template.ProjectID,
			"creativeId":          template.CreativeID,
			"jobId":               job.ID,
			"storagePolicy":       BatchVideoLocalStoragePolicy,
		},
	}
	operation := template.Operation
	if operation == "" {
		operation = "text_to_video"
	}
	return CreateTaskRequest{
		Type:           "canvas_video",
		Operation:      operation,
		Prompt:         template.Prompt,
		Model:          candidate.Model,
		LogicalModelID: candidate.LogicalModelID,
		Input:          input,
	}
}

func batchVideoAutomaticRetryAllowed(task *model.Task) bool {
	if task == nil || task.Status == model.TaskStatusCancelled || isContentModerationFailure(task.Error) {
		return false
	}
	message := strings.ToLower(task.Error)
	permanentMarkers := []string{
		"积分不足", "余额不足", "参数错误", "参数不", "资源不存在", "资源缺失", "资源尚未就绪", "参考图资源",
		"insufficient", "invalid_parameter", "invalid parameter", "resource not found", "quota_exceeded",
	}
	for _, marker := range permanentMarkers {
		if strings.Contains(message, marker) {
			return false
		}
	}
	return true
}

func (s *Service) reconcileBatchVideoJobs(ctx context.Context) error {
	jobs, err := s.repo.ReconcileBatchVideoJobs(100)
	if err != nil {
		return err
	}
	settings, err := s.BatchVideoSettings()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if job.CurrentTaskID == "" {
			_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "waiting"})
			continue
		}
		task, err := s.repo.TaskForUser(job.UserID, job.CurrentTaskID)
		if err != nil {
			continue
		}
		switch task.Status {
		case model.TaskStatusSucceeded:
			attempts, _ := s.repo.BatchVideoAttempts(job.ID)
			for _, attempt := range attempts {
				if attempt.TaskID == task.ID && attempt.Status != "succeeded" {
					finished := time.Now()
					_ = s.repo.UpdateBatchVideoAttempt(attempt.ID, map[string]any{"status": "succeeded", "finished_at": &finished})
				}
			}
			_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "succeeded", "last_error": "", "success_resource_id": taskResultResourceID(s, task.ID)})
		case model.TaskStatusFailed, model.TaskStatusCancelled:
			errorText := task.Error
			attemptStatus := "failed"
			jobStatus := "failed"
			if task.Status == model.TaskStatusCancelled {
				attemptStatus = "cancelled"
				jobStatus = "cancelled"
				errorText = firstNonEmpty(errorText, "任务已取消")
			}
			attempts, _ := s.repo.BatchVideoAttempts(job.ID)
			for _, attempt := range attempts {
				if attempt.TaskID == task.ID && attempt.Status != "succeeded" && attempt.Status != "failed" && attempt.Status != "cancelled" {
					finished := time.Now()
					_ = s.repo.UpdateBatchVideoAttempt(attempt.ID, map[string]any{"status": attemptStatus, "error": errorText, "finished_at": &finished})
				}
			}
			if !batchVideoAutomaticRetryAllowed(task) {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": jobStatus, "last_error": errorText})
				continue
			}
			sameModelAttempts := 0
			for _, attempt := range attempts {
				if attempt.ModelID == job.ModelID && attempt.RetryRound == job.RetryRound {
					sameModelAttempts++
				}
			}
			template := BatchVideoTaskTemplate{}
			_ = json.Unmarshal([]byte(job.TaskTemplateJSON), &template)
			if sameModelAttempts < settings.MaxAttemptsPerModel {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "waiting", "current_task_id": task.ID, "last_error": errorText})
				continue
			}
			next := job.FailoverCount + 1
			if next < len(template.Candidates) {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{
					"status":           "waiting",
					"current_task_id":  "",
					"failover_count":   next,
					"model_id":         template.Candidates[next].ModelID,
					"channel_model_id": template.Candidates[next].ChannelModelID,
					"logical_model_id": template.Candidates[next].LogicalModelID,
					"model_priority":   batchVideoModelPriority(template.Candidates[next].Priority),
					"last_error":       errorText,
				})
			} else {
				_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": "failed", "current_task_id": task.ID, "last_error": errorText})
			}
		case model.TaskStatusQueued, model.TaskStatusRunning:
			_ = s.repo.UpdateBatchVideoJob(job.UserID, job.ID, map[string]any{"status": string(task.Status)})
		}
	}
	return nil
}

func taskResultResourceID(s *Service, taskID string) string {
	results, err := s.repo.TaskResults(taskID)
	if err != nil {
		return ""
	}
	for _, result := range results {
		if strings.Contains(result.URL, "/api/resources/") {
			parts := strings.Split(result.URL, "/api/resources/")
			if len(parts) > 1 {
				id := strings.Split(parts[1], "/")[0]
				if id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func (s *Service) startBatchVideoDispatcher(ctx context.Context) {
	go s.RunBatchVideoDispatcher(ctx)
}
