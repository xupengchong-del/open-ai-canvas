// 管理端任务看板：跨用户任务列表、状态计数、按用户的成功/失败汇总与媒体投递。
//
// 列表与轮询路径都不返回 result_json / input_json 正文。媒体只在管理员明确点击
// 播放时通过 PrepareAdminTaskMediaDelivery 读取一次，且仍走既有的资源访问合同
// （OSS/CDN 场景下是 307 重定向，字节由浏览器直连对象存储，不经过本服务）。

package app

import (
	"strings"
	"time"

	"yingce/backend/internal/assets"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

type AdminTaskQuery struct {
	AnalyticsQuery
	Status  string
	Keyword string
	Page    int
	Limit   int
}

// 直接复用仓库读取模型，避免维护两套同形结构。
type AdminTaskCounts = repository.AdminTaskCounts
type AdminTaskUserStat = repository.AdminTaskUserStat

type AdminTaskSummary struct {
	ID                string     `json:"id"`
	UserID            string     `json:"userId"`
	Username          string     `json:"username"`
	DisplayName       string     `json:"displayName"`
	Type              string     `json:"type"`
	Status            string     `json:"status"`
	Stage             string     `json:"stage"`
	Progress          int        `json:"progress"`
	Operation         string     `json:"operation,omitempty"`
	Provider          string     `json:"provider,omitempty"`
	Model             string     `json:"model,omitempty"`
	ProviderRequestID string     `json:"providerRequestId,omitempty"`
	Attempts          int        `json:"attempts"`
	Error             string     `json:"error,omitempty"`
	Prompt            string     `json:"prompt,omitempty"`
	HasMedia          bool       `json:"hasMedia"`
	MediaKind         string     `json:"mediaKind,omitempty"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type AdminTaskMonitorPage struct {
	Tasks  []AdminTaskSummary         `json:"tasks"`
	Total  int64                      `json:"total"`
	Page   int                        `json:"page"`
	Limit  int                        `json:"pageSize"`
	Counts repository.AdminTaskCounts `json:"counts"`
}

func (s *Service) AdminTasks(actor *model.User, query AdminTaskQuery) (*AdminTaskMonitorPage, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	filter, err := adminTaskFilter(query)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.repo.AdminTasks(filter)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.AdminTaskCounts(filter)
	if err != nil {
		return nil, err
	}
	tasks := make([]AdminTaskSummary, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, adminTaskSummary(row))
	}
	return &AdminTaskMonitorPage{Tasks: tasks, Total: total, Page: filter.Page, Limit: filter.Limit, Counts: counts}, nil
}

func (s *Service) AdminTaskUserStats(actor *model.User, query AdminTaskQuery) ([]repository.AdminTaskUserStat, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	filter, err := adminTaskFilter(query)
	if err != nil {
		return nil, err
	}
	// 用户汇总始终覆盖整个时间窗，不受当前选中用户影响。
	filter.UserID = ""
	filter.Status = nil
	filter.Keyword = ""
	filter.Page = 0
	stats, err := s.repo.AdminTaskUserStats(filter)
	if err != nil {
		return nil, err
	}
	if stats == nil {
		stats = []repository.AdminTaskUserStat{}
	}
	return stats, nil
}

func (s *Service) AdminTask(actor *model.User, taskID string) (*AdminTaskSummary, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	row, err := s.repo.AdminTaskByID(strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, NotFound("任务不存在")
	}
	summary := adminTaskSummary(*row)
	return &summary, nil
}

// PrepareAdminTaskMediaDelivery 只在管理员点击播放时执行一次媒体授权。
func (s *Service) PrepareAdminTaskMediaDelivery(actor *model.User, taskID string, options ResourceAccessOptions, rangeHeader string) (*ResourceDelivery, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	task, err := s.repo.Task(strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	if err := validateAdminTaskMediaStatus(task.Status); err != nil {
		return nil, err
	}
	previewURL, _ := taskMediaPreview(task.ResultJSON, task.Type)
	resourceID := canvasResourceID(previewURL)
	if resourceID == "" {
		return nil, BadAuthRequest("该任务没有可预览媒体")
	}
	resource, err := s.repo.ResourceForUser(task.UserID, resourceID)
	if err != nil {
		return nil, err
	}
	if options.Purpose == "" {
		options.Purpose = assets.PurposeDisplay
	}
	return s.prepareResourceDelivery(task.UserID, resource, options, rangeHeader)
}

func validateAdminTaskMediaStatus(status model.TaskStatus) error {
	if status != model.TaskStatusSucceeded {
		return BadAuthRequest("只有成功任务可以预览媒体")
	}
	return nil
}

func adminTaskFilter(query AdminTaskQuery) (repository.AdminTaskFilter, error) {
	filter := repository.AdminTaskFilter{
		AnalyticsFilter: normalizeAnalyticsFilter(query.AnalyticsQuery),
		Keyword:         query.Keyword,
		Page:            query.Page,
		Limit:           query.Limit,
	}
	statuses, err := parseAdminTaskStatuses(query.Status)
	if err != nil {
		return repository.AdminTaskFilter{}, err
	}
	filter.Status = statuses
	return filter, nil
}

func parseAdminTaskStatuses(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "all" {
		return nil, nil
	}
	allowed := map[string]bool{
		string(model.TaskStatusQueued):    true,
		string(model.TaskStatusRunning):   true,
		string(model.TaskStatusSucceeded): true,
		string(model.TaskStatusFailed):    true,
		string(model.TaskStatusCancelled): true,
	}
	var statuses []string
	for _, part := range strings.Split(trimmed, ",") {
		status := strings.TrimSpace(part)
		if status == "" {
			continue
		}
		if !allowed[status] {
			return nil, BadAuthRequest("任务状态无效")
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func adminTaskSummary(row repository.AdminTaskRow) AdminTaskSummary {
	// 是否可播放只看结果长度与任务类型，不读取 result_json 正文。
	hasMedia := row.ResultJSONLength > 0
	mediaKind := ""
	if hasMedia {
		mediaKind = taskMediaKindForType(row.Type)
	}
	hasMedia = hasMedia && mediaKind != ""
	return AdminTaskSummary{
		ID:                row.ID,
		UserID:            row.UserID,
		Username:          row.UserUsername,
		DisplayName:       row.UserDisplayName,
		Type:              row.Type,
		Status:            string(row.Status),
		Stage:             row.Stage,
		Progress:          row.Progress,
		Operation:         row.Operation,
		Provider:          row.Provider,
		Model:             row.Model,
		ProviderRequestID: row.ProviderRequestID,
		Attempts:          row.Attempts,
		Error:             row.Error,
		Prompt:            truncateRunes(row.Prompt, 240),
		HasMedia:          hasMedia,
		MediaKind:         mediaKind,
		StartedAt:         row.StartedAt,
		CompletedAt:       row.CompletedAt,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

// taskMediaKindForType 从任务类型推断媒体种类，避免为了判断能否播放去读结果正文。
func taskMediaKindForType(taskType string) string {
	lower := strings.ToLower(taskType)
	switch {
	case strings.Contains(lower, "video"):
		return "video"
	case strings.Contains(lower, "image"):
		return "image"
	default:
		return ""
	}
}
