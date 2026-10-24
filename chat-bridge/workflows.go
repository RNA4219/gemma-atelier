package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Nightmareのformat 1のうち、端末でNPU確認済みの画像ノードだけを受け付ける。
type workflowNode struct {
	ID     string            `json:"id"`
	Type   string            `json:"type"`
	Params map[string]string `json:"params"`
	Inputs map[string]string `json:"inputs"`
	X      float64           `json:"x"`
	Y      float64           `json:"y"`
}
type workflowGraph struct {
	Format int            `json:"format"`
	Nodes  []workflowNode `json:"nodes"`
}
type installedModel struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Family    string   `json:"family"`
	Features  []string `json:"features"`
	Supported bool     `json:"supported"`
	Steps     int      `json:"steps"`
	CFG       float64  `json:"cfg"`
	Scheduler string   `json:"scheduler"`
}
type renderedImage struct {
	Node  string `json:"node"`
	Image string `json:"image"`
}

var workflowID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,47}$`)
var resultPath = regexp.MustCompile(`^/results/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+\.png$`)

func (b *bridge) workflowModels(ctx context.Context) ([]installedModel, error) {
	r, err := b.request(ctx, "GET", b.nightmare, "/models", nil, true)
	if err != nil {
		return nil, errors.New("Nightmareに接続できません")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("モデル一覧 HTTP %d", r.StatusCode)
	}
	var data struct {
		Models []installedModel `json:"models"`
	}
	if err = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&data); err != nil {
		return nil, errors.New("モデル一覧を読み取れません")
	}
	for i := range data.Models {
		data.Models[i].Supported = false
		if data.Models[i].ID == swapModel && data.Models[i].Family == "SDXL_SWAP" {
			data.Models[i].Supported = true
			data.Models[i].Label = "LoRA対応 · rinFlanime Illustrious"
		}
		for _, verified := range models {
			if data.Models[i].ID == verified.ID && data.Models[i].Family == "SDXL" {
				data.Models[i].Supported = true
				data.Models[i].Label = verified.Label
			}
		}
	}
	return data.Models, nil
}
func (b *bridge) workflowCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GETを使用してください", 405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	installed, err := b.workflowModels(ctx)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"models": installed, "format": 1, "max_generators": 4,
		"adapters":   localAdapters(),
		"controlnet": map[string]any{"enabled": b.sequentialControlReady(ctx), "reason": controlNetUnavailable},
		"note":       "手元のSDXL LoRAはrinFlanimeのSwapモデルで選択できます。テキストエンコーダ用LoRA・一部の層は適用対象外。VAEはモデル内蔵のNPU版を使います。"})
}

func validateWorkflow(g workflowGraph, installed []installedModel) error {
	if g.Format != 1 || len(g.Nodes) < 3 || len(g.Nodes) > 12 {
		return errors.New("形式1・3〜12ノードのワークフローを指定してください")
	}
	byID := map[string]workflowNode{}
	available := map[string]bool{}
	for _, m := range installed {
		available[m.ID] = m.Supported
	}
	counts := map[string]int{}
	for _, n := range g.Nodes {
		if !workflowID.MatchString(n.ID) {
			return errors.New("ノードIDは英数字・ハイフン・下線、先頭は英字にしてください")
		}
		if _, exists := byID[n.ID]; exists {
			return fmt.Errorf("ノードIDが重複しています: %s", n.ID)
		}
		if math.IsNaN(n.X) || math.IsNaN(n.Y) || math.IsInf(n.X, 0) || math.IsInf(n.Y, 0) || n.X < 0 || n.Y < 0 || n.X > 10000 || n.Y > 10000 {
			return fmt.Errorf("位置が不正です: %s", n.ID)
		}
		allowedParams, allowedInputs := map[string]bool{}, map[string]bool{}
		switch n.Type {
		case "core.prompt":
			if strings.Contains(strings.ToLower(n.Params["prompt"]+n.Params["negative"]), "<lora:") {
				return fmt.Errorf("%s: LoRAは画像生成ノードの選択欄で指定してください", n.ID)
			}
			allowedParams = map[string]bool{"prompt": true, "negative": true}
			if strings.TrimSpace(n.Params["prompt"]) == "" || len(n.Params["prompt"]) > 3000 || len(n.Params["negative"]) > 3000 {
				return fmt.Errorf("%s: プロンプトを入力してください（各3000バイト以内）", n.ID)
			}
			if _, ok := n.Params["negative"]; !ok {
				return fmt.Errorf("%s: ネガティブを指定してください。空欄も使えます", n.ID)
			}
		case "sdxl.sample":
			allowedParams = map[string]bool{"model": true, "steps": true, "cfg": true, "seed": true, "denoise": true, "scheduler": true, "width": true, "height": true, "aspect": true, "loras": true, "controlnet": true, "control_strength": true}
			if err := validateControl(n); err != nil {
				return fmt.Errorf("%s: %w", n.ID, err)
			}
			if err := validateLoras(n.Params["loras"], n.Params["model"]); err != nil {
				return fmt.Errorf("%s: %w", n.ID, err)
			}
			allowedInputs = map[string]bool{"prompt": true, "image": true, "control": true}
			if !available[n.Params["model"]] {
				return fmt.Errorf("%s: この端末でNPU確認済み・導入済みのモデルを選んでください", n.ID)
			}
			for _, key := range []string{"steps", "cfg", "seed", "denoise", "width", "height"} {
				value, err := strconv.ParseFloat(n.Params[key], 64)
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
					return fmt.Errorf("%s: %sは有限の数値で指定してください", n.ID, key)
				}
				lo, hi, integer := 0.0, 1.0, false
				switch key {
				case "steps":
					lo, hi, integer = 1, 30, true
					if n.Params["model"] == swapModel {
						hi = 40
					}
				case "cfg":
					lo, hi = 1, 3
					if n.Params["model"] == swapModel {
						hi = 10
					}
				case "seed":
					lo, hi, integer = 0, 2147483647, true
				case "width", "height":
					lo, hi, integer = 1024, 1024, true
				}
				if value < lo || value > hi || (integer && value != math.Trunc(value)) {
					return fmt.Errorf("%s: %sの範囲が不正です", n.ID, key)
				}
				if integer {
					if _, err := strconv.ParseUint(n.Params[key], 10, 32); err != nil {
						return fmt.Errorf("%s: %sは整数で指定してください", n.ID, key)
					}
				}
			}
			schedulerOK := n.Params["scheduler"] == "lcm"
			if n.Params["model"] == swapModel {
				schedulerOK = schedulerOK || n.Params["scheduler"] == "euler_a" || n.Params["scheduler"] == "euler" || n.Params["scheduler"] == "dpm"
			}
			if !schedulerOK || n.Params["aspect"] != "1:1" {
				return fmt.Errorf("%s: モデルに対応するサンプラー・1024角を指定してください", n.ID)
			}
			if n.Inputs["prompt"] == "" {
				return fmt.Errorf("%s: プロンプトを接続してください", n.ID)
			}
		case "core.image":
			allowedParams = map[string]bool{"image": true}
			if !storedImagePath.MatchString(n.Params["image"]) {
				return fmt.Errorf("%s: 参照画像を選んでください", n.ID)
			}
		case "core.output":
			allowedInputs = map[string]bool{"media": true}
			if n.Inputs["media"] == "" {
				return fmt.Errorf("%s: 出力する画像を接続してください", n.ID)
			}
		default:
			return fmt.Errorf("未対応のノードです: %s (%s)。ComfyUIのJSONとは別形式です", n.ID, n.Type)
		}
		for key := range n.Params {
			if !allowedParams[key] {
				return fmt.Errorf("%s: 未対応の設定 %s", n.ID, key)
			}
		}
		for key := range n.Inputs {
			if !allowedInputs[key] {
				return fmt.Errorf("%s: 未対応の入力 %s", n.ID, key)
			}
		}
		byID[n.ID] = n
		counts[n.Type]++
	}
	if counts["sdxl.sample"] < 1 || counts["sdxl.sample"] > 4 || counts["core.output"] < 1 || counts["core.output"] > 4 {
		return errors.New("生成・出力はそれぞれ1〜4ノードにしてください")
	}
	for _, n := range g.Nodes {
		for port, id := range n.Inputs {
			from, ok := byID[id]
			if !ok {
				return fmt.Errorf("%s: 接続先 %s がありません", n.ID, id)
			}
			want := "sdxl.sample"
			if port == "prompt" {
				want = "core.prompt"
			}
			if port == "control" && from.Type == "core.image" {
				want = "core.image"
			}
			if port == "image" && from.Type == "core.image" {
				want = "core.image"
			}
			if from.Type != want {
				return fmt.Errorf("%s: %sに接続できない種類です", n.ID, port)
			}
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return errors.New("接続が循環しています。前の段階から次の段階へ接続してください")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, from := range byID[id].Inputs {
			if err := visit(from); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, n := range g.Nodes {
		if n.Type == "core.output" {
			if err := visit(n.ID); err != nil {
				return err
			}
		}
	}
	if len(state) != len(g.Nodes) {
		return errors.New("出力につながっていないノードがあります。接続するか削除してください")
	}
	return nil
}

func (b *bridge) runWorkflow(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var body struct {
		Graph workflowGraph `json:"graph"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 96<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "ワークフローの形式を確認してください", 400)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "JSONは1件だけ送信してください", 400)
		return
	}
	if !b.busy.TryLock() {
		http.Error(w, "処理中です。完了してから操作してください", 409)
		return
	}
	defer b.busy.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Minute)
	defer cancel()
	lookup, stopLookup := context.WithTimeout(ctx, 5*time.Second)
	installed, err := b.workflowModels(lookup)
	stopLookup()
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if err = validateWorkflow(body.Graph, installed); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if usesControlNet(body.Graph) && !b.sequentialControlReady(ctx) {
		http.Error(w, controlNetUnavailable, http.StatusServiceUnavailable)
		return
	}
	for i := range body.Graph.Nodes {
		n := &body.Graph.Nodes[i]
		if n.Type == "sdxl.sample" && n.Params["seed"] == "0" {
			var raw [4]byte
			if _, err := rand.Read(raw[:]); err != nil {
				http.Error(w, "seedを作成できません", 500)
				return
			}
			// Nightmare reads seed with Kotlin toIntOrNull; keep it in signed Int range.
			seed := uint64(raw[0]&0x7f)<<24 | uint64(raw[1])<<16 | uint64(raw[2])<<8 | uint64(raw[3])
			if seed == 0 {
				seed = 1
			}
			n.Params["seed"] = strconv.FormatUint(seed, 10)
		}
	}
	payload, err := b.workflowPayload(body.Graph)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fl, _ := w.(http.Flusher)
	e := eventWriter{json.NewEncoder(w), fl}
	start := time.Now()
	e.send("status", map[string]any{"message": "ワークフローを端末内で実行しています…"})
	images, err := b.executeImageFlow(ctx, payload, e)
	if err != nil {
		e.send("error", map[string]any{"message": err.Error()})
		return
	}
	expected := map[string]bool{}
	for _, n := range body.Graph.Nodes {
		if n.Type == "core.output" {
			expected[n.ID] = true
		}
	}
	for _, image := range images {
		if !expected[image.Node] {
			e.send("error", map[string]any{"message": "出力ノードと画像が一致しません"})
			return
		}
		delete(expected, image.Node)
	}
	if len(expected) > 0 {
		e.send("error", map[string]any{"message": "一部の出力画像が返りませんでした"})
		return
	}
	e.send("done", map[string]any{"images": images, "graph": body.Graph, "seconds": time.Since(start).Seconds()})
}

