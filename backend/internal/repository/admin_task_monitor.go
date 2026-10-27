// 管理端任务看板的读取模型：跨用户任务列表、状态计数与按用户的成功/失败汇总。
//
// 列表故意不读取 result_json 与 input_json 正文：这两个字段是长文本，看板每 10 秒
// 轮询一次，传输整段 JSON 既浪费带宽也会放大数据库 IO。是否可播放媒体用
// length(result_json) 判定，不需要把正文带回应用层。

package repository

import (
	"strings"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

type AdminTaskFilter struct {
	AnalyticsFilter
	Status  []string
	Keyword string
	Page    int
	Limit   int
}

// AdminTaskCounts 是当前筛选条件下的状态分布，供看板顶部状态卡使用。
type AdminTaskCounts struct {
	Queued    int64 `json:"queued"`
	Running   int64 `json:"running"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Cancelled int64 `json:"cancelled"`
	Total     int64 `json:"total"`
}

// AdminTaskUserStat 只在用户维度上增加成功/失败计数；其余业务维度由既有
// GET /admin/analytics/users 提供，这里不重复聚合。
type AdminTaskUserStat struct {
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Succeeded   int64  `json:"succeeded"`
	Failed      int64  `json:"failed"`
	Total       int64  `json:"total"`
}

// AdminTaskRow 是列表查询的窄投影，带出用户展示名与媒体可用性标记。
type AdminTaskRow struct {
	ID                string           `json:"id"`
	UserID            string           `json:"userId"`
	UserUsername      string           `json:"userUsername"`
	UserDisplayName   string           `json:"userDisplayName"`
	ProjectID         string           `json:"projectId"`
	Type              string           `json:"type"`
	Status            model.TaskStatus `json:"status"`
	Stage             string           `json:"stage"`
	Progress          int              `json:"progress"`
	Operation         string           `json:"operation"`
	Provider          string           `json:"provider"`
	Model             string           `json:"model"`
	ProviderRequestID string           `json:"providerRequestId"`
	Attempts          int              `json:"attempts"`
	Error             string           `json:"error"`
	Prompt            string           `json:"prompt"`
	ResultJSONLength  int64            `json:"resultJsonLength"`
	StartedAt         *time.Time       `json:"startedAt"`
	CompletedAt       *time.Time       `json:"completedAt"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}

const adminTaskSelectColumns = "tasks.id, tasks.user_id, tasks.project_id, tasks.type, tasks.status, tasks.stage, tasks.progress, tasks.operation, tasks.provider, tasks.model, tasks.provider_request_id, tasks.attempts, tasks.error, tasks.prompt, tasks.started_at, tasks.completed_at, tasks.created_at, tasks.updated_at, users.username AS user_username, users.display_name AS user_display_name, length(tasks.result_json) AS result_json_length"

func (r *Repository) adminTaskFilterQuery(filter AdminTaskFilter) *gorm.DB {
	query := whereTimeRange(r.db.Model(&model.Task{}), "tasks.created_at", filter.From, filter.To).
		Joins("LEFT JOIN users ON users.id = tasks.user_id")
	if filter.UserID != "" {
		query = query.Where("tasks.user_id = ?", filter.UserID)
	}
	if len(filter.Status) > 0 {
		query = query.Where("tasks.status IN ?", filter.Status)
	}
	if filter.Capability != "" {
		if condition, args := taskCapabilityCondition(filter.Capability); condition != "" {
			query = query.Where(condition, args...)
		}
	}
	if value := strings.TrimSpace(filter.Keyword); value != "" {
		pattern := "%" + strings.ToLower(value) + "%"
		query = query.Where(
			"lower(tasks.id) LIKE ? OR lower(tasks.user_id) LIKE ? OR lower(users.username) LIKE ? OR lower(users.display_name) LIKE ? OR lower(tasks.model) LIKE ? OR lower(tasks.type) LIKE ? OR lower(tasks.error) LIKE ? OR lower(tasks.prompt) LIKE ?",
			pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}
	return query
}

// taskCapabilityCondition 把能力类型映射为任务类型谓词。任务 type 是自由字符串
// （canvas_text / canvas_video / video_xxx 等），沿用现有按名称包含判断的口径。
func taskCapabilityCondition(capability string) (string, []any) {
	switch capability {
	case "text":
		return "tasks.type IN ?", []any{[]string{"text", "canvas_text"}}
	case "image":
		return "tasks.type LIKE ?", []any{"%image%"}
	case "video":
		return "tasks.type LIKE ?", []any{"%video%"}
	case "audio":
		return "(tasks.type LIKE ? OR tasks.type LIKE ? OR tasks.type LIKE ?)", []any{"%audio%", "%voice%", "%speech%"}
	default:
		return "", nil
	}
}

func (r *Repository) AdminTasks(filter AdminTaskFilter) ([]AdminTaskRow, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	var total int64
	if err := r.adminTaskFilterQuery(filter).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AdminTaskRow
	err := r.adminTaskFilterQuery(filter).
		Select(adminTaskSelectColumns).
		Order("tasks.created_at desc").
		Offset((filter.Page - 1) * filter.Limit).
		Limit(filter.Limit).
		Scan(&rows).Error
	return rows, total, err
}

func (r *Repository) AdminTaskCounts(filter AdminTaskFilter) (AdminTaskCounts, error) {
	var rows []struct {
		Status model.TaskStatus
		Count  int64
	}
	// 计数不受分页影响，但保留状态以外的其他筛选条件。
	counting := filter
	counting.Page = 0
	counting.Limit = 0
	counting.Status = nil
	if err := r.adminTaskFilterQuery(counting).
		Select("tasks.status AS status, count(*) AS count").
		Group("tasks.status").
		Scan(&rows).Error; err != nil {
		return AdminTaskCounts{}, err
	}
	var counts AdminTaskCounts
	for _, row := range rows {
		switch row.Status {
		case model.TaskStatusQueued:
			counts.Queued += row.Count
		case model.TaskStatusRunning:
			counts.Running += row.Count
		case model.TaskStatusSucceeded:
			counts.Succeeded += row.Count
		case model.TaskStatusFailed:
			counts.Failed += row.Count
		case model.TaskStatusCancelled:
			counts.Cancelled += row.Count
		}
		counts.Total += row.Count
	}
	return counts, nil
}

// AdminTaskUserStats 按用户汇总成功/失败数量。这里只做 user_id + status 分组，
// 不引入新的业务维度。
func (r *Repository) AdminTaskUserStats(filter AdminTaskFilter) ([]AdminTaskUserStat, error) {
	var rows []AdminTaskUserStat
	query := whereTimeRange(r.db.Model(&model.Task{}), "tasks.created_at", filter.From, filter.To).
		Joins("LEFT JOIN users ON users.id = tasks.user_id").
		Select("tasks.user_id AS user_id, users.username AS username, users.display_name AS display_name, count(*) AS total, coalesce(sum(case when tasks.status = ? then 1 else 0 end), 0) AS succeeded, coalesce(sum(case when tasks.status = ? then 1 else 0 end), 0) AS failed", model.TaskStatusSucceeded, model.TaskStatusFailed).
		Group("tasks.user_id, users.username, users.display_name").
		Order("total desc")
	if filter.Capability != "" {
		if condition, args := taskCapabilityCondition(filter.Capability); condition != "" {
			query = query.Where(condition, args...)
		}
	}
	return rows, query.Scan(&rows).Error
}

// AdminTaskByID 读取单条任务用于详情抽屉，同样不返回 result_json 正文。
func (r *Repository) AdminTaskByID(id string) (*AdminTaskRow, error) {
	var rows []AdminTaskRow
	err := r.db.Model(&model.Task{}).
		Joins("LEFT JOIN users ON users.id = tasks.user_id").
		Where("tasks.id = ?", id).
		Select(adminTaskSelectColumns).
		Limit(1).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}
