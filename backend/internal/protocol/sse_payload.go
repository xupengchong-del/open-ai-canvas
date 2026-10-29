package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// mergeEventStreamPayload 把声明式插件收到的 SSE 文本响应合并成普通响应 JSON。
// 部分 OpenAI 兼容网关即使收到非流式请求也会返回 data: 帧；不合并就会被当成非法 JSON。
// 返回 matched=false 表示响应不是 SSE，调用方继续按普通 JSON 处理。
func mergeEventStreamPayload(body []byte) (map[string]any, bool, error) {
	trimmed := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return nil, false, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64<<10), len(body)+1)
	payload := map[string]any{}
	choices := map[int]map[string]any{}
	order := make([]int, 0, 1)
	var dataLines []string
	frames := 0

	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		raw := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if raw == "" || raw == "[DONE]" {
			return nil
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			return fmt.Errorf("流式事件不是 JSON：%w", err)
		}
		frames++
		for key, value := range frame {
			switch key {
			case "choices", "usage":
			default:
				if _, exists := payload[key]; !exists {
					payload[key] = value
				}
			}
		}
		if usage, ok := frame["usage"].(map[string]any); ok {
			payload["usage"] = usage
		}
		for _, item := range arrayValue(frame["choices"]) {
			choice, _ := item.(map[string]any)
			if choice == nil {
				continue
			}
			index := intValue(choice["index"])
			target := choices[index]
			if target == nil {
				target = map[string]any{"index": index, "message": map[string]any{}}
				choices[index] = target
				order = append(order, index)
			}
			message, _ := target["message"].(map[string]any)
			if message == nil {
				message = map[string]any{}
				target["message"] = message
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				mergeSSEChoice(message, delta)
			}
			if value, ok := choice["message"].(map[string]any); ok {
				mergeSSEChoice(message, value)
			}
			if finish, ok := choice["finish_reason"].(string); ok && finish != "" {
				target["finish_reason"] = finish
			}
			appendSSEText(target, "text", choice["text"])
		}
		if text, ok := frame["output_text"].(string); ok {
			appendSSEText(payload, "output_text", text)
		}
		if text, ok := frame["delta"].(string); ok {
			appendSSEText(payload, "output_text", text)
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return nil, true, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, true, fmt.Errorf("读取流式响应失败：%w", err)
	}
	if err := flush(); err != nil {
		return nil, true, err
	}
	if frames == 0 {
		return nil, true, fmt.Errorf("上游流式响应为空")
	}
	merged := make([]any, 0, len(order))
	for _, index := range order {
		merged = append(merged, choices[index])
	}
	payload["choices"] = merged
	return payload, true, nil
}

// mergeSSEChoice 逐帧累加 delta/message 里的正文和推理，保留最后一帧的 role。
func mergeSSEChoice(target map[string]any, frame map[string]any) {
	if role, ok := frame["role"].(string); ok && role != "" {
		target["role"] = role
	}
	appendSSEText(target, "content", frame["content"])
	appendSSEText(target, "reasoning_content", frame["reasoning_content"])
	appendSSEText(target, "reasoning", frame["reasoning"])
}

// appendSSEText 支持纯字符串和分段内容数组（[{type:"text",text:"..."}]）。
func appendSSEText(target map[string]any, key string, value any) {
	var addition string
	switch typed := value.(type) {
	case string:
		addition = typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			switch part := item.(type) {
			case string:
				builder.WriteString(part)
			case map[string]any:
				if text, ok := part["text"].(string); ok {
					builder.WriteString(text)
				}
			}
		}
		addition = builder.String()
	}
	if addition == "" {
		return
	}
	existing, _ := target[key].(string)
	target[key] = existing + addition
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	}
	return 0
}
