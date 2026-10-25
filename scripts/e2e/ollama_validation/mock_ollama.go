package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ocrModel         = "ocr-sandbox:latest"
	translationModel = "translate-sandbox:latest"
	maxBodyBytes     = 2 << 20
)

type mockState struct {
	mu             sync.Mutex
	resident       bool
	mode           string
	failAt         int
	chatRequests   int
	warmupRequests int
	psRequests     int
	tagsRequests   int
	showRequests   int
	schemaRequests int
	activeChats    int
	maxActiveChats int
	chatModels     []string
}

type chatInput struct {
	Model    string             `json:"model"`
	Messages []chatInputMessage `json:"messages"`
	Stream   bool               `json:"stream"`
	Format   json.RawMessage    `json:"format"`
}

type chatInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type proofreadInput struct {
	ContractVersion string `json:"contractVersion"`
	Cues            []struct {
		CueID      string `json:"cueId"`
		SourceHash string `json:"sourceTextHash"`
		Text       string `json:"text"`
	} `json:"cues"`
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get("http://127.0.0.1:11434/health")
		if err != nil {
			os.Exit(1)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--init-data" {
		if err := initializeDataVolume(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox volume initialization failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	state := &mockState{mode: "success"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/version", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"version": "mvforge-synthetic-ollama-mock-v1"})
	})
	mux.HandleFunc("GET /api/ps", state.processModels)
	mux.HandleFunc("GET /api/tags", state.installedModels)
	mux.HandleFunc("POST /api/show", state.showModel)
	mux.HandleFunc("POST /api/generate", state.warmup)
	mux.HandleFunc("POST /api/chat", state.chat)
	mux.HandleFunc("POST /__admin/reset", state.reset)
	mux.HandleFunc("POST /__admin/mode", state.setMode)
	mux.HandleFunc("GET /__admin/stats", state.stats)
	server := &http.Server{Addr: "0.0.0.0:11434", Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "mock provider stopped")
		os.Exit(1)
	}
}

func initializeDataVolume(root string) error {
	if root != "/media" {
		return errors.New("unexpected sandbox data root")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	paths := []string{
		"config", "config/xdg", "config/data", "config/state", "media", "raw", "staging",
		"ocr", "ocr/work", "ocr/state", "reports", "cache", "cache/xdg", "cache/go",
		"tmp", "sidecars", "work",
	}
	for _, relative := range paths {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func (state *mockState) processModels(writer http.ResponseWriter, _ *http.Request) {
	state.mu.Lock()
	state.psRequests++
	resident := state.resident
	state.mu.Unlock()
	models := []map[string]string{}
	if resident {
		models = append(models,
			map[string]string{"name": ocrModel, "model": ocrModel},
			map[string]string{"name": translationModel, "model": translationModel},
		)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"models": models})
}

func (state *mockState) installedModels(writer http.ResponseWriter, _ *http.Request) {
	state.mu.Lock()
	state.tagsRequests++
	state.mu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{"models": []map[string]any{
		{"name": ocrModel, "model": ocrModel, "size": 1024, "digest": "synthetic-ocr"},
		{"name": translationModel, "model": translationModel, "size": 1024, "digest": "synthetic-translation"},
	}})
}

func (state *mockState) showModel(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeBoundedJSON(request, &body); err != nil || !allowedModel(body.Name) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "unknown synthetic model"})
		return
	}
	state.mu.Lock()
	state.showRequests++
	state.mu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{"modelfile": "synthetic", "details": map[string]string{"family": "synthetic"}})
}

func (state *mockState) warmup(writer http.ResponseWriter, request *http.Request) {
	var body map[string]json.RawMessage
	if err := decodeBoundedJSON(request, &body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid warmup request"})
		return
	}
	var model string
	if err := json.Unmarshal(body["model"], &model); err != nil || !allowedModel(model) || len(body["prompt"]) != 0 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "warmup must name a synthetic model and omit prompt"})
		return
	}
	state.mu.Lock()
	state.warmupRequests++
	state.resident = true
	mode := state.mode
	state.mu.Unlock()
	if mode == "warmup-failed" {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "synthetic warmup failure"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"done": true})
}

