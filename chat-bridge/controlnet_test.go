package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngFixture(w, h int) []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

func TestControlNetImageUploadAndPayload(t *testing.T) {
	b := &bridge{data: t.TempDir()}
	os.Mkdir(filepath.Join(b.data, "images"), 0700)
	for _, tc := range []struct {
		name string
		raw  []byte
		code int
	}{
		{"png", pngFixture(32, 48), 200}, {"oversize", pngFixture(1025, 1), 400}, {"corrupt", []byte("not an image"), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"png": base64.StdEncoding.EncodeToString(tc.raw)})
			r := httptest.NewRequest("POST", "http://127.0.0.1:8085/api/workflows/image", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Chat-Client", "1")
			w := httptest.NewRecorder()
			b.handler().ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if tc.code != 200 {
				return
			}
			var result struct {
				Image string `json:"image"`
			}
			json.Unmarshal(w.Body.Bytes(), &result)
			g := workflowFixture()
			g.Nodes = g.Nodes[:3]
			g.Nodes[1].Params["model"] = swapModel
			g.Nodes[1].Params["controlnet"] = "openpose"
			g.Nodes[1].Params["control_strength"] = "0.8"
			g.Nodes[1].Inputs["control"] = "ref"
			g.Nodes = append(g.Nodes, workflowNode{ID: "ref", Type: "core.image", Params: map[string]string{"image": result.Image}, Inputs: map[string]string{}})
			payload, err := b.workflowPayload(g)
			if err != nil {
				t.Fatal(err)
			}
			native := payload["graph"].(workflowGraph)
			if native.Nodes[1].Type != "sdxlswap.sample" || native.Nodes[1].Inputs["control"] != "ref" || len(native.Nodes[3].Params) != 0 || g.Nodes[3].Params["image"] != result.Image {
				t.Fatal("image path leaked or saved graph changed")
			}
			if payload["images"].(map[string]string)["ref"] == "" {
				t.Fatal("no image sent to engine")
			}
			os.Remove(filepath.Join(b.data, "images", strings.TrimPrefix(result.Image, "/images/")))
			if _, err = b.workflowPayload(g); err == nil {
				t.Fatal("missing input accepted")
			}
		})
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8085/api/workflows/image", strings.NewReader(`{"png":""}`))
	w := httptest.NewRecorder()
	b.handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("missing client header accepted")
	}
}

func TestControlNetGraphBoundaries(t *testing.T) {
	fixture := func() workflowGraph {
		g := workflowFixture()
		g.Nodes = g.Nodes[:3]
		g.Nodes[1].Params["model"] = swapModel
		g.Nodes[1].Params["controlnet"] = "canny"
		g.Nodes[1].Params["control_strength"] = "1"
		g.Nodes[1].Inputs["control"] = "ref"
		return workflowGraph{Format: 1, Nodes: append(g.Nodes, workflowNode{ID: "ref", Type: "core.image", Params: map[string]string{"image": "/images/0123456789abcdef01234567.png"}, Inputs: map[string]string{}})}
	}
	installed := append(verifiedFixtures(), installedModel{ID: swapModel, Family: "SDXL_SWAP", Supported: true})
	for _, kind := range []string{"canny", "depth", "openpose"} {
		g := fixture()
		g.Nodes[1].Params["controlnet"] = kind
		g.Nodes[1].Params["loras"] = "Wada Arco[style]-Illus.safetensors@0.7"
		if err := validateWorkflow(g, installed); err != nil {
			t.Fatal(err)
		}
	}
	for name, change := range map[string]func(*workflowGraph){
		"normal-model":      func(g *workflowGraph) { g.Nodes[1].Params["model"] = models[0].ID },
		"missing-image":     func(g *workflowGraph) { delete(g.Nodes[1].Inputs, "control") },
		"unsupported-type":  func(g *workflowGraph) { g.Nodes[1].Params["controlnet"] = "tile" },
		"nan":               func(g *workflowGraph) { g.Nodes[1].Params["control_strength"] = "NaN" },
		"zero":              func(g *workflowGraph) { g.Nodes[1].Params["control_strength"] = "0" },
		"negative":          func(g *workflowGraph) { g.Nodes[1].Params["control_strength"] = "-1" },
		"large":             func(g *workflowGraph) { g.Nodes[1].Params["control_strength"] = "2.1" },
		"file-uri":          func(g *workflowGraph) { g.Nodes[3].Params = map[string]string{"uri": "file:///secret"} },
		"remote-image":      func(g *workflowGraph) { g.Nodes[3].Params["image"] = "https://example.com/image.png" },
		"prompt-as-control": func(g *workflowGraph) { g.Nodes[1].Inputs["control"] = "words" },
		"cycle":             func(g *workflowGraph) { g.Nodes[1].Inputs["control"] = "draw" },
	} {
		t.Run(name, func(t *testing.T) {
			g := fixture()
			change(&g)
			if validateWorkflow(g, installed) == nil {
				t.Fatal("invalid graph accepted")
			}
		})
	}
}

func TestUnavailableControlNetDoesNotRunEngine(t *testing.T) {
	runs := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			json.NewEncoder(w).Encode(map[string]any{"models": []installedModel{{ID: swapModel, Family: "SDXL_SWAP"}}})
			return
		}
		if r.URL.Path == "/run" {
			runs++
		}
		http.Error(w, "must not execute", 500)
	}))
	defer backend.Close()
	b := &bridge{nightmare: backend.URL, client: backend.Client()}
	g := workflowFixture()
	g.Nodes = g.Nodes[:3]
	g.Nodes[1].Params["model"] = swapModel
	g.Nodes[1].Params["controlnet"] = "canny"
	g.Nodes[1].Params["control_strength"] = "0.8"
	g.Nodes[1].Inputs["control"] = "ref"
	g.Nodes = append(g.Nodes, workflowNode{ID: "ref", Type: "core.image", Params: map[string]string{"image": "/images/0123456789abcdef01234567.png"}, Inputs: map[string]string{}})
	body, _ := json.Marshal(map[string]any{"graph": g})
	r := httptest.NewRequest("POST", "http://127.0.0.1:8085/api/workflows/run", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Chat-Client", "1")
	w := httptest.NewRecorder()
	b.handler().ServeHTTP(w, r)
	if w.Code != 503 || runs != 0 || !strings.Contains(w.Body.String(), "準備中") {
		t.Fatalf("code %d runs %d: %s", w.Code, runs, w.Body.String())
	}
}

func TestSequentialControlRequiresExactBackend(t *testing.T) {
	for _, version := range []string{sequentialEngineVersion, "1.6.211", "", "1.6.211-atelier-cnseq0"} {
		t.Run(version, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]string{"version": version})
			}))
			defer s.Close()
			b := &bridge{nightmare: s.URL, client: s.Client()}
			if got := b.sequentialControlReady(context.Background()); got != (version == sequentialEngineVersion) {
				t.Fatalf("version=%q ready=%v", version, got)
			}
		})
	}
}
