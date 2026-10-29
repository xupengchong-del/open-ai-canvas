package protocol

import (
	"context"
	"strings"
	"testing"
)

func sseManifestAdapter(t *testing.T) Adapter {
	t.Helper()
	manifest := []byte(`{
		"apiVersion":"yingce.plugin/v1",
		"id":"sse-text","version":"1.0.0","name":"SSE Text","author":"Test","documentation":"# SSE Text",
		"contributes":{"providers":[{"id":"sse-text","label":"SSE Text","capabilities":["text"],"scopes":["canvas"],"create":{"method":"POST","path":"/chat"},"response":{"textPaths":["choices.0.message.content"],"reasoningPaths":["choices.0.message.reasoning_content"],"usage":{"$ref":"response.usage"},"errorPaths":["error.code"],"messagePaths":["error.message"]}}]}
	}`)
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestManifestMergesSSETextDeltas(t *testing.T) {
	adapter := sseManifestAdapter(t)
	body := []byte(`data: {"id":"chat-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"你"}}]}

data: {"choices":[{"index":0,"delta":{"content":"好"}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}

data: [DONE]

`)
	created, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusSucceeded || created.Result == nil || created.Result.Text != "你好" {
		t.Fatalf("created = %#v", created)
	}
	if created.Result.Usage["completion_tokens"] != float64(2) {
		t.Fatalf("usage = %#v", created.Result.Usage)
	}
}

func TestManifestMergesSSEContentParts(t *testing.T) {
	adapter := sseManifestAdapter(t)
	body := []byte(`data: {"choices":[{"index":0,"delta":{"content":[{"type":"text","text":"第一段"},{"type":"text","text":"第二段"}]}}]}

data: [DONE]
`)
	created, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if created.Result == nil || created.Result.Text != "第一段第二段" {
		t.Fatalf("created = %#v", created)
	}
}

func TestManifestSSEWithoutTextReturnsClearError(t *testing.T) {
	adapter := sseManifestAdapter(t)
	body := []byte(`data: {"object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":270,"completion_tokens":0}}

data: [DONE]
`)
	_, err := adapter.ParseCreate(context.Background(), body)
	if err == nil || !strings.Contains(err.Error(), "没有正文") {
		t.Fatalf("error = %v, want empty stream body", err)
	}
}

func TestManifestSSEErrorFrameSurfacesUpstreamMessage(t *testing.T) {
	adapter := sseManifestAdapter(t)
	body := []byte(`data: {"error":{"code":"invalid_request","message":"model does not support image input"}}

data: [DONE]
`)
	created, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusFailed || !strings.Contains(created.Message, "does not support image input") {
		t.Fatalf("created = %#v", created)
	}
}

func TestMergeEventStreamPayloadIgnoresPlainJSON(t *testing.T) {
	if _, matched, err := mergeEventStreamPayload([]byte(`{"choices":[]}`)); matched || err != nil {
		t.Fatalf("matched=%v err=%v", matched, err)
	}
}
