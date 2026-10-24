package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type message struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []map[string]any `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}
type clientMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Prompt  string `json:"prompt,omitempty"`
	Image   string `json:"image,omitempty"`
}

func conversation(m model, input []clientMessage) []message {
	msgs := []message{{Role: "system", Content: systemPrompt(m)}}
	for i, item := range input {
		if item.Role == "assistant" && item.Prompt != "" && regexp.MustCompile(`^/images/[a-f0-9]{24}\.png$`).MatchString(item.Image) {
			id := fmt.Sprintf("image_%d", i)
			args, _ := json.Marshal(map[string]string{"prompt": item.Prompt})
			msgs = append(msgs, message{Role: "assistant", ToolCalls: []map[string]any{{"id": id, "type": "function", "function": map[string]string{"name": "generate_image", "arguments": string(args)}}}}, message{Role: "tool", ToolCallID: id, Content: "Image generation succeeded. The image is displayed in the chat."})
		}
		msgs = append(msgs, message{Role: item.Role, Content: item.Content})
	}
	return msgs
}

type model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Style string `json:"-"`
}

var models = []model{
	{"CyberRealistic-v10-DMD2-SDXL-8Gen3", "写真 · CyberRealistic", "Use concise natural English describing a photograph, unless the user requests another style."},
	{"Illustrious-v16-DMD2-SDXL-8Gen3", "イラスト · Illustrious", "Use concise comma-separated English illustration tags describing subject, composition, colors, lighting and the requested art style."},
}
var tools = []any{map[string]any{"type": "function", "function": map[string]any{
	"name": "generate_image", "description": "Create one new image on this phone from an English visual prompt. Also use for requests to redraw or change the previous image; reconstruct a complete prompt from context.",
	"parameters": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "description": "Complete English visual prompt, at most 55 words. Preserve the user's subject and requested changes."}}, "required": []string{"prompt"}},
}}}

type bridge struct {
	llm, nightmare, token, data string
	inference                   inferenceConfig
	client                      *http.Client
	busy                        sync.Mutex
}
type eventWriter struct {
	enc   *json.Encoder
	flush http.Flusher
}

func (e eventWriter) send(kind string, fields map[string]any) {
	fields["type"] = kind
	_ = e.enc.Encode(fields)
	if e.flush != nil {
		e.flush.Flush()
	}
}

func (b *bridge) request(ctx context.Context, method, base, path string, data any, auth bool) (*http.Response, error) {
	var body io.Reader
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
	return b.client.Do(req)
}
func (b *bridge) llmJSON(ctx context.Context, path string, body, out any) error {
	r, err := b.request(ctx, "POST", b.llm, path, body, false)
	if err != nil {
		return fmt.Errorf("会話モデルに接続できません: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 2000))
		return fmt.Errorf("会話モデル HTTP %d: %s", r.StatusCode, raw)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(out)
}
func systemPrompt(m model) string {
	return "You are Gemma, a friendly conversational assistant. Reply naturally in Japanese. For requests to draw, create or change an image, ALWAYS call generate_image. For other conversation, answer without calling tools. Do not claim an image was created before the tool succeeds. Use recent context to resolve references like 'that' and reconstruct the entire scene for revisions. The tool creates a new text-to-image picture, not a pixel edit. Preserve the user's specifics. Do not add people or styles the user did not ask for. Keep the English prompt under 55 words. " + m.Style
}
func payload(msgs []message) map[string]any {
	return map[string]any{"messages": msgs, "tools": tools, "tool_choice": "auto", "max_tokens": 384, "temperature": 0.5, "stream": false, "chat_template_kwargs": map[string]bool{"enable_thinking": false}}
}

// Keep the actual chat template and tool definitions inside the 2048-token context.
func (b *bridge) fit(ctx context.Context, msgs []message) ([]message, bool, error) {
	return b.fitThinking(ctx, msgs, nil)
}

func (b *bridge) fitThinking(ctx context.Context, msgs []message, thinking *bool) ([]message, bool, error) {
	trimmed := false
	for {
		var template struct {
			Prompt string `json:"prompt"`
		}
		if err := b.llmJSON(ctx, "/apply-template", b.chatPayload(msgs, thinking), &template); err != nil {
			return nil, trimmed, err
		}
		var tokens struct {
			Tokens []json.RawMessage `json:"tokens"`
		}
		if err := b.llmJSON(ctx, "/tokenize", map[string]any{"content": template.Prompt, "add_special": false, "parse_special": true}, &tokens); err != nil {
			return nil, trimmed, err
		}
		if len(tokens.Tokens) <= b.promptBudget(thinking) {
			return msgs, trimmed, nil
		}
		if len(msgs) <= 2 {
			return nil, trimmed, errors.New("メッセージが長いため、短く分けて送ってください。")
		}
		cut := 2
		for cut < len(msgs) && msgs[cut].Role != "user" {
			cut++
		}
		if cut == len(msgs) {
			return nil, trimmed, errors.New("現在の会話とツール結果がコンテキスト上限を超えました。")
		}
		msgs = append([]message{msgs[0]}, msgs[cut:]...)
		trimmed = true
	}
}
func (b *bridge) chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	if r.Header.Get("X-Chat-Client") != "1" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "invalid request", 403)
		return
	}
	var input struct {
		imageSettingsInput
		Messages []clientMessage `json:"messages"`
		Model    string          `json:"model"`
		Thinking *bool           `json:"thinking,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 96<<10)).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	settings, err := input.imageSettingsInput.resolve()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var chosen *model
	for i := range models {
		if models[i].ID == input.Model {
			chosen = &models[i]
		}
	}
	if chosen == nil {
		http.Error(w, "画像モデルを選択してください。", 400)
		return
	}
	if len(input.Messages) == 0 || len(input.Messages) > 40 || input.Messages[len(input.Messages)-1].Role != "user" {
		http.Error(w, "invalid messages", 400)
		return
	}
	for _, m := range input.Messages {
		if (m.Role != "user" && m.Role != "assistant") || len(m.Content) > 16000 || len(m.Prompt) > 3000 || len(m.Image) > 100 || strings.TrimSpace(m.Content) == "" {
			http.Error(w, "invalid message", 400)
			return
		}
	}
	if !b.busy.TryLock() {
		http.Error(w, "生成中です。完了してから送信してください。", 409)
		return
	}
	defer b.busy.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fl, _ := w.(http.Flusher)
	e := eventWriter{json.NewEncoder(w), fl}
	fail := func(err error) { e.send("error", map[string]any{"message": err.Error()}) }
	start := time.Now()
	e.send("status", map[string]any{"message": "返答を準備しています…"})
	msgs := conversation(*chosen, input.Messages)
	msgs, trimmed, err := b.fitThinking(ctx, msgs, input.Thinking)
	if err != nil {
		fail(err)
		return
	}
	var answer struct {
		Choices []struct {
			Finish  string `json:"finish_reason"`
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = b.llmJSON(ctx, "/v1/chat/completions", b.chatPayload(msgs, input.Thinking), &answer); err != nil {
		fail(err)
		return
	}
	llmSeconds := time.Since(start).Seconds()
	if len(answer.Choices) == 0 {
		fail(errors.New("Gemmaの回答が空でした。"))
		return
	}
	choice := answer.Choices[0]
	a := choice.Message
	if len(a.ToolCalls) == 0 {
		if strings.TrimSpace(a.Content) == "" {
			fail(errors.New("Gemmaの回答が空でした。"))
			return
		}
		e.send("done", map[string]any{"text": a.Content, "seconds": llmSeconds, "trimmed": trimmed, "truncated": choice.Finish == "length"})
		return
	}
	if len(a.ToolCalls) != 1 || a.ToolCalls[0].Function.Name != "generate_image" || choice.Finish == "length" {
		fail(errors.New("画像の指示を整理できませんでした。描きたい内容をもう一度送ってください。"))
		return
	}
	var args struct {
		Prompt string `json:"prompt"`
	}
	if err = json.Unmarshal([]byte(a.ToolCalls[0].Function.Arguments), &args); err != nil || len(strings.TrimSpace(args.Prompt)) == 0 || len(args.Prompt) > 3000 {
		fail(errors.New("画像プロンプトを読み取れませんでした。"))
		return
	}
	e.send("prompt", map[string]any{"prompt": args.Prompt, "model": chosen.Label})
	imgStart := time.Now()
	imageURL, err := b.generate(ctx, *chosen, args.Prompt, settings, e)
	if err != nil {
		fail(err)
		return
	}
	imageSeconds := time.Since(imgStart).Seconds()
	reply := "画像を生成しました。"
	warning := ""
	if b.inference.Profile == "bolt" {
		e.send("status", map[string]any{"message": "画像ができました。返答を準備しています…"})
		continuationStart := time.Now()
		text, continuationErr := b.afterImage(ctx, msgs, a.ToolCalls[0].ID, args.Prompt, imageURL, input.Thinking)
		llmSeconds += time.Since(continuationStart).Seconds()
		if continuationErr != nil {
			warning = continuationErr.Error()
			reply = "画像を生成しました。画像は保存済みですが、その後の会話応答を取得できませんでした。"
		} else {
			reply = text
		}
	}
	e.send("done", map[string]any{"text": reply, "warning": warning, "image": imageURL, "prompt": args.Prompt, "negative": settings.Negative, "cfg": settings.CFG, "model": chosen.Label, "seconds": time.Since(start).Seconds(), "image_seconds": imageSeconds, "llm_seconds": llmSeconds, "trimmed": trimmed})
}

func (b *bridge) generate(ctx context.Context, m model, prompt string, settings imageSettings, e eventWriter) (string, error) {
	var seedBytes [4]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return "", err
	}
	seed := int64(seedBytes[0])<<24 | int64(seedBytes[1])<<16 | int64(seedBytes[2])<<8 | int64(seedBytes[3])
	params := map[string]any{"flow": "txt2img", "model": m.ID, "prompt": prompt, "negative": settings.Negative, "steps": 8, "cfg": settings.CFG, "scheduler": "lcm", "width": 1024, "height": 1024, "seed": seed, "stream": true}
	e.send("status", map[string]any{"message": "画像モデルを読み込んでいます…"})
	images, err := b.executeImageFlow(ctx, params, e)
	if err != nil {
		return "", err
	}
	return images[0].Image, nil
}
func (b *bridge) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", b.chat)
	mux.HandleFunc("/api/tags", b.tags)
	mux.HandleFunc("/api/generate", b.generateTags)
	mux.HandleFunc("/api/workflows/catalog", b.workflowCatalog)
	mux.HandleFunc("/api/workflows/run", b.runWorkflow)
	mux.HandleFunc("/api/workflows/image", b.uploadWorkflowImage)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		healthy := func(base, path string) bool {
			res, err := b.request(ctx, "GET", base, path, nil, false)
			if err != nil {
				return false
			}
			defer res.Body.Close()
			return res.StatusCode == 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"gemma": healthy(b.llm, "/health"), "images": b.token != "" && healthy(b.nightmare, "/info"), "models": models})
	})
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/images/")
		if !regexp.MustCompile(`^[a-f0-9]{24}\.png$`).MatchString(name) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(b.data, "images", name))
	})
	web, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "invalid host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "http" || u.Host != r.Host {
				http.Error(w, "invalid origin", 403)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