// 既存の単発生成とノード生成で、完了判定・キャンセル・画像保存を共用する。
func (b *bridge) executeImageFlow(ctx context.Context, params map[string]any, e eventWriter) ([]renderedImage, error) {
	r, err := b.request(ctx, "POST", b.nightmare, "/run", params, true)
	if err != nil {
		return nil, errors.New("画像エンジンに接続できません")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("画像エンジン HTTP %d。モデル・実行中の処理を確認してください", r.StatusCode)
	}
	complete := false
	defer func() {
		if !complete {
			c, stop := context.WithTimeout(context.Background(), 3*time.Second)
			defer stop()
			res, _ := b.request(c, "POST", b.nightmare, "/cancel", map[string]any{}, true)
			if res != nil {
				res.Body.Close()
			}
		}
	}()
	var done struct {
		Error   string   `json:"error"`
		Failed  []string `json:"failed"`
		Results []struct {
			Node string `json:"node"`
			URL  string `json:"url"`
		} `json:"results"`
	}
	scan := bufio.NewScanner(r.Body)
	scan.Buffer(make([]byte, 4096), 2<<20)
	event := ""
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		switch event {
		case "node", "done_node":
			var n struct {
				Node    string `json:"node"`
				Outcome string `json:"outcome"`
			}
			if json.Unmarshal(raw, &n) == nil {
				e.send("node", map[string]any{"node": n.Node, "outcome": n.Outcome})
			}
		case "progress":
			var p struct {
				Step  int `json:"step"`
				Total int `json:"total"`
			}
			if json.Unmarshal(raw, &p) == nil {
				e.send("status", map[string]any{"message": fmt.Sprintf("画像を生成しています… %d / %d", p.Step, p.Total)})
			}
		case "error":
			return nil, errors.New("ワークフローの実行に失敗しました。Nightmareのログを確認してください")
		case "done":
			if json.Unmarshal(raw, &done) != nil {
				return nil, errors.New("画像の結果を読み取れません")
			}
			complete = true
			if done.Error != "" || len(done.Failed) > 0 || len(done.Results) < 1 || len(done.Results) > 4 {
				return nil, errors.New("すべての出力が完了しませんでした")
			}
		}
	}
	if err = scan.Err(); err != nil {
		return nil, errors.New("画像エンジンとの接続が切れました")
	}
	if !complete {
		return nil, errors.New("生成の途中で接続が切れました")
	}
	for _, result := range done.Results {
		if !resultPath.MatchString(result.URL) {
			return nil, errors.New("画像の取得先を確認できません")
		}
	}
	images := []renderedImage{}
	for _, result := range done.Results {
		image, err := b.storeResultImage(ctx, result.URL)
		if err != nil {
			return nil, err
		}
		images = append(images, renderedImage{result.Node, image})
	}
	return images, nil
}
func (b *bridge) storeResultImage(ctx context.Context, resultURL string) (string, error) {
	image, err := b.request(ctx, "GET", b.nightmare, resultURL, nil, true)
	if err != nil {
		return "", errors.New("生成画像を取得できません")
	}
	defer image.Body.Close()
	if image.StatusCode != 200 {
		return "", errors.New("生成画像を取得できません")
	}
	data, err := io.ReadAll(io.LimitReader(image.Body, (32<<20)+1))
	if err != nil || len(data) > 32<<20 || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "", errors.New("画像形式を確認できません")
	}
	var id [12]byte
	if _, err = rand.Read(id[:]); err != nil {
		return "", err
	}
	name := hex.EncodeToString(id[:]) + ".png"
	if err = os.WriteFile(filepath.Join(b.data, "images", name), data, 0600); err != nil {
		return "", errors.New("画像の保存に失敗しました")
	}
	return "/images/" + name, nil
}
