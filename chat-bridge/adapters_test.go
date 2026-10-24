package main

import (
	"strings"
	"testing"
)

func TestLocalAdapterBoundaries(t *testing.T) {
	const a = "Wada Arco[style]-Illus.safetensors"
	for name, spec := range map[string]string{"one": a + "@0.7", "multiple": a + "@2,aimoge.safetensors@0"} {
		t.Run(name, func(t *testing.T) {
			if err := validateLoras(spec, swapModel); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, spec := range []string{a + "@NaN", a + "@-1", a + "@2.1", a + "@1," + a + "@1", "../" + a + "@1", "boldline.safetensors@1", "qwen_image_vae.safetensors@1", a, "missing.safetensors@1", strings.Repeat(a+"@1,", 5)} {
		if validateLoras(spec, swapModel) == nil {
			t.Errorf("accepted invalid spec %q", spec)
		}
	}
	if validateLoras(a+"@1", models[0].ID) == nil {
		t.Fatal("plain model accepted LoRA")
	}
}

func TestSwapGraphKeepsSettingsAndTypes(t *testing.T) {
	g := workflowFixture()
	g.Nodes[1].Params["model"] = swapModel
	g.Nodes[1].Params["loras"] = "Wada Arco[style]-Illus.safetensors@0.7"
	g.Nodes[1].Params["scheduler"] = "euler_a"
	g.Nodes[1].Params["cfg"] = "5"
	installed := append(verifiedFixtures(), installedModel{ID: swapModel, Family: "SDXL_SWAP", Supported: true})
	if err := validateWorkflow(g, installed); err != nil {
		t.Fatal(err)
	}
	native := nativeWorkflow(g)
	if native.Nodes[1].Type != "sdxlswap.sample" || native.Nodes[4].Type != "sdxl.sample" || g.Nodes[1].Type != "sdxl.sample" {
		t.Fatal("wrong native type or mutated saved graph")
	}
	if native.Nodes[1].Params["loras"] != g.Nodes[1].Params["loras"] || native.Nodes[4].Inputs["image"] != "draw" {
		t.Fatal("lost settings/connection")
	}
	g.Nodes[0].Params["prompt"] = "fox <lora:missing:1>"
	if validateWorkflow(g, installed) == nil {
		t.Fatal("inline unrecorded LoRA accepted")
	}
}
