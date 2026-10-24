package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBoltConfigAndRequestOverride(t *testing.T) {
	t.Setenv("ATELIER_LLM_PROFILE", "bolt")
	t.Setenv("ATELIER_LLM_CONTEXT", "")
	t.Setenv("ATELIER_LLM_THINKING", "")
	c, err := inferenceFromEnv()
	if err != nil || c.Context != 8192 || c.Thinking {
		t.Fatal(c, err)
	}
	b := &bridge{inference: c}
	msgs := conversation(models[1], []clientMessage{{Role: "user", Content: "こんにちは"}})
	for _, on := range []bool{false, true} {
		body := b.chatPayload(msgs, &on)
		kwargs := body["chat_template_kwargs"].(map[string]bool)
		if kwargs["enable_thinking"] != on || !kwargs["conversational_mode"] || body["messages"].([]message)[0].Content != systemPrompt(models[1]) {
			t.Fatal(body)
		}
	}
	t.Setenv("ATELIER_LLM_PROFILE", "unknown")
	if _, err := inferenceFromEnv(); err == nil {
		t.Fatal("unknown profile accepted")
	}
}

func TestBoltReceivesRealToolResult(t *testing.T) {
	completions := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/apply-template":
			io.WriteString(w, `{"prompt":"native Bolt template"}`)
		case "/tokenize":
			io.WriteString(w, `{"tokens":[1,2,3]}`)
		case "/v1/chat/completions":
			completions++
			msgs := body["messages"].([]any)
			call := msgs[len(msgs)-2].(map[string]any)
			result := msgs[len(msgs)-1].(map[string]any)
			if call["role"] != "assistant" || result["role"] != "tool" || result["tool_call_id"] != "real-call-id" || body["tool_choice"] != "none" {
				t.Fatal(body)
			}
			var content map[string]string
			if json.Unmarshal([]byte(result["content"].(string)), &content) != nil || content["status"] != "success" || content["image"] != "/images/0123456789abcdef01234567.png" {
				t.Fatal(result)
			}
			io.WriteString(w, `{"choices":[{"message":{"content":"雪の中のキツネが描けました。"}}]}`)
		default:
			t.Error("unexpected request", r.URL.Path)
		}
	}))
	defer backend.Close()
	b := &bridge{llm: backend.URL, client: backend.Client(), inference: inferenceConfig{Profile: "bolt", Context: 8192}}
	msgs := conversation(models[1], []clientMessage{{Role: "user", Content: "雪の中のキツネを描いて"}})
	text, err := b.afterImage(t.Context(), msgs, "real-call-id", "fox, snow", "/images/0123456789abcdef01234567.png", nil)
	if err != nil || text != "雪の中のキツネが描けました。" || completions != 1 {
		t.Fatal(text, err, completions)
	}
}

func TestContextDoesNotOrphanToolResult(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apply-template" {
			io.WriteString(w, `{"prompt":"long"}`)
		} else {
			json.NewEncoder(w).Encode(map[string]any{"tokens": make([]int, 2000)})
		}
	}))
	defer backend.Close()
	b := &bridge{llm: backend.URL, client: backend.Client()}
	_, _, err := b.fit(t.Context(), []message{{Role: "system"}, {Role: "user"}, {Role: "assistant"}, {Role: "tool"}})
	if err == nil {
		t.Fatal("over-limit current tool turn must fail without trimming its assistant call")
	}
}
