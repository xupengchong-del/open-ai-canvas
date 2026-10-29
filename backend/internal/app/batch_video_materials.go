package app

import (
	"encoding/json"
	"strings"

	"yingce/backend/internal/model"
)

// ensureBatchVideoMaterialsIdle 只允许在没有分析、策划、生成或参考图任务时改动素材，
// 避免删除素材后任务仍读取已消失的文件。
func (s *Service) ensureBatchVideoMaterialsIdle(userID, projectID string) (*model.BatchVideoProject, error) {
	project, err := s.repo.BatchVideoProjectForUser(userID, projectID)
	if err != nil {
		return nil, err
	}
	analysisRunning := batchVideoAnalysisActive(project.AnalysisStatus)
	if !analysisRunning && project.AnalysisTaskID != "" {
		if task, err := s.repo.TaskForUser(userID, project.AnalysisTaskID); err == nil &&
			(task.Status == model.TaskStatusQueued || task.Status == model.TaskStatusRunning) {
			analysisRunning = true
		}
	}
	if analysisRunning {
		return nil, BadAuthRequest("分析进行中，请先中止分析后再调整素材")
	}
	activeRounds, err := s.repo.CountActiveBatchVideoRounds(userID, projectID)
	if err != nil {
		return nil, err
	}
	activeJobs, err := s.repo.CountActiveBatchVideoJobs(userID, projectID)
	if err != nil {
		return nil, err
	}
	submittedJobs, err := s.repo.CountSubmittedBatchVideoJobs(userID, projectID)
	if err != nil {
		return nil, err
	}
	if activeRounds > 0 || activeJobs > 0 || submittedJobs > 0 {
		return nil, BadAuthRequest("项目仍有进行中的策划/生成任务或已提交待开始的任务，请先中止或取消后再调整素材")
	}
	assets, err := s.repo.BatchVideoReferenceAssets(projectID, "generating")
	if err != nil {
		return nil, err
	}
	if len(assets) > 0 {
		return nil, BadAuthRequest("参考图仍在生成，请稍后再调整素材")
	}
	return project, nil
}

func batchVideoMaterialResourceIDs(batches []model.BatchVideoMaterialBatch) []string {
	ids := make([]string, 0)
	for _, batch := range batches {
		var payload batchVideoMaterialJSON
		if err := json.Unmarshal([]byte(batch.MaterialJSON), &payload); err != nil {
			continue
		}
		for _, material := range payload.Materials {
			if material.ResourceID != "" {
				ids = append(ids, material.ResourceID)
			}
		}
	}
	return batchVideoUniqueIDs(ids)
}

func (s *Service) batchVideoProjectResourceIDs(projectID string) []string {
	ids := make([]string, 0)
	if batches, err := s.repo.BatchVideoMaterialBatches(projectID); err == nil {
		ids = append(ids, batchVideoMaterialResourceIDs(batches)...)
	}
	if assets, err := s.repo.BatchVideoReferenceAssets(projectID, ""); err == nil {
		for _, asset := range assets {
			if asset.ResourceID != "" {
				ids = append(ids, asset.ResourceID)
			}
		}
	}
	if jobs, err := s.repo.BatchVideoJobs(projectID, "", 0, 0); err == nil {
		for _, job := range jobs {
			if job.SuccessResourceID != "" {
				ids = append(ids, job.SuccessResourceID)
			}
		}
	}
	return batchVideoUniqueIDs(ids)
}

