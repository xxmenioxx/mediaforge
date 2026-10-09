package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ollamaVersionMaxResponseBody = 64 * 1024
	ollamaTagsMaxResponseBody    = 1024 * 1024
	ollamaPSMaxResponseBody      = 512 * 1024
	ollamaGenerateMaxBody        = 64 * 1024
)

type ollamaProviderAPIClient struct {
	client *http.Client
}

type ollamaConnectionTestResult struct {
	State         string `json:"state"`
	CheckedAt     string `json:"checkedAt"`
	ServerVersion string `json:"serverVersion,omitempty"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}

type ollamaInstalledModel struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Digest     string                  `json:"digest,omitempty"`
	SizeBytes  int64                   `json:"sizeBytes,omitempty"`
	ModifiedAt string                  `json:"modifiedAt,omitempty"`
	Details    *ollamaInstalledDetails `json:"details,omitempty"`
}

type ollamaInstalledDetails struct {
	ParentModel       string   `json:"parentModel,omitempty"`
	Format            string   `json:"format,omitempty"`
	Family            string   `json:"family,omitempty"`
	Families          []string `json:"families,omitempty"`
	ParameterSize     string   `json:"parameterSize,omitempty"`
	QuantizationLevel string   `json:"quantizationLevel,omitempty"`
}

type ollamaModelsResponse struct {
	State           string                 `json:"state"`
	DiscoveredAt    string                 `json:"discoveredAt,omitempty"`
	InstalledModels []ollamaInstalledModel `json:"installedModels"`
	Diagnostic      string                 `json:"diagnostic,omitempty"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name       string `json:"name"`
		Model      string `json:"model"`
		Digest     string `json:"digest"`
		Size       int64  `json:"size"`
		ModifiedAt string `json:"modified_at"`
		Details    *struct {
			ParentModel       string   `json:"parent_model"`
			Format            string   `json:"format"`
			Family            string   `json:"family"`
			Families          []string `json:"families"`
			ParameterSize     string   `json:"parameter_size"`
			QuantizationLevel string   `json:"quantization_level"`
		} `json:"details"`
	} `json:"models"`
}

type ollamaPSResponse struct {
	Models []ollamaResidentModel `json:"models"`
}

type ollamaResidentModel struct {
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	ExpiresAt time.Time `json:"expires_at"`
	SizeVRAM  int64     `json:"size_vram"`
}

type ollamaWarmupRequest struct {
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
	KeepAlive string `json:"keep_alive"`
}

type ollamaModelReadinessRequest struct {
	ModelID string `json:"modelId"`
}

type ollamaModelReadinessResponse struct {
	State      string `json:"state"`
	ModelID    string `json:"modelId"`
	Available  bool   `json:"available"`
	CheckedAt  string `json:"checkedAt"`
	Diagnostic string `json:"diagnostic,omitempty"`
	Scope      string `json:"scope"`
}

func (client ollamaProviderAPIClient) httpClient() *http.Client {
	if client.client != nil {
		return client.client
	}
	return &http.Client{Timeout: ollamaTranslationHTTPTimeout}
}

func (h SettingsHandler) ollamaProviderAPIClient() ollamaProviderAPIClient {
	return ollamaProviderAPIClient{client: h.ollamaHTTPClient}
}

// ollamaProviderConfigurationID is the provider-neutral identity used by the
// #265 admission estimates and residency observations. It is derived from the
// canonical shared endpoint configuration without persisting the endpoint.
func ollamaProviderConfigurationID(endpoint string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(endpoint)))
	return "ollama-" + hex.EncodeToString(digest[:12])
}

func (client ollamaProviderAPIClient) get(ctx context.Context, endpoint, apiPath string, maximum int) ([]byte, int, error) {
	apiURL, err := ollamaAPIURL(endpoint, apiPath)
	if err != nil {
		return nil, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create Ollama request: %w", err)
	}
	response, err := client.httpClient().Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := readBoundedOllamaBody(ctx, response.Body, maximum)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return body, response.StatusCode, nil
}

