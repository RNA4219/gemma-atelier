package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTagsCanonicalizeAliasesAndKeepUnlisted(t *testing.T) {
	tags, unlisted, err := normalizeTags([]string{"1girls", "1girl", "looking at viewer", "moonlit_cafe_scene"})
	if err != nil || !reflect.DeepEqual(tags, []string{"1girl", "looking_at_viewer", "moonlit_cafe_scene"}) || !reflect.DeepEqual(unlisted, []string{"moonlit_cafe_scene"}) {
		t.Fatal(tags, unlisted, err)
	}
}

func TestTagConversionDoesNotGenerateImage(t *testing.T) {
	for _, tooLong := range []bool{false, true} {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/apply-template":
				io.WriteString(w, `{"prompt":"template"}`)
			case "/tokenize":
				n := 120
				if tooLong {
					n = 1600
				}
				json.NewEncoder(w).Encode(map[string]any{"tokens": make([]int, n)})
			case "/v1/chat/completions":
				if tooLong {
					t.Error("overlong input reached generation")
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["response_format"] == nil || body["tools"] != nil {
					t.Error("missing JSON constraint or unexpected image tools")
				}
				messages := body["messages"].([]any)
				if messages[1].(map[string]any)["content"] != "hatsune_mikuが笑顔で手を振っている" {
					t.Error("Japanese character name was not canonicalized", messages[1])
				}
				io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"tags\":[\"1girls\",\"blonde_hair\",\"purple eyes\"]}"}}]}`)
			default:
				t.Error("unexpected backend call", r.URL.Path)
				http.NotFound(w, r)
			}
		}))
		b := &bridge{llm: mock.URL, nightmare: mock.URL, client: mock.Client()}
		r := httptest.NewRequest("POST", "http://127.0.0.1/api/tags", strings.NewReader(`{"text":"初音ミクが笑顔で手を振っている"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Chat-Client", "1")
		w := httptest.NewRecorder()
		b.handler().ServeHTTP(w, r)
		if tooLong {
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		} else {
			var body struct {
				Prompt string `json:"prompt"`
			}
			json.Unmarshal(w.Body.Bytes(), &body)
			if w.Code != 200 || body.Prompt != "1girl, blonde_hair, purple_eyes" {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		mock.Close()
	}
}

func TestReviewedTagsReachImageEngineUnchanged(t *testing.T) {
	prompt := "1girl, (purple_eyes:1.2), black_ribbon"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/run":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["prompt"] != prompt {
				t.Error("reviewed tags changed", body["prompt"])
			}
			io.WriteString(w, "event: done\ndata: {\"failed\":[],\"results\":[{\"url\":\"/results/test/output.png\"}]}\n\n")
		case "/results/test/output.png":
			w.Write([]byte("\x89PNG\r\n\x1a\nfixture"))
		default:
			t.Error("reviewed tags must not call Gemma", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()
	data := t.TempDir()
	os.Mkdir(filepath.Join(data, "images"), 0700)
	b := &bridge{llm: mock.URL, nightmare: mock.URL, client: mock.Client(), data: data}
	raw, _ := json.Marshal(map[string]string{"model": models[1].ID, "prompt": prompt})
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/generate", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Chat-Client", "1")
	w := httptest.NewRecorder()
	b.handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"prompt_source":"tags"`) || !strings.Contains(w.Body.String(), `"type":"done"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestTagConversionRejectsMalformedOutput(t *testing.T) {
	for _, input := range [][]string{nil, {""}, {"日本語"}, {"1girl, solo"}} {
		if _, _, err := normalizeTags(input); err == nil {
			t.Fatal("accepted malformed tags", input)
		}
	}
}
