package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
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

var storedImagePath = regexp.MustCompile(`^/images/[a-f0-9]{24}\.png$`)

const sequentialEngineVersion = "1.6.211-atelier-cnseq1"
const controlNetUnavailable = "ControlNetは準備中です。UNetとControlNetを交互に読み込むAtelier NPUに接続できません。"

// 配布版Nightmareに接続し直した場合は、同時読み込みへ戻さず実行を止める。
func (b *bridge) sequentialControlReady(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r, err := b.request(ctx, "GET", b.nightmare, "/info", nil, false)
	if err != nil {
		return false
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return false
	}
	var info struct {
		Version string `json:"version"`
	}
	return json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&info) == nil && info.Version == sequentialEngineVersion
}

func usesControlNet(g workflowGraph) bool {
	for _, n := range g.Nodes {
		if n.Type == "sdxl.sample" && n.Params["controlnet"] != "" && n.Params["controlnet"] != "none" {
			return true
		}
	}
	return false
}

func validateControl(n workflowNode) error {
	kind := n.Params["controlnet"]
	if kind == "" || kind == "none" {
		if n.Inputs["control"] != "" || n.Params["control_strength"] != "" {
			return errors.New("ControlNetの種類を選ぶか、参照接続と強度を解除してください")
		}
		return nil
	}
	if n.Params["model"] != swapModel || (kind != "canny" && kind != "depth" && kind != "openpose") {
		return errors.New("ControlNetはrinFlanimeのSwapモデルでCanny・Depth・OpenPoseを選んでください")
	}
	v, err := strconv.ParseFloat(n.Params["control_strength"], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 2 {
		return errors.New("ControlNetの強度は0より大きく2以下にしてください。解除は「なし」を選びます")
	}
	if n.Inputs["control"] == "" {
		return errors.New("ControlNetの参照画像を接続してください")
	}
	return nil
}

// PNGへ正規化した入力だけを受け取り、EXIFや任意のファイルパスを渡さない。
func (b *bridge) uploadWorkflowImage(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var body struct {
		PNG string `json:"png"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "PNG画像を送信してください", 400)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(body.PNG)
	if err != nil || len(raw) > 4<<20 {
		http.Error(w, "画像は4MB以内にしてください", 400)
		return
	}
	config, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 1024 || config.Height > 1024 {
		http.Error(w, "画像は各辺1024px以内のPNGにしてください", 400)
		return
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		http.Error(w, "画像が壊れています", 400)
		return
	}
	var normalized bytes.Buffer
	if png.Encode(&normalized, img) != nil {
		http.Error(w, "画像を変換できません", 500)
		return
	}
	sum := sha256.Sum256(normalized.Bytes())
	name := hex.EncodeToString(sum[:12]) + ".png"
	if os.WriteFile(filepath.Join(b.data, "images", name), normalized.Bytes(), 0600) != nil {
		http.Error(w, "画像を保存できません", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"image": "/images/" + name, "width": config.Width, "height": config.Height})
}

func (b *bridge) workflowPayload(g workflowGraph) (map[string]any, error) {
	native := nativeWorkflow(g)
	images := map[string]string{}
	for i, n := range native.Nodes {
		if n.Type != "core.image" {
			continue
		}
		ref := n.Params["image"]
		if !storedImagePath.MatchString(ref) {
			return nil, errors.New("参照画像を選び直してください")
		}
		f, err := os.Open(filepath.Join(b.data, "images", strings.TrimPrefix(ref, "/images/")))
		if err != nil {
			return nil, errors.New("保存された参照画像がありません。この端末で画像を選び直してください")
		}
		raw, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
		f.Close()
		if err != nil || len(raw) > 4<<20 {
			return nil, errors.New("参照画像を読み込めません")
		}
		if cfg, err := png.DecodeConfig(bytes.NewReader(raw)); err != nil || cfg.Width > 1024 || cfg.Height > 1024 {
			return nil, errors.New("参照画像の形式を確認できません")
		}
		images[n.ID] = base64.StdEncoding.EncodeToString(raw)
		native.Nodes[i].Params = map[string]string{}
	}
	return map[string]any{"graph": native, "images": images, "stream": true}, nil
}
