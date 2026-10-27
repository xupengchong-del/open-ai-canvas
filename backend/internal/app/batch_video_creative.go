package app

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

type UpdateBatchVideoCreativeInput struct {
	Title       string `json:"title"`
	Hook        string `json:"hook"`
	Copywriting string `json:"copywriting"`
	VideoPrompt string `json:"videoPrompt"`
	ImagePrompt string `json:"imagePrompt"`
}

func (s *Service) UpdateBatchVideoCreativeContent(userID, projectID, creativeID string, input UpdateBatchVideoCreativeInput) (*model.BatchVideoCreative, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return nil, err
	}
	creative, err := s.repo.BatchVideoCreativeForUser(userID, creativeID)
	if err != nil || creative.ProjectID != projectID {
		return nil, BadAuthRequest("创意不存在")
	}
	jobCount, err := s.repo.CountBatchVideoJobsForCreative(userID, creativeID)
	if err != nil {
		return nil, err
	}
	if jobCount > 0 {
		return nil, BadAuthRequest("创意已提交生成，不能修改内容")
	}
	updated := *creative
	updated.Title = strings.TrimSpace(input.Title)
	updated.Hook = strings.TrimSpace(input.Hook)
	updated.Copywriting = strings.TrimSpace(input.Copywriting)
	updated.VideoPrompt = strings.TrimSpace(input.VideoPrompt)
	updated.ImagePrompt = strings.TrimSpace(input.ImagePrompt)
	for field, value := range map[string]string{
		"标题": updated.Title, "钩子": updated.Hook, "文案": updated.Copywriting,
		"视频提示词": updated.VideoPrompt, "图片提示词": updated.ImagePrompt,
	} {
		if value == "" {
			return nil, BadAuthRequest(field + "不能为空")
		}
	}
	if utf8.RuneCountInString(updated.Title) > 160 {
		return nil, BadAuthRequest("创意标题不能超过 160 字")
	}
	combination, copywriting := batchVideoCreativeDuplicateKeys(&updated)
	history, err := s.repo.BatchVideoCreatives(repository.BatchVideoCreativeFilter{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	for index := range history {
		if history[index].ID == creativeID {
			continue
		}
		otherCombination, otherCopywriting := batchVideoCreativeDuplicateKeys(&history[index])
		if combination == otherCombination || (copywriting != "" && copywriting == otherCopywriting) {
			return nil, BadAuthRequest("修改后与项目已有创意完全重复")
		}
	}
	updated.DuplicateHash = creativeDuplicateHash(&updated)
	if err := s.repo.UpdateBatchVideoCreative(userID, creativeID, map[string]any{
		"title":          updated.Title,
		"hook":           updated.Hook,
		"copywriting":    updated.Copywriting,
		"video_prompt":   updated.VideoPrompt,
		"image_prompt":   updated.ImagePrompt,
		"duplicate_hash": updated.DuplicateHash,
		"review_status":  "pending",
		"review_note":    "",
		"final_status":   "pending",
	}); err != nil {
		return nil, err
	}
	return s.repo.BatchVideoCreativeForUser(userID, creativeID)
}

func (s *Service) RematchBatchVideoCreative(userID, projectID, creativeID string) (*model.BatchVideoCreative, error) {
	if _, err := s.repo.BatchVideoProjectForUser(userID, projectID); err != nil {
		return nil, err
	}
	creative, err := s.repo.BatchVideoCreativeForUser(userID, creativeID)
	if err != nil || creative.ProjectID != projectID {
		return nil, BadAuthRequest("创意不存在")
	}
	if creative.MatchStatus == "generating" {
		return nil, BadAuthRequest("参考图正在生成，完成后才能重新匹配")
	}
	jobCount, err := s.repo.CountBatchVideoJobsForCreative(userID, creativeID)
	if err != nil {
		return nil, err
	}
	if jobCount > 0 {
		return nil, BadAuthRequest("创意已提交生成，不能修改参考图匹配")
	}
	var requirement batchVideoImageRequirement
	if err := json.Unmarshal([]byte(creative.ImageRequirementJSON), &requirement); err != nil {
		return nil, BadAuthRequest("创意图片需求无效，无法重新匹配")
	}
	tags := batchVideoNonEmptyStrings(append([]string{requirement.Scene, requirement.Angle, requirement.Style}, requirement.RequiredElements...))
	assets, err := s.repo.BatchVideoReferenceAssets(projectID, "approved")
	if err != nil {
		return nil, err
	}
	matched := batchVideoMatchReferenceAsset(assets, tags, 0)
	updates := map[string]any{
		"matched_reference_asset_id": "",
		"match_status":               "missing",
		"match_method":               "program",
		"match_score":                0,
		"review_status":              "pending",
		"review_note":                "",
		"final_status":               "pending",
	}
	if matched != nil {
		updates["matched_reference_asset_id"] = matched.ID
		updates["match_status"] = "matched"
		updates["match_score"] = batchVideoReferenceScore(matched, tags)
	}
	if err := s.repo.UpdateBatchVideoCreative(userID, creativeID, updates); err != nil {
		return nil, err
	}
	return s.repo.BatchVideoCreativeForUser(userID, creativeID)
}
