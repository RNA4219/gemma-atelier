package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationAndImagePipeline(t *testing.T) {
	for _, imageMode := range []bool{false, true} {
		t.Run(fmt.Sprint(imageMode), func(t *testing.T) {
			var prompt, selected string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/apply-template":
					io.WriteString(w, `{"prompt":"rendered template"}`)
				case "/tokenize":
					io.WriteString(w, `{"tokens":[1,2]}`)
				case "/v1/chat/completions":
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["tools"] == nil {
						t.Error("missing function definition")
					}
					if imageMode {
						io.WriteString(w, `{"choices":[{"finish_reason":"tool_calls","message":{"content":"描きます。","tool_calls":[{"function":{"name":"generate_image","arguments":"{\"prompt\":\"red fox, snowy forest, watercolor\"}"}}]}}]}`)
					} else {
						io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"こんにちは。"}}]}`)
					}
				case "/run":
					if r.Header.Get("Authorization") != "Bearer private-test-token" {
						t.Error("missing server credential")
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					prompt = body["prompt"].(string)
					selected = body["model"].(string)
					if body["scheduler"] != "lcm" || body["steps"] != float64(8) {
						t.Error("wrong DMD2 defaults")
					}
					io.WriteString(w, "event: progress\ndata: {\"step\":1,\"total\":8}\n\nevent: done\ndata: {\"failed\":[],\"results\":[{\"url\":\"/results/run1/output.png\"}]}\n\n")
				case "/results/run1/output.png":
					w.Write([]byte("\x89PNG\r\n\x1a\ntest fixture"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer backend.Close()
			data := t.TempDir()
			os.Mkdir(filepath.Join(data, "images"), 0700)
			b := &bridge{llm: backend.URL, nightmare: backend.URL, token: "private-test-token", data: data, client: backend.Client()}
			req := httptest.NewRequest("POST", "http://127.0.0.1:8085/api/chat", strings.NewReader(`{"messages":[{"role":"user","content":"キツネを描いて"}],"model":"Illustrious-v16-DMD2-SDXL-8Gen3"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Chat-Client", "1")
			rec := httptest.NewRecorder()
			b.handler().ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatal(rec.Code, rec.Body.String())
			}
			var done map[string]any
			for _, line := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n") {
				var e map[string]any
				if err := json.Unmarshal([]byte(line), &e); err != nil {
					t.Fatal(err)
				}
				if e["type"] == "error" {
					t.Fatal(e)
				}
				if e["type"] == "done" {
					done = e
				}
			}
			if done == nil {
				t.Fatal("no done event")
			}
			if strings.Contains(rec.Body.String(), "private-test-token") {
				t.Fatal("credential leaked")
			}
			if imageMode {
				if prompt != "red fox, snowy forest, watercolor" || selected != models[1].ID {
					t.Fatal(prompt, selected)
				}
				name := strings.TrimPrefix(done["image"].(string), "/images/")
				if _, err := os.Stat(filepath.Join(data, "images", name)); err != nil {
					t.Fatal(err)
				}
			} else if done["text"] != "こんにちは。" {
				t.Fatal(done)
			}
		})
	}
}
func TestRejectCrossOriginAndUnknownModel(t *testing.T) {
	b := &bridge{}
	for _, test := range []struct {
		host, origin, body string
		status             int
	}{
		{"attacker.example", "", "{}", 403},
		{"127.0.0.1:8085", "https://attacker.example", "{}", 403},
		{"127.0.0.1:8085", "", `{"model":"unknown","messages":[{"role":"user","content":"hello"}]}`, 400},
	} {
		r := httptest.NewRequest("POST", "http://"+test.host+"/api/chat", strings.NewReader(test.body))
		r.Header.Set("Origin", test.origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Chat-Client", "1")
		w := httptest.NewRecorder()
		b.handler().ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatal(w.Code, test)
		}
	}
}
func TestContextDropsWholeOldTurn(t *testing.T) {
	count := 0
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apply-template" {
			io.WriteString(w, `{"prompt":"test"}`)
			return
		}
		count++
		n := 100
		if count == 1 {
			n = 1800
		}
		json.NewEncoder(w).Encode(map[string]any{"tokens": make([]int, n)})
	}))
	defer mock.Close()
	b := &bridge{llm: mock.URL, client: mock.Client()}
	got, trimmed, err := b.fit(t.Context(), []message{{Role: "system", Content: "system"}, {Role: "user", Content: "old"}, {Role: "assistant", Content: "old reply"}, {Role: "user", Content: "new"}})
	if err != nil || !trimmed || len(got) != 2 || got[1].Content != "new" {
		t.Fatal(got, trimmed, err)
	}
}
func TestImageHistoryKeepsNativeToolCall(t *testing.T) {
	msgs := conversation(models[1], []clientMessage{{Role: "user", Content: "キツネを描いて"}, {Role: "assistant", Content: "画像を生成しました。", Prompt: "red fox, snowy forest", Image: "/images/0123456789abcdef01234567.png"}, {Role: "user", Content: "夕焼けにして"}})
	if len(msgs) != 6 || msgs[2].Role != "assistant" || len(msgs[2].ToolCalls) != 1 || msgs[3].Role != "tool" || msgs[3].ToolCallID != "image_1" || msgs[5].Content != "夕焼けにして" {
		t.Fatal(msgs)
	}
	args := msgs[2].ToolCalls[0]["function"].(map[string]string)["arguments"]
	if !strings.Contains(args, "red fox, snowy forest") {
		t.Fatal(args)
	}
}
func TestFailedImageDoesNotReportSuccess(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "event: done\ndata: {\"failed\":[\"sampler\"],\"results\":[]}\n\n")
	}))
	defer mock.Close()
	b := &bridge{nightmare: mock.URL, client: mock.Client()}
	w := httptest.NewRecorder()
	image, err := b.generate(t.Context(), models[0], "test", imageSettings{CFG: 1}, eventWriter{json.NewEncoder(w), w})
	if err == nil || image != "" {
		t.Fatal(image, err)
	}
}