func main() {
	addr := flag.String("listen", "127.0.0.1:8085", "loopback address")
	data := flag.String("data", "chat-data", "private storage")
	tokenFile := flag.String("token-file", "nightmare.token", "private token file")
	llm := flag.String("llm", "http://127.0.0.1:8084", "Gemma local API")
	nightmare := flag.String("nightmare", "http://127.0.0.1:8820", "Nightmare local API")
	allowMissingImages := flag.Bool("allow-missing-images", false, "allow chat before the image engine is configured")
	flag.Parse()
	inference, err := inferenceFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("listen address must be loopback")
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil && !(*allowMissingImages && os.IsNotExist(err)) {
		log.Fatal("private Nightmare token file unavailable")
	}
	if strings.TrimSpace(string(token)) == "" && !*allowMissingImages {
		log.Fatal("empty Nightmare token")
	}
	if err = os.MkdirAll(filepath.Join(*data, "images"), 0700); err != nil {
		log.Fatal(err)
	}
	b := &bridge{llm: *llm, nightmare: *nightmare, token: strings.TrimSpace(string(token)), data: *data, inference: inference, client: &http.Client{Timeout: 8 * time.Minute, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
	server := &http.Server{Addr: *addr, Handler: b.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 9 * time.Minute, IdleTimeout: 60 * time.Second}
	log.Printf("Gemma + image chat ready at http://%s", *addr)
	log.Fatal(server.ListenAndServe())
}
