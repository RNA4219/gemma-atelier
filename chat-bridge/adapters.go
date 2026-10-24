package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

//go:embed data/adapters.json
var adapterJSON []byte

type localAdapter struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Family    string `json:"family"`
	Supported bool   `json:"supported"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Note      string `json:"note"`
}

func localAdapters() []localAdapter {
	var items []localAdapter
	if err := json.Unmarshal(adapterJSON, &items); err != nil {
		panic(err)
	}
	return items
}

const swapModel = "community_rinflanime_v50_swap"

func validateLoras(spec string, modelID string) error {
	if spec == "" {
		return nil
	}
	if modelID != swapModel {
		return fmt.Errorf("LoRAにはrinFlanimeのSwapモデルを選んでください")
	}
	available := map[string]bool{}
	for _, a := range localAdapters() {
		available[a.Name] = a.Kind == "lora" && a.Supported
	}
	entries := strings.Split(spec, ",")
	if len(entries) > 4 {
		return fmt.Errorf("LoRAは1段階につき4個までです")
	}
	seen := map[string]bool{}
	for _, item := range entries {
		name, number, ok := strings.Cut(strings.TrimSpace(item), "@")
		if !ok || !available[name] || seen[name] {
			return fmt.Errorf("導入済みの対応LoRAを重複なしで選んでください")
		}
		v, err := strconv.ParseFloat(number, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 2 {
			return fmt.Errorf("LoRA強度は0〜2で指定してください")
		}
		seen[name] = true
	}
	return nil
}

// PWAの共通生成ノードを、実行先のモデルに合うNightmareノードへ変換する。
// 記録するフローは共通形式を保持し、再読込できるようにする。
func nativeWorkflow(g workflowGraph) workflowGraph {
	out := workflowGraph{Format: g.Format, Nodes: append([]workflowNode(nil), g.Nodes...)}
	for i := range out.Nodes {
		if out.Nodes[i].Type == "sdxl.sample" && out.Nodes[i].Params["model"] == swapModel {
			out.Nodes[i].Type = "sdxlswap.sample"
		}
	}
	return out
}
