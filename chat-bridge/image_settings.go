package main

import (
	"errors"
	"math"
)

// 省略した旧クライアントは従来の設定を維持し、空文字の指定は尊重する。
type imageSettingsInput struct {
	Negative *string  `json:"negative"`
	CFG      *float64 `json:"cfg"`
}

type imageSettings struct {
	Negative string  `json:"negative"`
	CFG      float64 `json:"cfg"`
}

func (input imageSettingsInput) resolve() (imageSettings, error) {
	settings := imageSettings{Negative: "low quality, blurry, text, watermark, bad anatomy", CFG: 1}
	if input.Negative != nil {
		settings.Negative = *input.Negative
	}
	if input.CFG != nil {
		settings.CFG = *input.CFG
	}
	if len(settings.Negative) > 3000 {
		return settings, errors.New("ネガティブプロンプトは3000バイト以内で入力してください。")
	}
	if math.IsNaN(settings.CFG) || math.IsInf(settings.CFG, 0) || settings.CFG < 1 || settings.CFG > 3 {
		return settings, errors.New("CFGは1〜3の範囲で指定してください。")
	}
	return settings, nil
}
