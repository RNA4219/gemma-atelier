package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
)

type inferenceConfig struct {
	Profile  string
	Context  int
	Thinking bool
}

func inferenceFromEnv() (inferenceConfig, error) {
	c := inferenceConfig{Profile: os.Getenv("ATELIER_LLM_PROFILE")}
	if c.Profile == "" {
		c.Profile = "gemma"
	}
	if c.Profile != "gemma" && c.Profile != "bolt" {
		return c, errors.New("ATELIER_LLM_PROFILE must be gemma or bolt")
	}
	c.Context = 2048
	if c.Profile == "bolt" {
		c.Context = 8192
	}
	if raw := os.Getenv("ATELIER_LLM_CONTEXT"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 2048 || n > 32768 {
			return c, errors.New("ATELIER_LLM_CONTEXT must be 2048-32768")
		}
		c.Context = n
	}
	switch os.Getenv("ATELIER_LLM_THINKING") {
	case "", "off":
	case "on":
		c.Thinking = true
	default:
		return c, errors.New("ATELIER_LLM_THINKING must be on or off")
	}
	return c, nil
}

func (b *bridge) chatPayload(msgs []message, thinking *bool) map[string]any {
	body := payload(msgs)
	on := b.inference.Thinking
	if thinking != nil {
		on = *thinking
	}
	kwargs := map[string]bool{"enable_thinking": on}
	if b.inference.Profile == "bolt" {
		kwargs["conversational_mode"] = true
	}
	body["chat_template_kwargs"] = kwargs
	if on {
		body["max_tokens"] = 1024
	}
	return body
}

func (b *bridge) promptBudget(thinking *bool) int {
	contextSize := b.inference.Context
	if contextSize == 0 {
		contextSize = 2048
	}
	maxTokens := b.chatPayload(nil, thinking)["max_tokens"].(int)
	return contextSize - maxTokens - 84
}

func (b *bridge) afterImage(ctx context.Context, msgs []message, callID, prompt, image string, thinking *bool) (string, error) {
	if callID == "" {
		return "", errors.New("画像ツールの呼び出しIDがありません")
	}
	args, _ := json.Marshal(map[string]string{"prompt": prompt})
	result, _ := json.Marshal(map[string]string{"status": "success", "image": image, "prompt": prompt})
	msgs = append(msgs,
		message{Role: "assistant", ToolCalls: []map[string]any{{"id": callID, "type": "function", "function": map[string]string{"name": "generate_image", "arguments": string(args)}}}},
		message{Role: "tool", ToolCallID: callID, Content: string(result)})
	msgs, _, err := b.fitThinking(ctx, msgs, thinking)
	if err != nil {
		return "", err
	}
	body := b.chatPayload(msgs, thinking)
	body["tool_choice"] = "none"
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := b.llmJSON(ctx, "/v1/chat/completions", body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) != 1 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", errors.New("画像生成後の会話応答が空です")
	}
	return response.Choices[0].Message.Content, nil
}
