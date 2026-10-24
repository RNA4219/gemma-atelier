package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func workflowFixture() workflowGraph {
	sample := func(id, model, prompt string) workflowNode {
		return workflowNode{ID: id, Type: "sdxl.sample", Params: map[string]string{"model": model, "steps": "8", "cfg": "1.5", "seed": "104729", "denoise": "0.35", "scheduler": "lcm", "width": "1024", "height": "1024", "aspect": "1:1"}, Inputs: map[string]string{"prompt": prompt}}
	}
	g := workflowGraph{Format: 1, Nodes: []workflowNode{
		{ID: "words", Type: "core.prompt", Params: map[string]string{"prompt": "red fox, watercolor", "negative": "glossy"}, Inputs: map[string]string{}},
		sample("draw", models[1].ID, "words"),
		{ID: "first", Type: "core.output", Params: map[string]string{}, Inputs: map[string]string{"media": "draw"}},
		{ID: "finish_words", Type: "core.prompt", Params: map[string]string{"prompt": "red fox, soft matte texture", "negative": ""}, Inputs: map[string]string{}},
		sample("finish", models[0].ID, "finish_words"),
		{ID: "second", Type: "core.output", Params: map[string]string{}, Inputs: map[string]string{"media": "finish"}},
	}}
	g.Nodes[4].Inputs["image"] = "draw"
	return g
}
func verifiedFixtures() []installedModel {
	return []installedModel{{ID: models[0].ID, Family: "SDXL", Supported: true}, {ID: models[1].ID, Family: "SDXL", Supported: true}}
}
func TestWorkflowRejectsInvalidGraphs(t *testing.T) {
	if err := validateWorkflow(workflowFixture(), verifiedFixtures()); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*workflowGraph){
		"cycle":            func(g *workflowGraph) { g.Nodes[1].Inputs["image"] = "finish" },
		"wrong-port":       func(g *workflowGraph) { g.Nodes[1].Inputs["prompt"] = "first" },
		"duplicate":        func(g *workflowGraph) { g.Nodes[4].ID = "draw" },
		"unknown-model":    func(g *workflowGraph) { g.Nodes[1].Params["model"] = "cpu-model" },
		"nan":              func(g *workflowGraph) { g.Nodes[1].Params["cfg"] = "NaN" },
		"float-seed":       func(g *workflowGraph) { g.Nodes[1].Params["seed"] = "1e3" },
		"int32-overflow":   func(g *workflowGraph) { g.Nodes[1].Params["seed"] = "2147483648" },
		"missing-negative": func(g *workflowGraph) { delete(g.Nodes[0].Params, "negative") },
		"unknown-param":    func(g *workflowGraph) { g.Nodes[1].Params["loras"] = "unsupported.safetensors" },
		"unknown-node":     func(g *workflowGraph) { g.Nodes[1].Type = "CheckpointLoaderSimple" },
		"orphan":           func(g *workflowGraph) { g.Nodes[4].Inputs["prompt"] = "words" },
	} {
		t.Run(name, func(t *testing.T) {
			g := workflowFixture()
			change(&g)
			if validateWorkflow(g, verifiedFixtures()) == nil {
				t.Fatal("invalid graph accepted")
			}
		})
	}
}
func TestWorkflowRunsMultipleModelsAndOutputs(t *testing.T) {
	for _, mode := range []string{"success", "partial", "unsafe-result", "missing-output", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			runs, downloads := 0, 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer private-workflow-token" {
					t.Error("missing token")
				}
				switch r.URL.Path {
				case "/models":
					json.NewEncoder(w).Encode(map[string]any{"models": verifiedFixtures()})
				case "/run":
					runs++
					var body struct {
						Graph workflowGraph `json:"graph"`
						Model string        `json:"model"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body.Model != "" || body.Graph.Nodes[1].Params["model"] != models[1].ID || body.Graph.Nodes[4].Params["model"] != models[0].ID || body.Graph.Nodes[4].Inputs["image"] != "draw" {
						t.Error("model or image connection changed")
					}
					if body.Graph.Nodes[0].Params["negative"] != "glossy" || body.Graph.Nodes[3].Params["negative"] != "" || body.Graph.Nodes[4].Params["cfg"] != "1.5" {
						t.Error("settings changed")
					}
					if seed, err := strconv.ParseInt(body.Graph.Nodes[1].Params["seed"], 10, 32); err != nil || seed <= 0 {
						t.Error("random seed is not a positive Kotlin Int")
					}
					answer := `{"failed":[],"results":[{"node":"first","url":"/results/test/first.png"},{"node":"second","url":"/results/test/second.png"}]}`
					if mode == "partial" {
						answer = `{"failed":["finish"],"results":[{"node":"first","url":"/results/test/first.png"}]}`
					}
					if mode == "unsafe-result" {
						answer = `{"results":[{"node":"first","url":"/results/test/../token.png"}]}`
					}
					if mode == "missing-output" {
						answer = `{"results":[{"node":"first","url":"/results/test/first.png"}]}`
					}
					fmt.Fprintf(w, "event: node\ndata: {\"node\":\"draw\"}\n\nevent: done\ndata: %s\n\n", answer)
				case "/results/test/first.png", "/results/test/second.png":
					downloads++
					w.Write([]byte("\x89PNG\r\n\x1a\nfixture"))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			}))
			defer backend.Close()
			data := t.TempDir()
			os.Mkdir(filepath.Join(data, "images"), 0700)
			b := &bridge{nightmare: backend.URL, token: "private-workflow-token", data: data, client: backend.Client()}
			if mode == "conflict" {
				b.busy.Lock()
				defer b.busy.Unlock()
			}
			g := workflowFixture()
			g.Nodes[1].Params["seed"] = "0"
			raw, _ := json.Marshal(map[string]any{"graph": g})
			r := httptest.NewRequest("POST", "http://127.0.0.1:8085/api/workflows/run", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Chat-Client", "1")
			w := httptest.NewRecorder()
			b.handler().ServeHTTP(w, r)
			if mode == "conflict" {
				if w.Code != 409 || runs != 0 {
					t.Fatal(w.Code, runs)
				}
				return
			}
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-workflow-token") {
				t.Fatal("token leaked")
			}
			var done map[string]json.RawMessage
			hasError := false
			for _, line := range strings.Split(strings.TrimSpace(w.Body.String()), "\n") {
				var event map[string]json.RawMessage
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Fatal(err)
				}
				if string(event["type"]) == `"done"` {
					done = event
				}
				if string(event["type"]) == `"error"` {
					hasError = true
				}
			}
			if mode == "success" {
				if done == nil || hasError || downloads != 2 {
					t.Fatal(w.Body.String(), downloads)
				}
				var images []renderedImage
				json.Unmarshal(done["images"], &images)
				if len(images) != 2 {
					t.Fatal(images)
				}
				for _, image := range images {
					if _, err := os.Stat(filepath.Join(data, strings.TrimPrefix(image.Image, "/"))); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				if done != nil || !hasError {
					t.Fatal(w.Body.String())
				}
				if mode != "missing-output" && downloads != 0 {
					t.Fatal("downloaded failed or unsafe result")
				}
			}
		})
	}
}
func TestWorkflowCatalogDoesNotTrustSupportedFlag(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"id":"unknown","family":"SDXL","supported":true}]}`)
	}))
	defer backend.Close()
	b := &bridge{nightmare: backend.URL, client: backend.Client()}
	list, err := b.workflowModels(t.Context())
	if err != nil || len(list) != 1 || list[0].Supported {
		t.Fatal(list, err)
	}
}