func (state *mockState) chat(writer http.ResponseWriter, request *http.Request) {
	var body chatInput
	if err := decodeBoundedJSON(request, &body); err != nil || !allowedModel(body.Model) || body.Stream || len(body.Messages) != 1 || body.Messages[0].Role != "user" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid bounded synthetic chat request"})
		return
	}
	var format any
	if len(body.Format) == 0 || json.Unmarshal(body.Format, &format) != nil || format == nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "structured response format is required"})
		return
	}
	state.mu.Lock()
	state.chatRequests++
	requestNumber := state.chatRequests
	state.chatModels = append(state.chatModels, body.Model)
	if _, schema := format.(map[string]any); schema {
		state.schemaRequests++
	}
	state.activeChats++
	if state.activeChats > state.maxActiveChats {
		state.maxActiveChats = state.activeChats
	}
	mode, failAt := state.mode, state.failAt
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		state.activeChats--
		state.mu.Unlock()
	}()
	if body.Model == translationModel && !validTranslationSchema(body.Format, body.Messages[0].Content) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "translation JSON schema does not match the requested cue batch"})
		return
	}
	if mode == "delay" {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-request.Context().Done():
			return
		}
	}
	if mode == "http-error" || (failAt > 0 && requestNumber == failAt && mode == "fail-at") {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "synthetic provider unavailable"})
		return
	}
	if mode == "malformed" || (failAt > 0 && requestNumber == failAt && mode == "malformed-at") {
		writeJSON(writer, http.StatusOK, map[string]any{"message": map[string]string{"content": "not-json"}, "done": true})
		return
	}

	inner, err := synthesizeChatResponse(body.Messages[0].Content)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "unrecognized synthetic contract"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"message": map[string]string{"content": string(inner)},
		"done":    true, "done_reason": "stop", "prompt_eval_count": 17, "eval_count": 11,
		"total_duration": 12000000,
	})
}

