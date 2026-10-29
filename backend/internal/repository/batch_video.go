package repository

import (
	"time"

	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// 批量视频仓储只做持久化与条件更新；业务状态机、模型选择和调度策略留在 app 层。

func (r *Repository) CreateBatchVideoProject(project *model.BatchVideoProject) error {
	return r.db.Create(project).Error
}

func (r *Repository) BatchVideoProjectForUser(userID, id string) (*model.BatchVideoProject, error) {
	var project model.BatchVideoProject
	if err := r.db.First(&project, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *Repository) BatchVideoProjects(userID string, status string, limit, offset int) ([]model.BatchVideoProject, error) {
	var projects []model.BatchVideoProject
	query := r.db.Where("user_id = ?", userID)
	if status != "" {
		query = query.Where("stage = ?", status)
	}
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	err := query.Order("created_at desc").Find(&projects).Error
	return projects, err
}

func (r *Repository) SaveBatchVideoProject(project *model.BatchVideoProject) error {
	project.UpdatedAt = time.Now()
	return r.db.Save(project).Error
}

func (r *Repository) UpdateBatchVideoProject(userID, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now()
	return r.db.Model(&model.BatchVideoProject{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error
}

// UpdateBatchVideoProjectUnlessPaused 用于调度/取消/重试等后台状态迁移，
// 避免用户刚暂停后又被一次迟到的状态更新改回 running/ready。
func (r *Repository) UpdateBatchVideoProjectUnlessPaused(userID, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now()
	return r.db.Model(&model.BatchVideoProject{}).
		Where("id = ? AND user_id = ? AND status <> ?", id, userID, "paused").
		Updates(updates).Error
}

// DeleteBatchVideoProject 删除项目及其全部从属记录；调用方需先确认没有进行中的分析、策划或生成任务。
func (r *Repository) DeleteBatchVideoProject(userID, projectID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var creativeIDs []string
		if err := tx.Model(&model.BatchVideoCreative{}).Where("project_id = ? AND user_id = ?", projectID, userID).Pluck("id", &creativeIDs).Error; err != nil {
			return err
		}
		if len(creativeIDs) > 0 {
			if err := tx.Where("creative_id IN ?", creativeIDs).Delete(&model.BatchVideoCreativeReference{}).Error; err != nil {
				return err
			}
		}
		for _, item := range []any{
			&model.BatchVideoAttempt{},
			&model.BatchVideoGenerationJob{},
			&model.BatchVideoCreative{},
			&model.BatchVideoCreativeRound{},
			&model.BatchVideoReferenceAsset{},
			&model.BatchVideoSellingPoint{},
			&model.BatchVideoMaterialBatch{},
			&model.BatchVideoStandardValidation{},
		} {
			if err := tx.Where("project_id = ? AND user_id = ?", projectID, userID).Delete(item).Error; err != nil {
				return err
			}
		}
		result := tx.Where("id = ? AND user_id = ?", projectID, userID).Delete(&model.BatchVideoProject{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *Repository) CountActiveBatchVideoJobs(userID, projectID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.BatchVideoGenerationJob{}).
		Where("user_id = ? AND project_id = ? AND status IN ?", userID, projectID, []string{"waiting", "dispatching", "queued", "running"}).
		Count(&count).Error
	return count, err
}

func (r *Repository) CountSubmittedBatchVideoJobs(userID, projectID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.BatchVideoGenerationJob{}).
		Where("user_id = ? AND project_id = ? AND status = ?", userID, projectID, "submitted").
		Count(&count).Error
	return count, err
}

func (r *Repository) CountActiveBatchVideoRounds(userID, projectID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.BatchVideoCreativeRound{}).
		Where("user_id = ? AND project_id = ? AND status IN ?", userID, projectID, []string{"queued", "generating", "running"}).
		Count(&count).Error
	return count, err
}

func (r *Repository) CreateBatchVideoMaterialBatch(batch *model.BatchVideoMaterialBatch) error {
	return r.db.Create(batch).Error
}

func (r *Repository) BatchVideoMaterialBatches(projectID string) ([]model.BatchVideoMaterialBatch, error) {
	var batches []model.BatchVideoMaterialBatch
	err := r.db.Where("project_id = ?", projectID).Order("batch_no asc").Find(&batches).Error
	return batches, err
}

func (r *Repository) NextBatchVideoMaterialBatchNo(projectID string) (int, error) {
	var value int
	err := r.db.Model(&model.BatchVideoMaterialBatch{}).Where("project_id = ?", projectID).Select("COALESCE(MAX(batch_no), 0)").Scan(&value).Error
	return value + 1, err
}

func (r *Repository) BatchVideoMaterialBatchForUser(userID, projectID, batchID string) (*model.BatchVideoMaterialBatch, error) {
	var batch model.BatchVideoMaterialBatch
	if err := r.db.Where("id = ? AND project_id = ? AND user_id = ?", batchID, projectID, userID).First(&batch).Error; err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *Repository) SaveBatchVideoMaterialBatch(batch *model.BatchVideoMaterialBatch) error {
	return r.db.Save(batch).Error
}

func (r *Repository) DeleteBatchVideoMaterialBatch(userID, batchID string) error {
	return r.db.Where("id = ? AND user_id = ?", batchID, userID).Delete(&model.BatchVideoMaterialBatch{}).Error
}

func (r *Repository) DeleteBatchVideoMaterialBatches(userID, projectID string) error {
	return r.db.Where("user_id = ? AND project_id = ?", userID, projectID).Delete(&model.BatchVideoMaterialBatch{}).Error
}

func (r *Repository) CreateBatchVideoSellingPoints(points []model.BatchVideoSellingPoint) error {
	if len(points) == 0 {
		return nil
	}
	return r.db.Create(&points).Error
}

func (r *Repository) BatchVideoSellingPoints(projectID string) ([]model.BatchVideoSellingPoint, error) {
	var points []model.BatchVideoSellingPoint
	err := r.db.Where("project_id = ?", projectID).Order("created_at asc").Find(&points).Error
	return points, err
}

func (r *Repository) CreateBatchVideoReferenceAsset(asset *model.BatchVideoReferenceAsset) error {
	return r.db.Create(asset).Error
}

func (r *Repository) BatchVideoReferenceAssets(projectID string, status string) ([]model.BatchVideoReferenceAsset, error) {
	var assets []model.BatchVideoReferenceAsset
	query := r.db.Where("project_id = ?", projectID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("created_at asc").Find(&assets).Error
	return assets, err
}

func (r *Repository) BatchVideoReferenceAssetForUser(userID, id string) (*model.BatchVideoReferenceAsset, error) {
	var asset model.BatchVideoReferenceAsset
	if err := r.db.First(&asset, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *Repository) SaveBatchVideoReferenceAsset(asset *model.BatchVideoReferenceAsset) error {
	asset.UpdatedAt = time.Now()
	return r.db.Save(asset).Error
}

func (r *Repository) BatchVideoReferenceAssetByHash(projectID, hash string) (*model.BatchVideoReferenceAsset, error) {
	if hash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var asset model.BatchVideoReferenceAsset
	if err := r.db.First(&asset, "project_id = ? AND content_hash = ?", projectID, hash).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *Repository) BatchVideoReferenceAssetForResource(userID, projectID, resourceID string) (*model.BatchVideoReferenceAsset, error) {
	var asset model.BatchVideoReferenceAsset
	if err := r.db.Where("user_id = ? AND project_id = ? AND resource_id = ?", userID, projectID, resourceID).Order("created_at asc").First(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *Repository) DeleteBatchVideoReferenceAsset(userID, id string) error {
	return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.BatchVideoReferenceAsset{}).Error
}

func (r *Repository) DeleteBatchVideoReferenceAssets(userID, projectID string) error {
	return r.db.Where("user_id = ? AND project_id = ?", userID, projectID).Delete(&model.BatchVideoReferenceAsset{}).Error
}

func (r *Repository) CountBatchVideoCreativeReferencesForAsset(userID, projectID, assetID string) (int64, error) {
	var count int64
	err := r.db.Table("batch_video_creative_references").
		Joins("JOIN batch_video_creatives ON batch_video_creatives.id = batch_video_creative_references.creative_id").
		Where("batch_video_creatives.user_id = ? AND batch_video_creatives.project_id = ? AND batch_video_creative_references.reference_asset_id = ?", userID, projectID, assetID).
		Count(&count).Error
	return count, err
}

func (r *Repository) NextBatchVideoRoundNo(projectID string) (int, error) {
	var value int
	err := r.db.Model(&model.BatchVideoCreativeRound{}).Where("project_id = ?", projectID).Select("COALESCE(MAX(round_no), 0)").Scan(&value).Error
	return value + 1, err
}

func (r *Repository) CreateBatchVideoCreativeRound(round *model.BatchVideoCreativeRound) error {
	return r.db.Create(round).Error
}

func (r *Repository) BatchVideoCreativeRounds(projectID string) ([]model.BatchVideoCreativeRound, error) {
	var rounds []model.BatchVideoCreativeRound
	err := r.db.Where("project_id = ?", projectID).Order("round_no asc").Find(&rounds).Error
	return rounds, err
}

func (r *Repository) CreateBatchVideoCreatives(creatives []model.BatchVideoCreative) error {
	if len(creatives) == 0 {
		return nil
	}
	return r.db.CreateInBatches(&creatives, 200).Error
}

type BatchVideoCreativeFilter struct {
	ProjectID    string
	RoundID      string
	ReviewStatus string
	MatchStatus  string
	Limit        int
	Offset       int
}

func (r *Repository) BatchVideoCreatives(filter BatchVideoCreativeFilter) ([]model.BatchVideoCreative, error) {
	var creatives []model.BatchVideoCreative
	query := r.db.Where("project_id = ?", filter.ProjectID)
	if filter.RoundID != "" {
		query = query.Where("round_id = ?", filter.RoundID)
	}
	if filter.ReviewStatus != "" {
		query = query.Where("review_status = ?", filter.ReviewStatus)
	}
	if filter.MatchStatus != "" {
		query = query.Where("match_status = ?", filter.MatchStatus)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}
	err := query.Order("round_id asc, seq asc").Find(&creatives).Error
	return creatives, err
}

func (r *Repository) CountBatchVideoCreatives(filter BatchVideoCreativeFilter) (int64, error) {
	var count int64
	query := r.db.Model(&model.BatchVideoCreative{}).Where("project_id = ?", filter.ProjectID)
	if filter.RoundID != "" {
		query = query.Where("round_id = ?", filter.RoundID)
	}
	if filter.ReviewStatus != "" {
		query = query.Where("review_status = ?", filter.ReviewStatus)
	}
	if filter.MatchStatus != "" {
		query = query.Where("match_status = ?", filter.MatchStatus)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *Repository) BatchVideoCreativeForUser(userID, id string) (*model.BatchVideoCreative, error) {
	var creative model.BatchVideoCreative
	if err := r.db.First(&creative, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		return nil, err
	}
	return &creative, nil
}

func (r *Repository) UpdateBatchVideoCreative(userID, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now()
	return r.db.Model(&model.BatchVideoCreative{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error
}

func (r *Repository) UpdateBatchVideoCreativesReview(userID, projectID string, ids []string, status, note string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.Model(&model.BatchVideoCreative{}).
		Where("user_id = ? AND project_id = ? AND id IN ?", userID, projectID, ids).
		Updates(map[string]any{"review_status": status, "review_note": note, "final_status": status, "updated_at": time.Now()})
	return result.RowsAffected, result.Error
}

func (r *Repository) CreateBatchVideoCreativeReferences(refs []model.BatchVideoCreativeReference) error {
	if len(refs) == 0 {
		return nil
	}
	return r.db.Create(&refs).Error
}

func (r *Repository) BatchVideoCreativeReferences(creativeIDs []string) ([]model.BatchVideoCreativeReference, error) {
	if len(creativeIDs) == 0 {
		return nil, nil
	}
	var refs []model.BatchVideoCreativeReference
	err := r.db.Where("creative_id IN ?", creativeIDs).Order("sort_order asc").Find(&refs).Error
	return refs, err
}

func (r *Repository) CreateBatchVideoJobs(jobs []model.BatchVideoGenerationJob) error {
	if len(jobs) == 0 {
		return nil
	}
	return r.db.CreateInBatches(&jobs, 200).Error
}

func (r *Repository) BatchVideoJobs(projectID string, status string, limit, offset int) ([]model.BatchVideoGenerationJob, error) {
	var jobs []model.BatchVideoGenerationJob
	query := r.db.Where("project_id = ?", projectID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	err := query.Order("created_at asc").Find(&jobs).Error
	return jobs, err
}

func (r *Repository) BatchVideoJobForUser(userID, id string) (*model.BatchVideoGenerationJob, error) {
	var job model.BatchVideoGenerationJob
	if err := r.db.First(&job, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *Repository) CountBatchVideoJobsForCreative(userID, creativeID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.BatchVideoGenerationJob{}).Where("user_id = ? AND creative_id = ?", userID, creativeID).Count(&count).Error
	return count, err
}

func (r *Repository) SaveBatchVideoJob(job *model.BatchVideoGenerationJob) error {
	job.UpdatedAt = time.Now()
	return r.db.Save(job).Error
}

func (r *Repository) UpdateBatchVideoJob(userID, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now()
	return r.db.Model(&model.BatchVideoGenerationJob{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error
}

func (r *Repository) CancelUnstartedBatchVideoJobs(userID, projectID string) (int64, error) {
	result := r.db.Model(&model.BatchVideoGenerationJob{}).
		Where("user_id = ? AND project_id = ? AND status IN ?", userID, projectID, []string{"submitted", "waiting"}).
		Updates(map[string]any{"status": "cancelled", "last_error": "用户取消未开始任务", "updated_at": time.Now()})
	return result.RowsAffected, result.Error
}

// StartBatchVideoProject 把 submitted 任务释放为 waiting，并把未暂停项目标为 running；
// 返回 started=false 表示项目已暂停或已删除。条件更新避免“开始”覆盖同时发生的暂停。
func (r *Repository) StartBatchVideoProject(userID, projectID string, scheduledAt *time.Time) (int64, bool, error) {
	now := time.Now()
	if scheduledAt != nil {
		result := r.db.Model(&model.BatchVideoProject{}).
			Where("id = ? AND user_id = ? AND status <> ?", projectID, userID, "paused").
			Updates(map[string]any{"scheduled_start_at": scheduledAt, "updated_at": now})
		return 0, result.RowsAffected == 1, result.Error
	}
	var released int64
	started := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		project := tx.Model(&model.BatchVideoProject{}).
			Where("id = ? AND user_id = ? AND status <> ?", projectID, userID, "paused").
			Updates(map[string]any{"status": "running", "scheduled_start_at": nil, "updated_at": now})
		if project.Error != nil {
			return project.Error
		}
		if project.RowsAffected == 0 {
			return nil
		}
		started = true
		result := tx.Model(&model.BatchVideoGenerationJob{}).
			Where("user_id = ? AND project_id = ? AND status = ?", userID, projectID, "submitted").
			Updates(map[string]any{"status": "waiting", "updated_at": now})
		released = result.RowsAffected
		return result.Error
	})
	return released, started, err
}

func (r *Repository) DueScheduledBatchVideoProjects(before time.Time) ([]model.BatchVideoProject, error) {
	var projects []model.BatchVideoProject
	// SQLite 把时间存成文本；两边都转 UTC，避免本地时区偏移导致字符串比较提前触发。
	err := r.db.Where("scheduled_start_at IS NOT NULL AND scheduled_start_at <= ? AND status <> ?", before.UTC(), "paused").
		Order("scheduled_start_at asc").Find(&projects).Error
	return projects, err
}

// ReleaseScheduledBatchVideoProject 在项目未被暂停时释放本轮提交，并清空定时标记；
// 若期间被暂停，则保留定时标记，恢复调度后由下一轮释放。
func (r *Repository) ReleaseScheduledBatchVideoProject(userID, projectID string) (int64, error) {
	var released int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var project model.BatchVideoProject
		if err := tx.First(&project, "id = ? AND user_id = ?", projectID, userID).Error; err != nil {
			return err
		}
		if project.Status == "paused" {
			return nil
		}
		now := time.Now()
		result := tx.Model(&model.BatchVideoGenerationJob{}).
			Where("user_id = ? AND project_id = ? AND status = ?", userID, projectID, "submitted").
			Updates(map[string]any{"status": "waiting", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		released = result.RowsAffected
		if released == 0 {
			// 任务可能在定时到点前被取消或删除；只清空定时，避免把项目误标为运行中。
			return tx.Model(&model.BatchVideoProject{}).
				Where("id = ? AND user_id = ? AND status <> ?", projectID, userID, "paused").
				Updates(map[string]any{"scheduled_start_at": nil, "updated_at": now}).Error
		}
		return tx.Model(&model.BatchVideoProject{}).
			Where("id = ? AND user_id = ? AND status <> ?", projectID, userID, "paused").
			Updates(map[string]any{"status": "running", "scheduled_start_at": nil, "updated_at": now}).Error
	})
	return released, err
}

// ClaimBatchVideoJob 条件更新 waiting -> dispatching，避免多实例重复创建任务。
func (r *Repository) ClaimBatchVideoJob(userID, id string) (bool, error) {
	result := r.db.Model(&model.BatchVideoGenerationJob{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, "waiting").
		Updates(map[string]any{"status": "dispatching", "updated_at": time.Now()})
	return result.RowsAffected == 1, result.Error
}

func (r *Repository) WaitingBatchVideoJobs(limit int) ([]model.BatchVideoGenerationJob, error) {
	var jobs []model.BatchVideoGenerationJob
	if limit <= 0 {
		limit = 50
	}
	err := r.db.Where("status = ?", "waiting").Order("model_priority desc, created_at asc").Limit(limit).Find(&jobs).Error
	return jobs, err
}

func (r *Repository) ReconcileBatchVideoJobs(limit int) ([]model.BatchVideoGenerationJob, error) {
	var jobs []model.BatchVideoGenerationJob
	if limit <= 0 {
		limit = 100
	}
	err := r.db.Where("status IN ?", []string{"queued", "running"}).Order("updated_at asc").Limit(limit).Find(&jobs).Error
	return jobs, err
}

func (r *Repository) TaskResults(taskID string) ([]model.Result, error) {
	var results []model.Result
	err := r.db.Where("task_id = ?", taskID).Order("created_at asc").Find(&results).Error
	return results, err
}

func (r *Repository) RecoverStaleBatchVideoJobs(before time.Time) (int64, error) {
	result := r.db.Model(&model.BatchVideoGenerationJob{}).
		Where("status = ? AND updated_at < ?", "dispatching", before).
		Updates(map[string]any{"status": "waiting", "updated_at": time.Now()})
	return result.RowsAffected, result.Error
}

func (r *Repository) CreateBatchVideoAttempt(attempt *model.BatchVideoAttempt) error {
	return r.db.Create(attempt).Error
}

func (r *Repository) AttachBatchVideoTask(userID, jobID string, attempt *model.BatchVideoAttempt, updates map[string]any) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var previous int
		if err := tx.Model(&model.BatchVideoAttempt{}).Where("job_id = ?", jobID).Select("COALESCE(MAX(attempt_no), -1)").Scan(&previous).Error; err != nil {
			return err
		}
		attempt.AttemptNo = previous + 1
		if err := tx.Create(attempt).Error; err != nil {
			return err
		}
		updates["updated_at"] = time.Now()
		result := tx.Model(&model.BatchVideoGenerationJob{}).Where("id = ? AND user_id = ? AND status = ?", jobID, userID, "dispatching").Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskStateConflict
		}
		return nil
	})
}

func (r *Repository) BatchVideoAttempts(jobID string) ([]model.BatchVideoAttempt, error) {
	var attempts []model.BatchVideoAttempt
	err := r.db.Where("job_id = ?", jobID).Order("attempt_no asc").Find(&attempts).Error
	return attempts, err
}

func (r *Repository) NextBatchVideoAttemptNo(jobID string) (int, error) {
	var value int
	err := r.db.Model(&model.BatchVideoAttempt{}).Where("job_id = ?", jobID).Select("COALESCE(MAX(attempt_no), -1)").Scan(&value).Error
	return value + 1, err
}

func (r *Repository) UpdateBatchVideoAttempt(id string, updates map[string]any) error {
	return r.db.Model(&model.BatchVideoAttempt{}).Where("id = ?", id).Updates(updates).Error
}

type BatchVideoCounts struct {
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

func (r *Repository) BatchVideoCounts(projectID string) (BatchVideoCounts, error) {
	var counts BatchVideoCounts
	var err error
	count := func(value *int64, query string, args ...any) {
		if err != nil {
			return
		}
		err = r.db.Raw(query, args...).Scan(value).Error
	}
	count(&counts.CreativeTotal, "SELECT COUNT(*) FROM batch_video_creatives WHERE project_id = ?", projectID)
	count(&counts.Approved, "SELECT COUNT(*) FROM batch_video_creatives WHERE project_id = ? AND review_status = 'approved'", projectID)
	count(&counts.PendingReview, "SELECT COUNT(*) FROM batch_video_creatives WHERE project_id = ? AND review_status = 'pending'", projectID)
	count(&counts.Rejected, "SELECT COUNT(*) FROM batch_video_creatives WHERE project_id = ? AND review_status = 'rejected'", projectID)
	count(&counts.JobsTotal, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ?", projectID)
	count(&counts.Submitted, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ? AND status = 'submitted'", projectID)
	count(&counts.Waiting, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ? AND status IN ('waiting','dispatching')", projectID)
	count(&counts.Running, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ? AND status IN ('queued','running')", projectID)
	count(&counts.Succeeded, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ? AND status = 'succeeded'", projectID)
	count(&counts.Failed, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ? AND status = 'failed'", projectID)
	count(&counts.Cancelled, "SELECT COUNT(*) FROM batch_video_generation_jobs WHERE project_id = ? AND status = 'cancelled'", projectID)
	count(&counts.Attempts, "SELECT COUNT(*) FROM batch_video_attempts WHERE project_id = ?", projectID)
	count(&counts.RetrySucceeded, "SELECT COUNT(*) FROM batch_video_attempts WHERE project_id = ? AND attempt_no > 0 AND status = 'succeeded'", projectID)
	count(&counts.RetryFailed, "SELECT COUNT(*) FROM batch_video_attempts WHERE project_id = ? AND attempt_no > 0 AND status = 'failed'", projectID)
	return counts, err
}

func (r *Repository) CreateBatchVideoStandardValidation(validation *model.BatchVideoStandardValidation) error {
	return r.db.Create(validation).Error
}

func (r *Repository) UnlockBatchVideoMass(validation *model.BatchVideoStandardValidation, unlockedAt time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(validation).Error; err != nil {
			return err
		}
		result := tx.Model(&model.BatchVideoProject{}).
			Where("id = ? AND user_id = ?", validation.ProjectID, validation.UserID).
			Updates(map[string]any{
				"mass_unlocked_at": unlockedAt,
				"mass_unlocked_by": validation.UnlockedBy,
				"stage":            "mass_unlocked",
				"status":           "ready",
				"updated_at":       time.Now(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *Repository) LatestBatchVideoStandardValidation(projectID string) (*model.BatchVideoStandardValidation, error) {
	var validation model.BatchVideoStandardValidation
	if err := r.db.Where("project_id = ?", projectID).Order("created_at desc").First(&validation).Error; err != nil {
		return nil, err
	}
	return &validation, nil
}

func (r *Repository) ResourceByContentHash(userID, hash string) (*model.Resource, error) {
	if hash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var resource model.Resource
	if err := r.db.First(&resource, "user_id = ? AND content_hash = ? AND status = ?", userID, hash, model.ResourceStatusReady).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (r *Repository) ResourceByContentHashProvider(userID, hash, provider string) (*model.Resource, error) {
	if hash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var resource model.Resource
	if err := r.db.First(&resource, "user_id = ? AND content_hash = ? AND provider = ? AND status = ?", userID, hash, provider, model.ResourceStatusReady).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (r *Repository) SaveResourceContentHash(userID, id, hash string) error {
	return r.db.Model(&model.Resource{}).Where("id = ? AND user_id = ?", id, userID).Update("content_hash", hash).Error
}

func (r *Repository) CreateResourceStorageVariant(variant *model.ResourceStorageVariant) error {
	return r.db.Create(variant).Error
}

func (r *Repository) ResourceStorageVariant(resourceID, purpose, provider, bucket string) (*model.ResourceStorageVariant, error) {
	var variant model.ResourceStorageVariant
	if err := r.db.First(&variant, "resource_id = ? AND purpose = ? AND provider = ? AND bucket = ?", resourceID, purpose, provider, bucket).Error; err != nil {
		return nil, err
	}
	return &variant, nil
}

func (r *Repository) SaveResourceStorageVariant(variant *model.ResourceStorageVariant) error {
	variant.UpdatedAt = time.Now()
	return r.db.Save(variant).Error
}

func (r *Repository) ResourceStorageVariants(resourceID string) ([]model.ResourceStorageVariant, error) {
	var items []model.ResourceStorageVariant
	err := r.db.Where("resource_id = ?", resourceID).Find(&items).Error
	return items, err
}

func (r *Repository) DeleteResourceStorageVariant(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.ResourceStorageVariant{}).Error
}

// BatchVideoReferenceUsage 返回参考图被创意引用的次数，删除前用于保护。
func (r *Repository) BatchVideoReferenceUsage(referenceAssetID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.BatchVideoCreativeReference{}).Where("reference_asset_id = ?", referenceAssetID).Count(&count).Error
	return count, err
}

// EnabledChannelModelsByCapability 返回已启用的渠道模型，供批视频模块做授权过滤。
func (r *Repository) EnabledChannelModelsByCapability(capability string) ([]model.ChannelModel, error) {
	var items []model.ChannelModel
	if err := r.db.Where("enabled = ? AND capability = ?", true, capability).Order("sort_order asc, created_at asc").Find(&items).Error; err != nil {
		return nil, err
	}
	pointers := make([]*model.ChannelModel, len(items))
	for index := range items {
		pointers[index] = &items[index]
	}
	if err := r.attachChannelModelPriceTiers(pointers); err != nil {
		return nil, err
	}
	return items, nil
}

// BatchVideoProjectsForAnalysis 返回等待回收分析任务的工程，供调度器轮询。
func (r *Repository) BatchVideoProjectsForAnalysis(limit int) ([]model.BatchVideoProject, error) {
	var items []model.BatchVideoProject
	query := r.db.Where("analysis_task_id <> '' AND analysis_status IN ?", []string{"queued", "running", "awaiting_decision"})
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Order("updated_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) BatchVideoRoundsForPlanning(limit int) ([]model.BatchVideoCreativeRound, error) {
	var items []model.BatchVideoCreativeRound
	query := r.db.Where("task_id <> '' AND status IN ?", []string{"generating", "queued", "running"})
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Order("updated_at asc").Find(&items).Error
	return items, err
}

func (r *Repository) UpdateBatchVideoCreativeRound(userID, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now()
	return r.db.Model(&model.BatchVideoCreativeRound{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error
}

func (r *Repository) BatchVideoCreativeRoundForUser(userID, id string) (*model.BatchVideoCreativeRound, error) {
	var round model.BatchVideoCreativeRound
	if err := r.db.First(&round, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		return nil, err
	}
	return &round, nil
}

func (r *Repository) BatchVideoSellingPointTextExists(projectID, text string) (bool, error) {
	var count int64
	err := r.db.Model(&model.BatchVideoSellingPoint{}).Where("project_id = ? AND canonical_text = ?", projectID, text).Count(&count).Error
	return count > 0, err
}

func (r *Repository) BatchVideoReferenceAssetsGenerating(limit int) ([]model.BatchVideoReferenceAsset, error) {
	var items []model.BatchVideoReferenceAsset
	query := r.db.Where("generation_task_id <> '' AND status = ?", "generating")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Order("updated_at asc").Find(&items).Error
	return items, err
}