func (client ollamaProviderAPIClient) postJSON(ctx context.Context, endpoint, apiPath string, payload any, maximum int) ([]byte, int, error) {
	apiURL, err := ollamaAPIURL(endpoint, apiPath)
	if err != nil {
		return nil, 0, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("encode Ollama request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL.String(), strings.NewReader(string(encoded)))
	if err != nil {
		return nil, 0, fmt.Errorf("create Ollama request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient().Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := readBoundedOllamaBody(ctx, response.Body, maximum)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return body, response.StatusCode, nil
}

func (client ollamaProviderAPIClient) loadedModels(ctx context.Context, endpoint string) ([]ollamaResidentModel, error) {
	body, status, err := client.get(ctx, endpoint, "/api/ps", ollamaPSMaxResponseBody)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Ollama process endpoint returned HTTP status %d", status)
	}
	var response ollamaPSResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.New("Ollama process endpoint returned a malformed response")
	}
	if response.Models == nil {
		return nil, errors.New("Ollama process endpoint omitted its models list")
	}
	for _, model := range response.Models {
		if strings.TrimSpace(model.Name) == "" && strings.TrimSpace(model.Model) == "" {
			return nil, errors.New("Ollama process endpoint returned a model without an identity")
		}
	}
	return response.Models, nil
}

func (client ollamaProviderAPIClient) warmModel(ctx context.Context, endpoint, model string) error {
	body, status, err := client.postJSON(ctx, endpoint, "/api/generate", ollamaWarmupRequest{
		Model:     model,
		Stream:    false,
		KeepAlive: ollamaModelKeepAlive,
	}, ollamaGenerateMaxBody)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return fmt.Errorf("Ollama warmup endpoint returned HTTP status %d", status)
	}
	var response struct {
		Done  bool   `json:"done"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return errors.New("Ollama warmup endpoint returned a malformed response")
	}
	if strings.TrimSpace(response.Error) != "" {
		return errors.New("Ollama rejected model warmup")
	}
	if !response.Done {
		return errors.New("Ollama warmup did not complete")
	}
	return nil
}

func testOllamaConnection(ctx context.Context, endpoint string, client ollamaProviderAPIClient) ollamaConnectionTestResult {
	result := ollamaConnectionTestResult{State: "unavailable", CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if strings.TrimSpace(endpoint) == "" {
		result.State = ollamaProviderStateInvalid
		result.Diagnostic = "Ollama endpoint is not configured"
		return result
	}
	if err := validateOllamaEndpoint(endpoint); err != nil {
		result.State = ollamaProviderStateInvalid
		result.Diagnostic = err.Error()
		return result
	}
	body, status, err := client.get(ctx, endpoint, "/api/version", ollamaVersionMaxResponseBody)
	if err != nil {
		result.Diagnostic = "Ollama connection test could not complete"
		var limitErr *ollamaResponseBodyLimitError
		if errors.As(err, &limitErr) {
			result.State = "invalid_response"
			result.Diagnostic = "Ollama version response exceeded the configured limit"
		}
		return result
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		result.Diagnostic = fmt.Sprintf("Ollama version endpoint returned HTTP status %d", status)
		return result
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &version); err != nil || strings.TrimSpace(version.Version) == "" {
		result.State = "invalid_response"
		result.Diagnostic = "Ollama version endpoint returned a malformed response"
		return result
	}
	result.State = "reachable"
	result.ServerVersion = strings.TrimSpace(version.Version)
	result.Diagnostic = ""
	return result
}

func listOllamaInstalledModels(ctx context.Context, endpoint string, client ollamaProviderAPIClient) ollamaModelsResponse {
	result := ollamaModelsResponse{State: "unavailable", InstalledModels: []ollamaInstalledModel{}}
	body, status, err := client.get(ctx, endpoint, "/api/tags", ollamaTagsMaxResponseBody)
	if err != nil {
		result.Diagnostic = "Ollama model catalog could not be read"
		var limitErr *ollamaResponseBodyLimitError
		if errors.As(err, &limitErr) {
			result.State = "invalid_response"
			result.Diagnostic = "Ollama model catalog exceeded the configured limit"
		}
		return result
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		result.Diagnostic = fmt.Sprintf("Ollama model catalog returned HTTP status %d", status)
		return result
	}
	var tags ollamaTagsResponse
	if err := json.Unmarshal(body, &tags); err != nil {
		result.State = "invalid_response"
		result.Diagnostic = "Ollama model catalog returned a malformed response"
		return result
	}
	if tags.Models == nil {
		result.State = "invalid_response"
		result.Diagnostic = "Ollama model catalog returned a malformed response"
		return result
	}
	for _, model := range tags.Models {
		id := strings.TrimSpace(model.Name)
		if id == "" {
			continue
		}
		installed := ollamaInstalledModel{
			ID:         id,
			Name:       id,
			Digest:     strings.TrimSpace(model.Digest),
			SizeBytes:  model.Size,
			ModifiedAt: strings.TrimSpace(model.ModifiedAt),
		}
		if model.Details != nil {
			installed.Details = &ollamaInstalledDetails{
				ParentModel:       model.Details.ParentModel,
				Format:            model.Details.Format,
				Family:            model.Details.Family,
				Families:          append([]string(nil), model.Details.Families...),
				ParameterSize:     model.Details.ParameterSize,
				QuantizationLevel: model.Details.QuantizationLevel,
			}
		}
		result.InstalledModels = append(result.InstalledModels, installed)
	}
	sort.Slice(result.InstalledModels, func(i, j int) bool {
		return result.InstalledModels[i].ID < result.InstalledModels[j].ID
	})
	result.State = "success"
	result.DiscoveredAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result
}

func checkOllamaModelReadiness(ctx context.Context, endpoint, modelID string, client *http.Client) TranslationProviderReadiness {
	settings := defaultTranslationSettings()
	settings.Provider = translationProviderOllama
	settings.OllamaEndpoint = endpoint
	settings.OllamaModel = modelID
	provider, err := newOllamaTranslationProviderWithClient(settings, client)
	if err != nil {
		return TranslationProviderReadiness{State: translationReadinessInvalidConfiguration, Diagnostic: boundedTranslationReadinessDiagnostic(err.Error())}
	}
	return provider.TestConnection(ctx)
}

func ollamaModelReadinessResult(modelID string, readiness TranslationProviderReadiness, checkedAt time.Time) (ollamaModelReadinessResponse, int) {
	result := ollamaModelReadinessResponse{
		State:      "unavailable",
		ModelID:    modelID,
		CheckedAt:  checkedAt.UTC().Format(time.RFC3339Nano),
		Diagnostic: boundedTranslationReadinessDiagnostic(readiness.Diagnostic),
		Scope:      "metadata_check",
	}
	status := http.StatusServiceUnavailable
	switch readiness.State {
	case translationReadinessReady:
		result.State = "available"
		result.Available = true
		status = http.StatusOK
	case translationReadinessModelNotFound:
		result.State = "model_not_found"
		status = http.StatusNotFound
	case translationReadinessInvalidConfiguration:
		result.State = ollamaProviderStateInvalid
		status = http.StatusBadRequest
	default:
		if readiness.FailureState == "invalid_response" {
			result.State = "invalid_response"
			status = http.StatusBadGateway
		}
	}
	return result, status
}

func (h SettingsHandler) TestOllamaConnection(c *gin.Context) {
	if err := MigrateOllamaProviderSettings(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	configuration, err := loadOllamaProviderConfiguration(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if configuration.State == ollamaProviderStateMigrationConflict {
		c.JSON(http.StatusConflict, ollamaConnectionTestResult{State: configuration.State, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Diagnostic: "Ollama endpoint migration conflict must be resolved first"})
		return
	}
	if configuration.State != ollamaProviderStateConfigured {
		status := http.StatusBadRequest
		if configuration.State == ollamaProviderStateUnconfigured || configuration.State == ollamaProviderStateMigrationRequired {
			status = http.StatusConflict
		}
		c.JSON(status, ollamaConnectionTestResult{State: configuration.State, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Diagnostic: "Ollama provider endpoint is not usable"})
		return
	}
	result := testOllamaConnection(c.Request.Context(), configuration.Endpoint, h.ollamaProviderAPIClient())
	status := http.StatusOK
	if result.State == "unavailable" {
		status = http.StatusServiceUnavailable
	} else if result.State == "invalid_response" {
		status = http.StatusBadGateway
	} else if result.State == ollamaProviderStateInvalid {
		status = http.StatusBadRequest
	}
	c.JSON(status, result)
}

func (h SettingsHandler) ListOllamaModels(c *gin.Context) {
	if err := MigrateOllamaProviderSettings(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	configuration, err := loadOllamaProviderConfiguration(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if configuration.State != ollamaProviderStateConfigured {
		status := http.StatusConflict
		if configuration.State == ollamaProviderStateInvalid {
			status = http.StatusBadRequest
		}
		c.JSON(status, ollamaModelsResponse{State: configuration.State, InstalledModels: []ollamaInstalledModel{}, Diagnostic: "Ollama provider endpoint is not usable"})
		return
	}
	result := listOllamaInstalledModels(c.Request.Context(), configuration.Endpoint, h.ollamaProviderAPIClient())
	status := http.StatusOK
	if result.State == "unavailable" {
		status = http.StatusServiceUnavailable
	} else if result.State == "invalid_response" {
		status = http.StatusBadGateway
	}
	c.JSON(status, result)
}

func (h SettingsHandler) OllamaRuntimeStatus(c *gin.Context) {
	if err := MigrateOllamaProviderSettings(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, aiModelRuntimeStatus{State: "unavailable", Diagnostic: "Ollama provider configuration is unavailable", Models: []aiModelRuntimeModel{}, Operations: []aiModelRuntimeOperation{}})
		return
	}
	configuration, err := loadOllamaProviderConfiguration(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, aiModelRuntimeStatus{State: "unavailable", Diagnostic: "Ollama provider configuration is unavailable", Models: []aiModelRuntimeModel{}, Operations: []aiModelRuntimeOperation{}})
		return
	}
	if configuration.State != ollamaProviderStateConfigured {
		c.JSON(http.StatusServiceUnavailable, aiModelRuntimeStatus{State: "unavailable", Diagnostic: "Ollama provider configuration is not ready", Models: []aiModelRuntimeModel{}, Operations: []aiModelRuntimeOperation{}})
		return
	}
	result := h.aiModelLifecycle.runtimeStatus(c.Request.Context(), configuration.Endpoint)
	status := http.StatusOK
	if result.State != "available" {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, result)
}

func (h SettingsHandler) TestOllamaModelReadiness(c *gin.Context) {
	var request ollamaModelReadinessRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Ollama model readiness request"})
		return
	}
	modelID := strings.TrimSpace(request.ModelID)
	if modelID == "" || len(modelID) > 256 || strings.ContainsAny(modelID, "\x00\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modelId must be a non-empty model identifier up to 256 characters"})
		return
	}
	if err := MigrateOllamaProviderSettings(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	configuration, err := loadOllamaProviderConfiguration(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if configuration.State != ollamaProviderStateConfigured {
		status := http.StatusConflict
		if configuration.State == ollamaProviderStateInvalid {
			status = http.StatusBadRequest
		}
		c.JSON(status, ollamaModelReadinessResponse{State: configuration.State, ModelID: modelID, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Scope: "metadata_check"})
		return
	}
	readiness := checkOllamaModelReadiness(c.Request.Context(), configuration.Endpoint, modelID, h.ollamaHTTPClient)
	result, status := ollamaModelReadinessResult(modelID, readiness, time.Now())
	c.JSON(status, result)
}