func validTranslationSchema(encoded json.RawMessage, prompt string) bool {
	start := strings.LastIndex(prompt, `{"ContractVersion"`)
	if start < 0 {
		return false
	}
	var batch struct {
		Cues []struct {
			ID string `json:"ID"`
		} `json:"Cues"`
	}
	if json.Unmarshal([]byte(prompt[start:]), &batch) != nil || len(batch.Cues) == 0 {
		return false
	}
	var schema struct {
		Type     string                     `json:"type"`
		Required []string                   `json:"required"`
		Props    map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(encoded, &schema) != nil || schema.Type != "object" || !includes(schema.Required, "contractVersion") || !includes(schema.Required, "cues") {
		return false
	}
	var cues struct {
		Type     string          `json:"type"`
		MinItems int             `json:"minItems"`
		MaxItems int             `json:"maxItems"`
		Items    json.RawMessage `json:"items"`
	}
	if json.Unmarshal(schema.Props["cues"], &cues) != nil || cues.Type != "array" || cues.MinItems != len(batch.Cues) || cues.MaxItems != len(batch.Cues) {
		return false
	}
	var item struct {
		Required []string `json:"required"`
		Props    struct {
			ID struct {
				Enum []string `json:"enum"`
			} `json:"id"`
		} `json:"properties"`
		AdditionalProperties *bool `json:"additionalProperties"`
	}
	if json.Unmarshal(cues.Items, &item) != nil || !includes(item.Required, "id") || !includes(item.Required, "text") || item.AdditionalProperties == nil || *item.AdditionalProperties {
		return false
	}
	for _, cue := range batch.Cues {
		if !includes(item.Props.ID.Enum, cue.ID) {
			return false
		}
	}
	return true
}

func includes(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func synthesizeChatResponse(content string) ([]byte, error) {
	start := strings.LastIndex(content, `{"ContractVersion"`)
	if lower := strings.LastIndex(content, `{"contractVersion"`); lower > start {
		start = lower
	}
	if start < 0 {
		return nil, errors.New("contract payload missing")
	}
	payload := []byte(content[start:])
	var translation struct {
		ContractVersion string `json:"ContractVersion"`
		SourceLanguage  string `json:"SourceLanguage"`
		TargetLocale    string `json:"TargetLocale"`
		Cues            []struct {
			ID   string `json:"ID"`
			Text string `json:"Text"`
		} `json:"Cues"`
	}
	if json.Unmarshal(payload, &translation) == nil && translation.ContractVersion != "" &&
		translation.SourceLanguage != "" && translation.TargetLocale != "" && len(translation.Cues) > 0 && translation.Cues[0].ID != "" {
		cues := make([]map[string]string, 0, len(translation.Cues))
		for _, cue := range translation.Cues {
			translated := strings.NewReplacer("Good morning", "Buenos días", "Hello", "Hola", "world", "mundo").Replace(cue.Text)
			cues = append(cues, map[string]string{"id": cue.ID, "text": translated})
		}
		return json.Marshal(map[string]any{"contractVersion": translation.ContractVersion, "cues": cues})
	}
	var proofreading proofreadInput
	if err := json.Unmarshal(payload, &proofreading); err != nil || proofreading.ContractVersion == "" || len(proofreading.Cues) == 0 {
		return nil, errors.New("proofreading contract missing")
	}
	findings := make([]map[string]string, 0, len(proofreading.Cues))
	for _, cue := range proofreading.Cues {
		findings = append(findings, map[string]string{
			"cueId": cue.CueID, "sourceTextHash": cue.SourceHash, "status": "suggested",
			"suggestedText": strings.ReplaceAll(cue.Text, "|", " "), "category": "spelling",
			"reasonCode": "synthetic-ocr-confusion", "reason": "Synthetic bounded OCR review suggestion.",
		})
	}
	return json.Marshal(map[string]any{"contractVersion": proofreading.ContractVersion, "findings": findings})
}

func (state *mockState) reset(writer http.ResponseWriter, request *http.Request) {
	if _, err := io.Copy(io.Discard, http.MaxBytesReader(writer, request.Body, 1024)); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid reset request"})
		return
	}
	state.mu.Lock()
	state.resident = false
	state.mode = "success"
	state.failAt = 0
	state.chatRequests = 0
	state.warmupRequests = 0
	state.psRequests = 0
	state.tagsRequests = 0
	state.showRequests = 0
	state.schemaRequests = 0
	state.activeChats = 0
	state.maxActiveChats = 0
	state.chatModels = nil
	state.mu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]string{"state": "reset"})
}

func (state *mockState) setMode(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Mode   string `json:"mode"`
		FailAt int    `json:"failAt"`
	}
	if err := decodeBoundedJSON(request, &body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid mode request"})
		return
	}
	switch body.Mode {
	case "success", "malformed", "http-error", "delay", "warmup-failed", "malformed-at", "fail-at":
	default:
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "unsupported synthetic mode"})
		return
	}
	state.mu.Lock()
	state.mode, state.failAt = body.Mode, body.FailAt
	state.mu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{"mode": body.Mode, "failAt": body.FailAt})
}

func (state *mockState) stats(writer http.ResponseWriter, _ *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{
		"resident": state.resident, "chatRequests": state.chatRequests, "warmupRequests": state.warmupRequests,
		"psRequests": state.psRequests, "tagsRequests": state.tagsRequests, "showRequests": state.showRequests,
		"schemaRequests": state.schemaRequests, "activeChats": state.activeChats,
		"maxActiveChats": state.maxActiveChats, "chatModels": append([]string(nil), state.chatModels...),
	})
}

func decodeBoundedJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, maxBodyBytes))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request contains trailing content")
	}
	return nil
}

func allowedModel(model string) bool {
	return model == ocrModel || model == translationModel
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
