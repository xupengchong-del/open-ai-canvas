package model

import "time"

// 批量视频模块独立建表，不复用画布批次字段，避免与画布节点 metadata 耦合。
type BatchVideoProject struct {
	ID                     string     `json:"id" gorm:"primaryKey;size:36"`
	UserID                 string     `json:"userId" gorm:"index;size:36;index:idx_batch_video_projects_user_created,priority:1"`
	Name                   string     `json:"name" gorm:"size:160;not null"`
	ProductName            string     `json:"productName" gorm:"size:160"`
	Stage                  string     `json:"stage" gorm:"size:40;index;not null;default:draft"`
	Status                 string     `json:"status" gorm:"size:40;index;not null;default:draft"`
	SettingsJSON           string     `json:"settingsJson" gorm:"type:text"`
	ProductProfileJSON     string     `json:"productProfileJson" gorm:"type:text"`
	StandardValidationJSON string     `json:"standardValidationJson" gorm:"type:text"`
	CurrentProfileVersion  int        `json:"currentProfileVersion" gorm:"not null;default:1"`
	ReferencePoolVersion   int        `json:"referencePoolVersion" gorm:"not null;default:1"`
	AnalysisTaskID         string     `json:"analysisTaskId" gorm:"size:36;index"`
	AnalysisStatus         string     `json:"analysisStatus" gorm:"size:24;index;not null;default:none"`
	AnalysisError          string     `json:"analysisError" gorm:"type:text"`
	MassUnlockedAt         *time.Time `json:"massUnlockedAt,omitempty"`
	MassUnlockedBy         string     `json:"massUnlockedBy,omitempty" gorm:"size:36"`
	CreatedAt              time.Time  `json:"createdAt" gorm:"index:idx_batch_video_projects_user_created,priority:2"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

func (BatchVideoProject) TableName() string { return "batch_video_projects" }

type BatchVideoMaterialBatch struct {
	ID                string    `json:"id" gorm:"primaryKey;size:36"`
	ProjectID         string    `json:"projectId" gorm:"index;size:36;index:idx_batch_video_material_batches_project_no,priority:1"`
	UserID            string    `json:"userId" gorm:"index;size:36"`
	BatchNo           int       `json:"batchNo" gorm:"index:idx_batch_video_material_batches_project_no,priority:2"`
	Label             string    `json:"label" gorm:"size:160"`
	MaterialJSON      string    `json:"materialJson" gorm:"type:text"`
	AnalysisJSON      string    `json:"analysisJson" gorm:"type:text"`
	AnalysisDeltaJSON string    `json:"analysisDeltaJson" gorm:"type:text"`
	VisualVersion     int       `json:"visualVersion" gorm:"not null;default:1"`
	Status            string    `json:"status" gorm:"size:40;index;not null;default:draft"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (BatchVideoMaterialBatch) TableName() string { return "batch_video_material_batches" }

type BatchVideoSellingPoint struct {
	ID               string    `json:"id" gorm:"primaryKey;size:36"`
	ProjectID        string    `json:"projectId" gorm:"index;size:36;index:idx_batch_video_selling_points_project_status,priority:1"`
	UserID           string    `json:"userId" gorm:"index;size:36"`
	CanonicalText    string    `json:"canonicalText" gorm:"size:500;not null"`
	EvidenceJSON     string    `json:"evidenceJson" gorm:"type:text"`
	Confidence       float64   `json:"confidence"`
	Source           string    `json:"source" gorm:"size:24"`
	Status           string    `json:"status" gorm:"size:24;index:idx_batch_video_selling_points_project_status,priority:2;not null;default:active"`
	UsageCount       int       `json:"usageCount"`
	FirstSeenBatchID string    `json:"firstSeenBatchId" gorm:"size:36"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (BatchVideoSellingPoint) TableName() string { return "batch_video_selling_points" }

type BatchVideoReferenceAsset struct {
	ID          string `json:"id" gorm:"primaryKey;size:36"`
	ProjectID   string `json:"projectId" gorm:"index;size:36;index:idx_batch_video_reference_assets_project_status,priority:1;index:idx_batch_video_reference_assets_project_hash,priority:1"`
	UserID      string `json:"userId" gorm:"index;size:36"`
	ResourceID  string `json:"resourceId" gorm:"index;size:36"`
	ContentHash string `json:"contentHash" gorm:"size:64;index:idx_batch_video_reference_assets_project_hash,priority:2"`
	Title       string `json:"title" gorm:"size:160"`
	Description string `json:"description" gorm:"type:text"`
	ImagePrompt string `json:"imagePrompt" gorm:"type:text"`
	TagsJSON    string `json:"tagsJson" gorm:"type:text"`
	Source      string `json:"source" gorm:"size:24"`
	Status      string `json:"status" gorm:"size:24;index:idx_batch_video_reference_assets_project_status,priority:2;not null;default:draft"`
	Version     int    `json:"version" gorm:"not null;default:1"`
	// GeneratedReference 资产由批量模块的图片任务生成；CreativeID 用于回填匹配。
	GenerationTaskID string     `json:"generationTaskId" gorm:"size:36;index"`
	CreativeID       string     `json:"creativeId" gorm:"size:36;index"`
	GenerationError  string     `json:"generationError" gorm:"type:text"`
	UsageCount       int        `json:"usageCount"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func (BatchVideoReferenceAsset) TableName() string { return "batch_video_reference_assets" }

type BatchVideoCreativeReference struct {
	ID               string `json:"id" gorm:"primaryKey;size:36"`
	CreativeID       string `json:"creativeId" gorm:"index;size:36;uniqueIndex:idx_batch_video_creative_references_unique,priority:1"`
	ReferenceAssetID string `json:"referenceAssetId" gorm:"index;size:36;uniqueIndex:idx_batch_video_creative_references_unique,priority:2"`
	AssetVersion     int    `json:"assetVersion" gorm:"not null;default:1"`
	Role             string `json:"role" gorm:"size:24"`
	Caption          string `json:"caption" gorm:"type:text"`
	SortOrder        int    `json:"sortOrder"`
}

func (BatchVideoCreativeReference) TableName() string { return "batch_video_creative_references" }

type BatchVideoCreativeRound struct {
	ID                   string    `json:"id" gorm:"primaryKey;size:36"`
	ProjectID            string    `json:"projectId" gorm:"index;size:36;index:idx_batch_video_creative_rounds_project_no,priority:1"`
	UserID               string    `json:"userId" gorm:"index;size:36"`
	RoundNo              int       `json:"roundNo" gorm:"index:idx_batch_video_creative_rounds_project_no,priority:2"`
	Mode                 string    `json:"mode" gorm:"size:24;not null;default:standard"`
	RequestedCount       int       `json:"requestedCount"`
	TemplateSnapshotJSON string    `json:"templateSnapshotJson" gorm:"type:text"`
	MaterialBatchIDsJSON string    `json:"materialBatchIdsJson" gorm:"type:text"`
	ProfileVersion       int       `json:"profileVersion"`
	TaskID               string    `json:"taskId" gorm:"size:36;index"`
	ModelID              string    `json:"modelId" gorm:"size:36"`
	Error                string    `json:"error" gorm:"type:text"`
	Status               string    `json:"status" gorm:"size:32;index;not null;default:draft"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

func (BatchVideoCreativeRound) TableName() string { return "batch_video_creative_rounds" }

type BatchVideoCreative struct {
	ID                      string    `json:"id" gorm:"primaryKey;size:36"`
	ProjectID               string    `json:"projectId" gorm:"index;size:36;index:idx_batch_video_creatives_project_round,priority:1;index:idx_batch_video_creatives_project_review,priority:1"`
	RoundID                 string    `json:"roundId" gorm:"index;size:36;index:idx_batch_video_creatives_project_round,priority:2"`
	UserID                  string    `json:"userId" gorm:"index;size:36"`
	Seq                     int       `json:"seq" gorm:"index:idx_batch_video_creatives_project_round,priority:3"`
	TemplateID              string    `json:"templateId" gorm:"size:80"`
	SellingPointID          string    `json:"sellingPointId" gorm:"index;size:36"`
	Title                   string    `json:"title" gorm:"size:300"`
	Hook                    string    `json:"hook" gorm:"type:text"`
	Copywriting             string    `json:"copywriting" gorm:"type:text"`
	VideoPrompt             string    `json:"videoPrompt" gorm:"type:text"`
	ImageRequirementJSON    string    `json:"imageRequirementJson" gorm:"type:text"`
	ImagePrompt             string    `json:"imagePrompt" gorm:"type:text"`
	DurationSeconds         int       `json:"durationSeconds"`
	AspectRatio             string    `json:"aspectRatio" gorm:"size:16"`
	MatchedReferenceAssetID string    `json:"matchedReferenceAssetId" gorm:"index;size:36"`
	MatchStatus             string    `json:"matchStatus" gorm:"size:24;index;not null;default:pending"`
	MatchMethod             string    `json:"matchMethod" gorm:"size:24"`
	MatchScore              float64   `json:"matchScore"`
	ReviewStatus            string    `json:"reviewStatus" gorm:"size:24;index:idx_batch_video_creatives_project_review,priority:2;not null;default:pending"`
	ReviewNote              string    `json:"reviewNote" gorm:"type:text"`
	FinalStatus             string    `json:"finalStatus" gorm:"size:24;not null;default:pending"`
	DuplicateHash           string    `json:"duplicateHash" gorm:"size:64;index"`
	SourceSnapshotJSON      string    `json:"sourceSnapshotJson" gorm:"type:text"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

func (BatchVideoCreative) TableName() string { return "batch_video_creatives" }

type BatchVideoGenerationJob struct {
	ID                string    `json:"id" gorm:"primaryKey;size:36"`
	ProjectID         string    `json:"projectId" gorm:"index;size:36;index:idx_batch_video_generation_jobs_project_status,priority:1"`
	CreativeID        string    `json:"creativeId" gorm:"index;size:36"`
	UserID            string    `json:"userId" gorm:"index;size:36"`
	ModelID           string    `json:"modelId" gorm:"size:120"`
	ChannelModelID    string    `json:"channelModelId" gorm:"size:36"`
	LogicalModelID    string    `json:"logicalModelId" gorm:"size:36"`
	Mode              string    `json:"mode" gorm:"size:16;not null;default:single"`
	Status            string    `json:"status" gorm:"size:24;index:idx_batch_video_generation_jobs_project_status,priority:2;not null;default:waiting"`
	CurrentTaskID     string    `json:"currentTaskId" gorm:"index;size:36"`
	TaskTemplateJSON  string    `json:"-" gorm:"type:text"`
	AttemptsUsed      int       `json:"attemptsUsed"`
	FailoverCount     int       `json:"failoverCount"`
	RetryRound        int       `json:"retryRound" gorm:"not null;default:0"`
	SuccessResourceID string    `json:"successResourceId" gorm:"size:36"`
	LastError         string    `json:"lastError" gorm:"type:text"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (BatchVideoGenerationJob) TableName() string { return "batch_video_generation_jobs" }

type BatchVideoAttempt struct {
	ID                  string     `json:"id" gorm:"primaryKey;size:36"`
	ProjectID           string     `json:"projectId" gorm:"index;size:36"`
	CreativeID          string     `json:"creativeId" gorm:"index;size:36"`
	JobID               string     `json:"jobId" gorm:"index;size:36;index:idx_batch_video_attempts_job_no,priority:1"`
	UserID              string     `json:"userId" gorm:"index;size:36"`
	AttemptNo           int        `json:"attemptNo" gorm:"index:idx_batch_video_attempts_job_no,priority:2"`
	RetryRound          int        `json:"retryRound" gorm:"not null;default:0"`
	ModelID             string     `json:"modelId" gorm:"size:120"`
	TaskID              string     `json:"taskId" gorm:"index;size:36"`
	Status              string     `json:"status" gorm:"size:24;index"`
	Error               string     `json:"error" gorm:"type:text"`
	SwitchedFromModelID string     `json:"switchedFromModelId" gorm:"size:120"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	FinishedAt          *time.Time `json:"finishedAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
}

func (BatchVideoAttempt) TableName() string { return "batch_video_attempts" }

type BatchVideoStandardValidation struct {
	ID                       string     `json:"id" gorm:"primaryKey;size:36"`
	ProjectID                string     `json:"projectId" gorm:"index;size:36"`
	UserID                   string     `json:"userId" gorm:"index;size:36"`
	StandardRoundID          string     `json:"standardRoundId" gorm:"size:36"`
	CreativeCount            int        `json:"creativeCount"`
	ApprovedCount            int        `json:"approvedCount"`
	SuccessCount             int        `json:"successCount"`
	SuccessRate              float64    `json:"successRate"`
	ValidatedTemplateIDsJSON string     `json:"validatedTemplateIdsJson" gorm:"type:text"`
	ValidatedModelIDsJSON    string     `json:"validatedModelIdsJson" gorm:"type:text"`
	ReferencePoolVersion     int        `json:"referencePoolVersion"`
	MaterialBatchIDsJSON     string     `json:"materialBatchIdsJson" gorm:"type:text"`
	ProfileVersion           int        `json:"profileVersion"`
	Status                   string     `json:"status" gorm:"size:24;not null;default:pending"`
	UnlockedBy               string     `json:"unlockedBy" gorm:"size:36"`
	UnlockedAt               *time.Time `json:"unlockedAt,omitempty"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

func (BatchVideoStandardValidation) TableName() string { return "batch_video_standard_validations" }

// ResourceStorageVariant 记录同一逻辑资源的派生存储副本；批量视频只允许视频参考图产生 OSS 副本。
type ResourceStorageVariant struct {
	ID               string    `json:"id" gorm:"primaryKey;size:36"`
	ResourceID       string    `json:"resourceId" gorm:"index;size:36;uniqueIndex:idx_resource_storage_variants_identity,priority:1"`
	UserID           string    `json:"userId" gorm:"index;size:36"`
	Purpose          string    `json:"purpose" gorm:"size:40;uniqueIndex:idx_resource_storage_variants_identity,priority:2"`
	Provider         string    `json:"provider" gorm:"size:24;uniqueIndex:idx_resource_storage_variants_identity,priority:3"`
	Endpoint         string    `json:"endpoint"`
	Bucket           string    `json:"bucket" gorm:"size:160;uniqueIndex:idx_resource_storage_variants_identity,priority:4"`
	ObjectKey        string    `json:"objectKey"`
	PublicURL        string    `json:"publicUrl"`
	StorageSettingID string    `json:"-" gorm:"index;size:36"`
	ContentHash      string    `json:"contentHash" gorm:"size:64;index"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (ResourceStorageVariant) TableName() string { return "resource_storage_variants" }