func batchVideoUniqueIDs(ids []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// cleanupBatchVideoResources 删除已解除引用的资源记录和物理文件。
// 已完成的任务、日志和结果只作为历史保留，不再保护它们读过的素材；
// 排队或运行中的任务仍会阻止清理。其他项目、画布和业务记录的引用也会保留资源。
// DiscardBatchVideoUploads 回收上传弹窗中尚未确认的素材；仍被任何业务记录或物理文件共享的资源会保留。
func (s *Service) DiscardBatchVideoUploads(userID string, resourceIDs []string) (int, error) {
	ids := batchVideoUniqueIDs(resourceIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > 100 {
		return 0, BadAuthRequest("一次最多清理 100 个上传文件")
	}
	if err := s.cleanupBatchVideoResources(userID, ids); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (s *Service) cleanupBatchVideoResources(userID string, resourceIDs []string) error {
	ids := batchVideoUniqueIDs(resourceIDs)
	if len(ids) == 0 {
		return nil
	}
	resources, err := s.repo.ResourcesForUserIDs(userID, ids)
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return nil
	}
	owned := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		owned[resource.ID] = struct{}{}
	}
	snapshot, err := s.repo.ResourceReferenceSnapshotExcludingAssets(userID, nil, ids)
	if err != nil {
		return err
	}
	referenced := map[string]bool{}
	for _, direct := range snapshot.Direct {
		if _, ok := owned[direct.ResourceID]; ok {
			referenced[direct.ResourceID] = true
		}
	}
	for _, document := range snapshot.Documents {
		if document.TaskStatus == model.TaskStatusSucceeded || document.TaskStatus == model.TaskStatusFailed || document.TaskStatus == model.TaskStatusCancelled {
			// 任务日志和结果只作历史；已完成任务的输入仍需区分：批量视频自己的任务
			// 不应保护已解除引用的素材，其他功能的任务结果可能仍是资源的唯一引用。
			if document.Kind == "任务日志" || document.Kind == "任务结果" {
				continue
			}
			if document.Kind == "任务" && batchVideoTaskInput(document.PrimaryJSON) {
				continue
			}
		}
		for id := range documentReferencedResourceIDs(document.PrimaryJSON, owned) {
			referenced[id] = true
		}
	}
	candidates := make([]model.Resource, 0, len(resources))
	for _, resource := range resources {
		if !referenced[resource.ID] {
			candidates = append(candidates, resource)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	physical := map[string]*model.Resource{}
	for index := range candidates {
		resource := &candidates[index]
		shared, err := s.repo.ResourceStorageReferenceCount(resource, ids)
		if err != nil {
			return err
		}
		if shared == 0 {
			physical[resourceStorageIdentity(resource)] = resource
		}
	}
	jobs := resourceDeletionJobs(userID, physical)
	if err := s.repo.DeleteDetachedResources(candidates, jobs); err != nil {
		return err
	}
	if len(jobs) > 0 {
		s.runWorkerTask(func() { s.drainResourceDeletionJobs(len(jobs)) })
	}
	return nil
}

// batchVideoTaskInput 通过任务输入里的项目标记识别批量视频任务。
// 已完成的其他功能任务仍可能只靠 result_json 指向资源，不能一并忽略。
func batchVideoTaskInput(inputJSON string) bool {
	var payload struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(inputJSON), &payload); err != nil {
		return false
	}
	value, _ := payload.Metadata["batchVideoProjectId"].(string)
	return strings.TrimSpace(value) != ""
}

func (s *Service) markBatchVideoMaterialsChanged(userID, projectID string, project *model.BatchVideoProject, assetRemoved bool) error {
	data := map[string]any{"analysis_status": "stale", "analysis_current_asset_id": ""}
	if assetRemoved {
		data["reference_pool_version"] = project.ReferencePoolVersion + 1
	}
	return s.repo.UpdateBatchVideoProject(userID, projectID, data)
}

func (s *Service) DeleteBatchVideoMaterial(userID, projectID, batchID, resourceID string) (*BatchVideoProjectDetail, error) {
	project, err := s.ensureBatchVideoMaterialsIdle(userID, projectID)
	if err != nil {
		return nil, err
	}
	batch, err := s.repo.BatchVideoMaterialBatchForUser(userID, projectID, batchID)
	if err != nil {
		return nil, BadAuthRequest("素材批次不存在")
	}
	var payload batchVideoMaterialJSON
	if err := json.Unmarshal([]byte(batch.MaterialJSON), &payload); err != nil {
		return nil, BadAuthRequest("素材批次数据无效，无法删除")
	}
	index := -1
	for i, material := range payload.Materials {
		if material.ResourceID == resourceID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, BadAuthRequest("素材不存在或已被删除")
	}
	asset, _ := s.repo.BatchVideoReferenceAssetForResource(userID, projectID, resourceID)
	if asset != nil {
		references, err := s.repo.CountBatchVideoCreativeReferencesForAsset(userID, projectID, asset.ID)
		if err != nil {
			return nil, err
		}
		if references > 0 {
			return nil, BadAuthRequest("该素材已被创意引用，请先删除相关创意后再删除素材")
		}
	}
	payload.Materials = append(payload.Materials[:index], payload.Materials[index+1:]...)
	if len(payload.Materials) == 0 && len(payload.Documents) == 0 {
		if err := s.repo.DeleteBatchVideoMaterialBatch(userID, batch.ID); err != nil {
			return nil, err
		}
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		batch.MaterialJSON = string(encoded)
		if err := s.repo.SaveBatchVideoMaterialBatch(batch); err != nil {
			return nil, err
		}
	}
	remainingBatches, err := s.repo.BatchVideoMaterialBatches(projectID)
	if err != nil {
		return nil, err
	}
	remaining := batchVideoMaterialResourceIDs(remainingBatches)
	resourceIDs := []string{}
	if !containsBatchVideoID(remaining, resourceID) {
		resourceIDs = append(resourceIDs, resourceID)
		if asset != nil {
			if err := s.repo.DeleteBatchVideoReferenceAsset(userID, asset.ID); err != nil {
				return nil, err
			}
		}
	}
	if err := s.markBatchVideoMaterialsChanged(userID, projectID, project, asset != nil && len(resourceIDs) > 0); err != nil {
		return nil, err
	}
	if err := s.cleanupBatchVideoResources(userID, resourceIDs); err != nil {
		_ = s.log(userID, "", "warn", "删除素材后清理文件失败", err.Error())
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

func (s *Service) DeleteBatchVideoMaterialBatch(userID, projectID, batchID string) (*BatchVideoProjectDetail, error) {
	project, err := s.ensureBatchVideoMaterialsIdle(userID, projectID)
	if err != nil {
		return nil, err
	}
	batch, err := s.repo.BatchVideoMaterialBatchForUser(userID, projectID, batchID)
	if err != nil {
		return nil, BadAuthRequest("素材批次不存在")
	}
	var payload batchVideoMaterialJSON
	if err := json.Unmarshal([]byte(batch.MaterialJSON), &payload); err != nil {
		return nil, BadAuthRequest("素材批次数据无效，无法删除")
	}
	batchResourceIDs := make([]string, 0, len(payload.Materials))
	for _, material := range payload.Materials {
		if material.ResourceID != "" {
			batchResourceIDs = append(batchResourceIDs, material.ResourceID)
		}
	}
	if err := s.deleteBatchVideoMaterialBatchWithAssets(userID, projectID, project, batch, batchResourceIDs); err != nil {
		return nil, err
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

func (s *Service) ClearBatchVideoMaterials(userID, projectID string) (*BatchVideoProjectDetail, error) {
	project, err := s.ensureBatchVideoMaterialsIdle(userID, projectID)
	if err != nil {
		return nil, err
	}
	batches, err := s.repo.BatchVideoMaterialBatches(projectID)
	if err != nil {
		return nil, err
	}
	if len(batches) == 0 {
		return nil, BadAuthRequest("项目还没有素材")
	}
	resourceIDs := batchVideoMaterialResourceIDs(batches)
	assets, err := s.repo.BatchVideoReferenceAssets(projectID, "")
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		if asset.ResourceID != "" {
			resourceIDs = append(resourceIDs, asset.ResourceID)
		}
	}
	resourceIDs = batchVideoUniqueIDs(resourceIDs)
	for _, asset := range assets {
		references, err := s.repo.CountBatchVideoCreativeReferencesForAsset(userID, projectID, asset.ID)
		if err != nil {
			return nil, err
		}
		if references > 0 {
			return nil, BadAuthRequest("部分素材已被创意引用，请先删除相关创意后再清空素材")
		}
	}
	if err := s.repo.DeleteBatchVideoMaterialBatches(userID, projectID); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteBatchVideoReferenceAssets(userID, projectID); err != nil {
		return nil, err
	}
	if err := s.markBatchVideoMaterialsChanged(userID, projectID, project, len(resourceIDs) > 0); err != nil {
		return nil, err
	}
	if err := s.cleanupBatchVideoResources(userID, resourceIDs); err != nil {
		_ = s.log(userID, "", "warn", "清空素材后清理文件失败", err.Error())
	}
	return s.BatchVideoProjectDetail(userID, projectID)
}

// deleteBatchVideoMaterialBatchWithAssets 先校验创意引用，再删除批次；
// 同一资源仍被其他批次使用时保留参考图资产和文件。
func (s *Service) deleteBatchVideoMaterialBatchWithAssets(userID, projectID string, project *model.BatchVideoProject, batch *model.BatchVideoMaterialBatch, batchResourceIDs []string) error {
	batches, err := s.repo.BatchVideoMaterialBatches(projectID)
	if err != nil {
		return err
	}
	remaining := make([]model.BatchVideoMaterialBatch, 0, len(batches))
	for _, item := range batches {
		if item.ID != batch.ID {
			remaining = append(remaining, item)
		}
	}
	remainingIDs := batchVideoMaterialResourceIDs(remaining)
	removed := make([]string, 0, len(batchResourceIDs))
	for _, resourceID := range batchVideoResourceIDsNotIn(batchResourceIDs, remainingIDs) {
		asset, _ := s.repo.BatchVideoReferenceAssetForResource(userID, projectID, resourceID)
		if asset != nil {
			references, err := s.repo.CountBatchVideoCreativeReferencesForAsset(userID, projectID, asset.ID)
			if err != nil {
				return err
			}
			if references > 0 {
				return BadAuthRequest("该批次有素材已被创意引用，请先删除相关创意后再删除批次")
			}
		}
		removed = append(removed, resourceID)
	}
	if err := s.repo.DeleteBatchVideoMaterialBatch(userID, batch.ID); err != nil {
		return err
	}
	for _, resourceID := range removed {
		if asset, _ := s.repo.BatchVideoReferenceAssetForResource(userID, projectID, resourceID); asset != nil {
			if err := s.repo.DeleteBatchVideoReferenceAsset(userID, asset.ID); err != nil {
				return err
			}
		}
	}
	if err := s.markBatchVideoMaterialsChanged(userID, projectID, project, len(removed) > 0); err != nil {
		return err
	}
	if err := s.cleanupBatchVideoResources(userID, removed); err != nil {
		_ = s.log(userID, "", "warn", "删除素材批次后清理文件失败", err.Error())
	}
	return nil
}

func batchVideoResourceIDsNotIn(values, other []string) []string {
	otherSet := map[string]bool{}
	for _, value := range other {
		otherSet[value] = true
	}
	result := make([]string, 0, len(values))
	for _, value := range batchVideoUniqueIDs(values) {
		if !otherSet[value] {
			result = append(result, value)
		}
	}
	return result
}

func containsBatchVideoID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
