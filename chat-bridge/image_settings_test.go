package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNegativeSettingsReachBothImageRoutes(t *testing.T) {
	for _, route := range []string{"/api/chat", "/api/generate"} {
		for _, negative := range []string{"glossy, shiny clothes, plastic", ""} {
			t.Run(route+"/"+negative, func(t *testing.T) {
				called := false
				mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/apply-template":
						io.WriteString(w, `{"prompt":"test"}`)
					case "/tokenize":
						io.WriteString(w, `{"tokens":[1]}`)
					case "/v1/chat/completions":
						io.WriteString(w, `{"choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"function":{"name":"generate_image","arguments":"{\"prompt\":\"hatsune_miku, dancing\"}"}}]}}]}`)
					case "/run":
						called = true
						var body map[string]any
						json.NewDecoder(r.Body).Decode(&body)
						if body["negative"] != negative || body["cfg"] != 1.5 || body["prompt"] != "hatsune_miku, dancing" {
							t.Error("settings changed before engine", body)
						}
						io.WriteString(w, "event: done\ndata: {\"failed\":[],\"results\":[{\"url\":\"/results/test/image.png\"}]}\n\n")
					case "/results/test/image.png":
						w.Write([]byte("\x89PNG\r\n\x1a\nfixture"))
					default:
						t.Error("unexpected request", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer mock.Close()
				data := t.TempDir()
				os.Mkdir(filepath.Join(data, "images"), 0700)
				b := &bridge{llm: mock.URL, nightmare: mock.URL, data: data, client: mock.Client()}
				body := map[string]any{"model": models[1].ID, "prompt": "hatsune_miku, dancing", "negative": negative, "cfg": 1.5, "messages": []map[string]string{{"role": "user", "content": "ミクを描いて"}}}
				raw, _ := json.Marshal(body)
				r := httptest.NewRequest("POST", "http://127.0.0.1"+route, strings.NewReader(string(raw)))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Chat-Client", "1")
				w := httptest.NewRecorder()
				b.handler().ServeHTTP(w, r)
				var result map[string]any
				lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
				json.Unmarshal([]byte(lines[len(lines)-1]), &result)
				if !called || w.Code != 200 || result["type"] != "done" || result["negative"] != negative || result["cfg"] != 1.5 {
					t.Fatal(w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestImageSettingsDefaultsAndInvalidRequests(t *testing.T) {
	defaults, err := (imageSettingsInput{}).resolve()
	if err != nil || defaults.CFG != 1 || defaults.Negative != "low quality, blurry, text, watermark, bad anatomy" {
		t.Fatal(defaults, err)
	}
	for _, route := range []string{"/api/chat", "/api/generate"} {
		for _, settings := range []string{`"cfg":0`, `"cfg":3.1`, `"cfg":"1.5"`, `"negative":"` + strings.Repeat("a", 3001) + `"`} {
			body := `{"model":"` + models[1].ID + `","prompt":"test","messages":[{"role":"user","content":"test"}],` + settings + `}`
			r := httptest.NewRequest("POST", "http://127.0.0.1"+route, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Chat-Client", "1")
			w := httptest.NewRecorder()
			(&bridge{}).handler().ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(route, settings, w.Code)
			}
		}
	}
}
