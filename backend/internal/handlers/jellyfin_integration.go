package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	jellyfinSettingsKey            = "jellyfin"
	jellyfinReasonPublished        = "asset_published"
	jellyfinReasonTestReady        = "test_encode_ready"
	jellyfinReasonTestRemoved      = "test_encode_removed"
	jellyfinReasonManual           = "manual_refresh"
	jellyfinStatusPending          = "pending"
	jellyfinStatusSuccess          = "success"
	jellyfinStatusSkippedDisabled  = "skipped_disabled"
	jellyfinStatusSkippedUnmapped  = "skipped_unmapped"
	jellyfinStatusRetryableFailure = "retryable_failure"
	jellyfinStatusPermanentError   = "permanent_configuration_error"
	jellyfinTargetServerWide       = "server-wide"
	jellyfinMaximumDebounce        = 60 * time.Second
	jellyfinMaximumAttempts        = 5
	jellyfinRequestTimeout         = 10 * time.Second
)

var jellyfinRetryDelays = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute}
var sharedJellyfinHTTPClient = &http.Client{Timeout: jellyfinRequestTimeout}
var activeJellyfinCoordinatorMu sync.RWMutex
var activeJellyfinCoordinator *JellyfinCoordinator

type jellyfinLibraryMapping struct {
	MVForgeLibraryID  uint   `json:"mvforgeLibraryId"`
	JellyfinLibraryID string `json:"jellyfinLibraryId"`
	JellyfinName      string `json:"jellyfinName,omitempty"`
}

type jellyfinSettings struct {
	Enabled             bool                     `json:"enabled"`
	BaseURL             string                   `json:"baseUrl"`
	APIToken            string                   `json:"apiToken,omitempty"`
	RefreshAfterPublish bool                     `json:"refreshAfterPublish"`
	RefreshTestEncode   bool                     `json:"refreshTestEncode"`
	DebounceSeconds     int                      `json:"debounceSeconds"`
	Mappings            []jellyfinLibraryMapping `json:"mappings"`
}

type jellyfinLibrary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CollectionType string `json:"collectionType,omitempty"`
}

type jellyfinConnectionResult struct {
	Reachable        bool              `json:"reachable"`
	Authenticated    bool              `json:"authenticated"`
	ServerVersion    string            `json:"serverVersion,omitempty"`
	VersionAvailable bool              `json:"versionAvailable"`
	Libraries        []jellyfinLibrary `json:"libraries"`
	FailureClass     string            `json:"failureClass,omitempty"`
}

type jellyfinOutcomeView struct {
	ID             uint      `json:"id"`
	Reason         string    `json:"reason"`
	Status         string    `json:"status"`
	EffectiveScope string    `json:"effectiveScope,omitempty"`
	FailureClass   string    `json:"failureClass,omitempty"`
	AttemptCount   int       `json:"attemptCount"`
	DurationMillis int64     `json:"durationMillis"`
	UpdatedAt      time.Time `json:"updatedAt"`
	PublicationID  string    `json:"publicationId,omitempty"`
	TestEncodeID   uint      `json:"testEncodeId,omitempty"`
	LibraryID      uint      `json:"libraryId,omitempty"`
	RetryEligible  bool      `json:"retryEligible"`
}

func defaultJellyfinSettings() jellyfinSettings {
	return jellyfinSettings{RefreshAfterPublish: true, RefreshTestEncode: true, DebounceSeconds: 10, Mappings: []jellyfinLibraryMapping{}}
}

func loadJellyfinSettingsChecked(db *gorm.DB) (jellyfinSettings, int64, error) {
	settings := defaultJellyfinSettings()
	if db == nil {
		return settings, 0, nil
	}
	var stored models.AppSetting
	err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).First(&stored, "key = ?", jellyfinSettingsKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return settings, 0, nil
	}
	if err != nil {
		return settings, 0, err
	}
	loaded, err := jellyfinSettingsFromValue(stored.Value, false)
	if err != nil {
		return loaded, stored.UpdatedAt.UnixNano(), err
	}
	return loaded, stored.UpdatedAt.UnixNano(), nil
}

func loadJellyfinSettings(db *gorm.DB) (jellyfinSettings, int64) {
	settings, revision, err := loadJellyfinSettingsChecked(db)
	if err != nil {
		return defaultJellyfinSettings(), 0
	}
	return settings, revision
}

func jellyfinSettingsFromValue(value models.JSONMap, strict bool) (jellyfinSettings, error) {
	settings := defaultJellyfinSettings()
	if raw, ok := value["enabled"]; ok {
		parsed, valid := raw.(bool)
		if !valid {
			return jellyfinSettingsValueError(strict, "enabled must be a boolean", settings)
		}
		settings.Enabled = parsed
	}
	if raw, ok := value["baseUrl"]; ok {
		parsed, valid := raw.(string)
		if !valid {
			return jellyfinSettingsValueError(strict, "baseUrl must be a string", settings)
		}
		settings.BaseURL = strings.TrimSpace(parsed)
		if len(settings.BaseURL) > 2048 {
			return jellyfinSettingsValueError(strict, "baseUrl is too long", settings)
		}
	}
	if raw, ok := value["apiToken"]; ok {
		parsed, valid := raw.(string)
		if !valid {
			return jellyfinSettingsValueError(strict, "apiToken must be a string", settings)
		}
		settings.APIToken = parsed
		if len(settings.APIToken) > 4096 {
			return jellyfinSettingsValueError(strict, "apiToken is too long", settings)
		}
	}
	for key, destination := range map[string]*bool{"refreshAfterPublish": &settings.RefreshAfterPublish, "refreshTestEncode": &settings.RefreshTestEncode} {
		if raw, ok := value[key]; ok {
			parsed, valid := raw.(bool)
			if !valid {
				return jellyfinSettingsValueError(strict, key+" must be a boolean", settings)
			}
			*destination = parsed
		}
	}
	if raw, ok := value["debounceSeconds"]; ok {
		parsed, valid := jellyfinInteger(raw)
		if !valid || parsed < 0 || parsed > 60 {
			return jellyfinSettingsValueError(strict, "debounceSeconds must be between 0 and 60", settings)
		}
		settings.DebounceSeconds = parsed
	}
	if raw, ok := value["mappings"]; ok {
		entries, valid := raw.([]any)
		if !valid {
			if typed, typedOK := raw.(models.JSONList); typedOK {
				entries = []any(typed)
				valid = true
			}
		}
		if !valid {
			return jellyfinSettingsValueError(strict, "mappings must be a list", settings)
		}
		if len(entries) > 500 {
			return jellyfinSettingsValueError(strict, "mappings must contain at most 500 entries", settings)
		}
		settings.Mappings = make([]jellyfinLibraryMapping, 0, len(entries))
		for _, entry := range entries {
			mapping, valid := jellyfinMapping(entry)
			if !valid || mapping.MVForgeLibraryID == 0 || strings.TrimSpace(mapping.JellyfinLibraryID) == "" || mapping.JellyfinLibraryID != strings.TrimSpace(mapping.JellyfinLibraryID) {
				return jellyfinSettingsValueError(strict, "each mapping must contain stable MVForge and Jellyfin Library IDs", settings)
			}
			if len(mapping.JellyfinLibraryID) > 256 || len(mapping.JellyfinName) > 512 {
				return jellyfinSettingsValueError(strict, "mapping values are too long", settings)
			}
			settings.Mappings = append(settings.Mappings, mapping)
		}
	}
	if settings.DebounceSeconds < 0 || settings.DebounceSeconds > 60 {
		return jellyfinSettingsValueError(strict, "debounceSeconds must be between 0 and 60", settings)
	}
	if settings.BaseURL != "" {
		parsed, err := url.Parse(settings.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return jellyfinSettingsValueError(strict, "baseUrl must be an HTTP or HTTPS server URL without credentials or query values", settings)
		}
		settings.BaseURL = strings.TrimRight(settings.BaseURL, "/")
	}
	if !strict {
		return settings, nil
	}
	seenMVForge := map[uint]bool{}
	seenJellyfin := map[string]bool{}
	for _, mapping := range settings.Mappings {
		if seenMVForge[mapping.MVForgeLibraryID] || seenJellyfin[mapping.JellyfinLibraryID] {
			return jellyfinSettings{}, fmt.Errorf("Library mappings must be unique")
		}
		seenMVForge[mapping.MVForgeLibraryID], seenJellyfin[mapping.JellyfinLibraryID] = true, true
	}
	return settings, nil
}

func jellyfinSettingsValueError(strict bool, message string, defaults jellyfinSettings) (jellyfinSettings, error) {
	if strict {
		return jellyfinSettings{}, fmt.Errorf("%s", message)
	}
	return defaults, nil
}

func jellyfinInteger(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case uint:
		return int(number), true
	case float64:
		if number == float64(int(number)) {
			return int(number), true
		}
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil
	}
	return 0, false
}

func jellyfinMapping(value any) (jellyfinLibraryMapping, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		if typed, typedOK := value.(models.JSONMap); typedOK {
			object, ok = map[string]any(typed), true
		}
	}
	if !ok {
		return jellyfinLibraryMapping{}, false
	}
	libraryID, valid := jellyfinInteger(object["mvforgeLibraryId"])
	if !valid || libraryID <= 0 {
		return jellyfinLibraryMapping{}, false
	}
	jellyfinID, ok := object["jellyfinLibraryId"].(string)
	if !ok {
		return jellyfinLibraryMapping{}, false
	}
	name, _ := object["jellyfinName"].(string)
	return jellyfinLibraryMapping{MVForgeLibraryID: uint(libraryID), JellyfinLibraryID: jellyfinID, JellyfinName: name}, true
}

func jellyfinSettingsStoredValue(settings jellyfinSettings) models.JSONMap {
	mappings := make(models.JSONList, 0, len(settings.Mappings))
	for _, mapping := range settings.Mappings {
		mappings = append(mappings, models.JSONMap{"mvforgeLibraryId": mapping.MVForgeLibraryID, "jellyfinLibraryId": mapping.JellyfinLibraryID, "jellyfinName": mapping.JellyfinName})
	}
	return models.JSONMap{
		"enabled": settings.Enabled, "baseUrl": settings.BaseURL, "apiToken": settings.APIToken,
		"refreshAfterPublish": settings.RefreshAfterPublish, "refreshTestEncode": settings.RefreshTestEncode,
		"debounceSeconds": settings.DebounceSeconds, "mappings": mappings,
	}
}

func jellyfinSettingsProjection(settings jellyfinSettings) models.JSONMap {
	mappings := make(models.JSONList, 0, len(settings.Mappings))
	for _, mapping := range settings.Mappings {
		entry := models.JSONMap{"mvforgeLibraryId": mapping.MVForgeLibraryID, "jellyfinLibraryId": mapping.JellyfinLibraryID}
		if mapping.JellyfinName != "" {
			entry["jellyfinName"] = mapping.JellyfinName
		}
		mappings = append(mappings, entry)
	}
	return models.JSONMap{
		"enabled": settings.Enabled, "baseUrl": settings.BaseURL,
		"credentialConfigured": settings.APIToken != "", "refreshAfterPublish": settings.RefreshAfterPublish,
		"refreshTestEncode": settings.RefreshTestEncode, "debounceSeconds": settings.DebounceSeconds, "mappings": mappings,
	}
}

func jellyfinSettingsForUpdate(db *gorm.DB, input models.JSONMap) (models.JSONMap, error) {
	previous, _, err := loadJellyfinSettingsChecked(db)
	if err != nil {
		return nil, fmt.Errorf("unable to load existing Jellyfin settings")
	}
	settings, err := jellyfinSettingsFromValue(input, true)
	if err != nil {
		return nil, err
	}
	if raw, present := input["apiToken"]; present {
		token, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("apiToken must be a string")
		}
		if token != "" {
			settings.APIToken = token
		} else {
			settings.APIToken = previous.APIToken
		}
	} else {
		settings.APIToken = previous.APIToken
	}
	if raw, present := input["clearCredential"]; present {
		clear, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("clearCredential must be a boolean")
		}
		if clear {
			settings.APIToken = ""
		}
	}
	seenMVForge := map[uint]bool{}
	seenJellyfin := map[string]bool{}
	for _, mapping := range settings.Mappings {
		if seenMVForge[mapping.MVForgeLibraryID] || seenJellyfin[mapping.JellyfinLibraryID] {
			return nil, fmt.Errorf("Library mappings must be unique")
		}
		seenMVForge[mapping.MVForgeLibraryID], seenJellyfin[mapping.JellyfinLibraryID] = true, true
		var library models.Library
		if err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).First(&library, mapping.MVForgeLibraryID).Error; err != nil {
			return nil, fmt.Errorf("mapping references an unknown MVForge Library ID")
		}
	}
	return jellyfinSettingsStoredValue(settings), nil
}

type jellyfinClient struct {
	http *http.Client
}

var sharedJellyfinClient = jellyfinClient{http: sharedJellyfinHTTPClient}

func (client jellyfinClient) getJSON(ctx context.Context, settings jellyfinSettings, endpoint string, authenticated bool, output any) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.BaseURL+endpoint, nil)
	if err != nil {
		return nil, &jellyfinFailure{class: "invalid_configuration", permanent: true}
	}
	if authenticated && settings.APIToken != "" {
		request.Header.Set("X-Emby-Token", settings.APIToken)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return nil, classifyJellyfinError(err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return response, classifyJellyfinError(readErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		failure := classifyJellyfinStatus(response.StatusCode)
		failure.retryAfter = jellyfinRetryAfter(response.Header.Get("Retry-After"))
		return response, failure
	}
	if output != nil && len(body) > 0 {
		if err := json.Unmarshal(body, output); err != nil {
			return response, &jellyfinFailure{class: "unexpected_response", permanent: true}
		}
	}
	return response, nil
}

func (client jellyfinClient) post(ctx context.Context, settings jellyfinSettings, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.BaseURL+endpoint, nil)
	if err != nil {
		return &jellyfinFailure{class: "invalid_configuration", permanent: true}
	}
	request.Header.Set("X-Emby-Token", settings.APIToken)
	response, err := client.http.Do(request)
	if err != nil {
		return classifyJellyfinError(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		failure := classifyJellyfinStatus(response.StatusCode)
		failure.retryAfter = jellyfinRetryAfter(response.Header.Get("Retry-After"))
		return failure
	}
	return nil
}

type jellyfinFailure struct {
	class      string
	permanent  bool
	retryAfter time.Duration
}

func (failure *jellyfinFailure) Error() string { return failure.class }

func classifyJellyfinError(err error) *jellyfinFailure {
	if errors.Is(err, context.DeadlineExceeded) {
		return &jellyfinFailure{class: "timeout"}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &jellyfinFailure{class: "timeout"}
	}
	return &jellyfinFailure{class: "network_error"}
}

func classifyJellyfinStatus(status int) *jellyfinFailure {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &jellyfinFailure{class: "invalid_credentials", permanent: true}
	case http.StatusTooManyRequests:
		return &jellyfinFailure{class: "rate_limited"}
	case http.StatusNotFound, http.StatusBadRequest:
		return &jellyfinFailure{class: "invalid_configuration", permanent: true}
	default:
		if status >= 500 {
			return &jellyfinFailure{class: "server_error"}
		}
		return &jellyfinFailure{class: "unexpected_response", permanent: true}
	}
}

func jellyfinRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		if seconds > int((2*time.Minute)/time.Second) {
			return 2 * time.Minute
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if delay := time.Until(at); delay > 0 {
			if delay > 2*time.Minute {
				return 2 * time.Minute
			}
			return delay
		}
	}
	return 0
}

func (client jellyfinClient) testConnection(ctx context.Context, settings jellyfinSettings) jellyfinConnectionResult {
	result := jellyfinConnectionResult{Libraries: []jellyfinLibrary{}}
	var serverInfo struct {
		Version string `json:"Version"`
	}
	if response, err := client.getJSON(ctx, settings, "/System/Info/Public", false, &serverInfo); response != nil {
		result.Reachable = true
		if err == nil {
			result.ServerVersion = strings.TrimSpace(serverInfo.Version)
			result.VersionAvailable = result.ServerVersion != ""
		}
	}
	libraries, failure := client.discoverLibraries(ctx, settings)
	if failure != nil {
		result.FailureClass = failure.class
		return result
	}
	result.Reachable, result.Authenticated, result.Libraries = true, true, libraries
	return result
}

func (client jellyfinClient) discoverLibraries(ctx context.Context, settings jellyfinSettings) ([]jellyfinLibrary, *jellyfinFailure) {
	var response struct {
		Items []struct {
			ID             string `json:"Id"`
			Name           string `json:"Name"`
			CollectionType string `json:"CollectionType"`
		} `json:"Items"`
	}
	_, err := client.getJSON(ctx, settings, "/Library/MediaFolders", true, &response)
	if err != nil {
		failure, ok := err.(*jellyfinFailure)
		if !ok {
			failure = &jellyfinFailure{class: "unexpected_response", permanent: true}
		}
		return nil, failure
	}
	libraries := make([]jellyfinLibrary, 0, len(response.Items))
	for _, item := range response.Items {
		if item.ID == "" {
			return nil, &jellyfinFailure{class: "unexpected_response", permanent: true}
		}
		libraries = append(libraries, jellyfinLibrary{ID: item.ID, Name: item.Name, CollectionType: item.CollectionType})
	}
	return libraries, nil
}

type jellyfinTrigger struct {
	Reason        string
	SourceKey     string
	LibraryID     uint
	PublicationID string
	TestEncodeID  uint
	Reconcile     bool
}

func recordJellyfinTrigger(db *gorm.DB, trigger jellyfinTrigger) error {
	if db == nil || trigger.SourceKey == "" || !validJellyfinReason(trigger.Reason) {
		return nil
	}
	settings, revision, settingsErr := loadJellyfinSettingsChecked(db)
	if settingsErr != nil {
		return fmt.Errorf("unable to load Jellyfin integration settings")
	}
	status := jellyfinStatusPending
	var mappedID string
	enabledForReason := jellyfinSettingsEnabledForReason(settings, trigger.Reason)
	if !enabledForReason {
		status = jellyfinStatusSkippedDisabled
	} else {
		for _, mapping := range settings.Mappings {
			if mapping.MVForgeLibraryID == trigger.LibraryID {
				mappedID = mapping.JellyfinLibraryID
				break
			}
		}
		if mappedID == "" {
			status = jellyfinStatusSkippedUnmapped
		}
	}
	now := time.Now().UTC()
	window := time.Duration(settings.DebounceSeconds) * time.Second
	target := jellyfinTargetServerWide
	err := db.Transaction(func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		var existing models.JellyfinIntegrationOutcome
		lookup := tx.Where("source_key = ?", trigger.SourceKey).First(&existing)
		if lookup.Error == nil {
			if !trigger.Reconcile || existing.Status == jellyfinStatusPending {
				return nil
			}
		} else if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		first := now
		var pending []models.JellyfinIntegrationOutcome
		if status == jellyfinStatusPending {
			if err := tx.Where("status = ? AND effective_target = ? AND config_revision = ?", jellyfinStatusPending, target, revision).Find(&pending).Error; err != nil {
				return err
			}
			for _, item := range pending {
				if item.FirstTriggeredAt.Before(first) {
					first = item.FirstTriggeredAt
				}
			}
		}
		due := now.Add(window)
		maximumDue := first.Add(jellyfinMaximumDebounce)
		if due.After(maximumDue) {
			due = maximumDue
		}
		if status == jellyfinStatusPending {
			for _, item := range pending {
				if item.NextAttemptAt != nil && item.NextAttemptAt.After(due) {
					due = item.NextAttemptAt.UTC()
				}
			}
			pendingIDs := make([]uint, 0, len(pending))
			for _, item := range pending {
				pendingIDs = append(pendingIDs, item.ID)
			}
			if len(pendingIDs) > 0 {
				if err := tx.Model(&models.JellyfinIntegrationOutcome{}).Where("id IN ?", pendingIDs).Update("due_at", due).Error; err != nil {
					return err
				}
			}
		}
		outcome := models.JellyfinIntegrationOutcome{
			SourceKey: trigger.SourceKey, Reason: trigger.Reason, PublicationID: trigger.PublicationID,
			TestEncodeID: trigger.TestEncodeID, LibraryID: trigger.LibraryID, MappedLibraryID: mappedID,
			EffectiveTarget: target, ConfigRevision: revision, Status: status,
			FirstTriggeredAt: first, DueAt: due, CreatedAt: now, UpdatedAt: now,
		}
		if status != jellyfinStatusPending {
			outcome.DueAt = now
		}
		if lookup.Error == nil {
			outcome.ID, outcome.AttemptCount, outcome.EffectiveScope = existing.ID, existing.AttemptCount, existing.EffectiveScope
			if err := tx.Model(&existing).Updates(map[string]any{
				"reason": outcome.Reason, "publication_id": outcome.PublicationID, "test_encode_id": outcome.TestEncodeID,
				"library_id": outcome.LibraryID, "mapped_library_id": outcome.MappedLibraryID,
				"effective_target": outcome.EffectiveTarget, "config_revision": outcome.ConfigRevision,
				"status": outcome.Status, "failure_class": "", "duration_millis": 0,
				"first_triggered_at": outcome.FirstTriggeredAt, "due_at": outcome.DueAt,
				"next_attempt_at": nil, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			return trimJellyfinLedger(tx, now)
		}
		if err := tx.Create(&outcome).Error; err != nil {
			return err
		}
		return trimJellyfinLedger(tx, now)
	})
	if err == nil {
		signalJellyfinCoordinator()
	}
	return err
}

func trimJellyfinLedger(tx *gorm.DB, now time.Time) error {
	ledger := &models.JellyfinIntegrationOutcome{}
	terminal := tx.Model(ledger).Where("status <> ?", jellyfinStatusPending)
	if err := terminal.Where("updated_at < ?", now.Add(-180*24*time.Hour)).Delete(ledger).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Model(ledger).Where("status <> ?", jellyfinStatusPending).Count(&count).Error; err != nil {
		return err
	}
	if count <= 2000 {
		return nil
	}
	ids := make([]uint, 0, count-2000)
	if err := tx.Model(ledger).Where("status <> ?", jellyfinStatusPending).Order("updated_at asc, id asc").Limit(int(count-2000)).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return tx.Where("id IN ?", ids).Delete(ledger).Error
}

func jellyfinSettingsEnabledForReason(settings jellyfinSettings, reason string) bool {
	if !settings.Enabled {
		return false
	}
	switch reason {
	case jellyfinReasonPublished:
		return settings.RefreshAfterPublish
	case jellyfinReasonTestReady, jellyfinReasonTestRemoved:
		return settings.RefreshTestEncode
	case jellyfinReasonManual:
		return true
	default:
		return false
	}
}

func validJellyfinReason(reason string) bool {
	switch reason {
	case jellyfinReasonPublished, jellyfinReasonTestReady, jellyfinReasonTestRemoved, jellyfinReasonManual:
		return true
	default:
		return false
	}
}

func recordInvalidAssetPublished(db *gorm.DB, eventID, publicationID string) error {
	if eventID == "" {
		return nil
	}
	now := time.Now().UTC()
	outcome := models.JellyfinIntegrationOutcome{
		SourceKey: "asset-published:" + eventID, Reason: jellyfinReasonPublished,
		PublicationID: publicationID, EffectiveTarget: jellyfinTargetServerWide,
		Status: jellyfinStatusPermanentError, FailureClass: "invalid_event",
		FirstTriggeredAt: now, DueAt: now, CreatedAt: now, UpdatedAt: now,
	}
	err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_key"}}, DoNothing: true}).Create(&outcome).Error
	if err == nil {
		signalJellyfinCoordinator()
	}
	return err
}

func testEncodeJellyfinSourceKey(reason string, testID uint) string {
	return fmt.Sprintf("%s:%d", reason, testID)
}

func recordJellyfinTestEncodeReady(db *gorm.DB, testID, libraryID uint, readyPersisted, validationPassed bool) error {
	if !readyPersisted || !validationPassed || testID == 0 || libraryID == 0 {
		return nil
	}
	return recordJellyfinTrigger(db, jellyfinTrigger{
		Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, testID),
		LibraryID: libraryID, TestEncodeID: testID,
	})
}

type JellyfinCoordinator struct {
	db         *gorm.DB
	stop       chan struct{}
	wake       chan struct{}
	once       sync.Once
	unregister func()
}

func StartJellyfinCoordinator(db *gorm.DB) *JellyfinCoordinator {
	activeJellyfinCoordinatorMu.Lock()
	if activeJellyfinCoordinator != nil {
		coordinator := activeJellyfinCoordinator
		activeJellyfinCoordinatorMu.Unlock()
		return coordinator
	}
	coordinator := &JellyfinCoordinator{db: db, stop: make(chan struct{}), wake: make(chan struct{}, 1)}
	activeJellyfinCoordinator = coordinator
	coordinator.unregister = RegisterAssetPublishedSubscriber(coordinator.assetPublished)
	activeJellyfinCoordinatorMu.Unlock()
	go coordinator.run()
	return coordinator
}

func (coordinator *JellyfinCoordinator) Stop() {
	if coordinator == nil {
		return
	}
	coordinator.once.Do(func() {
		close(coordinator.stop)
		activeJellyfinCoordinatorMu.Lock()
		if activeJellyfinCoordinator == coordinator {
			activeJellyfinCoordinator = nil
		}
		activeJellyfinCoordinatorMu.Unlock()
		if coordinator.unregister != nil {
			coordinator.unregister()
		}
	})
}

func (coordinator *JellyfinCoordinator) assetPublished(ctx context.Context, eventID string, payload models.JSONMap) error {
	publicationID, _ := payload["publicationId"].(string)
	libraryID, ok := jellyfinUint(payload["libraryId"])
	if strings.TrimSpace(eventID) == "" {
		return errors.New("AssetPublished is missing its stable event identity")
	}
	if !ok || libraryID == 0 || strings.TrimSpace(publicationID) == "" {
		return recordInvalidAssetPublished(coordinator.db, eventID, publicationID)
	}
	return recordJellyfinTrigger(coordinator.db, jellyfinTrigger{
		Reason: jellyfinReasonPublished, SourceKey: "asset-published:" + eventID,
		LibraryID: libraryID, PublicationID: publicationID,
	})
}

func jellyfinUint(value any) (uint, bool) {
	if number, ok := jellyfinInteger(value); ok && number > 0 {
		return uint(number), true
	}
	if text, ok := value.(string); ok {
		parsed, err := strconv.ParseUint(text, 10, 32)
		return uint(parsed), err == nil && parsed > 0
	}
	return 0, false
}

func (coordinator *JellyfinCoordinator) run() {
	for {
		coordinator.processDue()
		delay := coordinator.nextDelay()
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-coordinator.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-coordinator.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}

func (coordinator *JellyfinCoordinator) nextDelay() time.Duration {
	var next models.JellyfinIntegrationOutcome
	if err := coordinator.db.Where("status = ?", jellyfinStatusPending).Order("due_at asc, id asc").First(&next).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return time.Hour
		}
		return time.Second
	}
	delay := time.Until(next.DueAt)
	if delay < 50*time.Millisecond {
		return 50 * time.Millisecond
	}
	return delay
}

func signalJellyfinCoordinator() {
	activeJellyfinCoordinatorMu.RLock()
	coordinator := activeJellyfinCoordinator
	activeJellyfinCoordinatorMu.RUnlock()
	if coordinator == nil {
		return
	}
	select {
	case coordinator.wake <- struct{}{}:
	default:
	}
}

func (coordinator *JellyfinCoordinator) processDue() {
	if coordinator == nil || coordinator.db == nil {
		return
	}
	now := time.Now().UTC()
	var first models.JellyfinIntegrationOutcome
	if err := coordinator.db.Where("status = ? AND due_at <= ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", jellyfinStatusPending, now, now).Order("due_at asc, id asc").First(&first).Error; err != nil {
		return
	}
	var batch []models.JellyfinIntegrationOutcome
	if err := coordinator.db.Where("status = ? AND effective_target = ? AND config_revision = ?", jellyfinStatusPending, first.EffectiveTarget, first.ConfigRevision).Order("id asc").Find(&batch).Error; err != nil || len(batch) == 0 {
		return
	}
	for _, item := range batch {
		if item.DueAt.After(now) || (item.NextAttemptAt != nil && item.NextAttemptAt.After(now)) {
			return
		}
	}
	settings, revision, settingsErr := loadJellyfinSettingsChecked(coordinator.db)
	if settingsErr != nil {
		return
	}
	if revision != first.ConfigRevision {
		coordinator.finishBatch(batch, jellyfinStatusPermanentError, "configuration_changed", "", 0, now)
		return
	}
	if !jellyfinSettingsEnabledForReason(settings, first.Reason) {
		coordinator.finishBatch(batch, jellyfinStatusSkippedDisabled, "", "", 0, now)
		return
	}
	maxAttempts := 0
	for _, item := range batch {
		if item.AttemptCount > maxAttempts {
			maxAttempts = item.AttemptCount
		}
	}
	if maxAttempts >= jellyfinMaximumAttempts {
		coordinator.finishBatch(batch, jellyfinStatusRetryableFailure, "request_outcome_unknown", "server_wide", 0, now)
		return
	}
	attempt := maxAttempts + 1
	delay := jellyfinBackoff(attempt)
	nextAttempt := now.Add(delay)
	if attempt >= jellyfinMaximumAttempts {
		nextAttempt = now
	}
	ids := make([]uint, 0, len(batch))
	for _, item := range batch {
		ids = append(ids, item.ID)
	}
	if err := coordinator.db.Model(&models.JellyfinIntegrationOutcome{}).Where("id IN ? AND status = ?", ids, jellyfinStatusPending).Updates(map[string]any{
		"attempt_count": attempt, "last_attempt_at": now, "next_attempt_at": nextAttempt,
		"due_at": nextAttempt, "effective_scope": "server_wide", "updated_at": now,
	}).Error; err != nil {
		return
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), jellyfinRequestTimeout)
	failure := coordinator.refreshBatch(ctx, settings, batch)
	cancel()
	duration := time.Since(start).Milliseconds()
	finished := time.Now().UTC()
	if failure == nil {
		coordinator.finishBatch(batch, jellyfinStatusSuccess, "", "server_wide", duration, finished)
		return
	}
	if failure.permanent {
		coordinator.finishBatch(batch, jellyfinStatusPermanentError, failure.class, "", duration, finished)
		return
	}
	if attempt >= jellyfinMaximumAttempts {
		coordinator.finishBatch(batch, jellyfinStatusRetryableFailure, failure.class, "", duration, finished)
		return
	}
	if failure.retryAfter > 0 {
		delay = failure.retryAfter
		if delay > 2*time.Minute {
			delay = 2 * time.Minute
		}
		nextAttempt = finished.Add(delay)
	}
	_ = coordinator.db.Model(&models.JellyfinIntegrationOutcome{}).Where("id IN ? AND status = ?", ids, jellyfinStatusPending).Updates(map[string]any{
		"failure_class": failure.class, "next_attempt_at": nextAttempt,
		"due_at": nextAttempt, "duration_millis": duration, "updated_at": finished,
	}).Error
}

func jellyfinBackoff(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	index := attempt - 1
	if index >= len(jellyfinRetryDelays) {
		return jellyfinRetryDelays[len(jellyfinRetryDelays)-1]
	}
	return jellyfinRetryDelays[index]
}

func (coordinator *JellyfinCoordinator) refreshBatch(ctx context.Context, settings jellyfinSettings, batch []models.JellyfinIntegrationOutcome) *jellyfinFailure {
	if !settings.Enabled || settings.APIToken == "" || settings.BaseURL == "" {
		return &jellyfinFailure{class: "invalid_configuration", permanent: true}
	}
	client := sharedJellyfinClient
	libraries, failure := client.discoverLibraries(ctx, settings)
	if failure != nil {
		return failure
	}
	available := make(map[string]bool, len(libraries))
	for _, library := range libraries {
		available[library.ID] = true
	}
	for _, outcome := range batch {
		if outcome.MappedLibraryID == "" || !available[outcome.MappedLibraryID] {
			return &jellyfinFailure{class: "stale_mapping", permanent: true}
		}
	}
	// Jellyfin's item refresh API refreshes metadata for an item; the contract
	// does not guarantee descendant additions/removals. Use the explicit broad
	// scan endpoint until Jellyfin documents a reliable scoped library scan.
	returnFailure := client.post(ctx, settings, "/Library/Refresh")
	if returnFailure == nil {
		return nil
	}
	failure, ok := returnFailure.(*jellyfinFailure)
	if !ok {
		return &jellyfinFailure{class: "unexpected_response", permanent: true}
	}
	return failure
}

func (coordinator *JellyfinCoordinator) finishBatch(batch []models.JellyfinIntegrationOutcome, status, failureClass, scope string, duration int64, at time.Time) {
	ids := make([]uint, 0, len(batch))
	for _, item := range batch {
		ids = append(ids, item.ID)
	}
	updates := map[string]any{
		"status": status, "failure_class": failureClass, "effective_scope": scope,
		"duration_millis": duration, "next_attempt_at": nil, "updated_at": at,
	}
	if scope == "" {
		delete(updates, "effective_scope")
	}
	_ = coordinator.db.Model(&models.JellyfinIntegrationOutcome{}).Where("id IN ? AND status = ?", ids, jellyfinStatusPending).Updates(updates).Error
}

func (handler SettingsHandler) TestJellyfinConnection(c *gin.Context) {
	settings, _, err := loadJellyfinSettingsChecked(handler.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jellyfinConnectionResult{Libraries: []jellyfinLibrary{}, FailureClass: "integration_state_unavailable"})
		return
	}
	if settings.BaseURL == "" || settings.APIToken == "" {
		c.JSON(http.StatusOK, jellyfinConnectionResult{Libraries: []jellyfinLibrary{}, FailureClass: "invalid_configuration"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), jellyfinRequestTimeout)
	defer cancel()
	c.JSON(http.StatusOK, sharedJellyfinClient.testConnection(ctx, settings))
}

type JellyfinHandler struct {
	db *gorm.DB
}

func NewJellyfinHandler(db *gorm.DB) JellyfinHandler { return JellyfinHandler{db: db} }

func (handler JellyfinHandler) Outcomes(c *gin.Context) {
	var rows []models.JellyfinIntegrationOutcome
	if err := handler.db.Order("updated_at desc").Limit(200).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to load Jellyfin integration outcomes"})
		return
	}
	settings, revision, settingsErr := loadJellyfinSettingsChecked(handler.db)
	if settingsErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to load Jellyfin integration settings"})
		return
	}
	outcomes := make([]jellyfinOutcomeView, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, jellyfinOutcomeView{
			ID: row.ID, Reason: row.Reason, Status: row.Status, EffectiveScope: row.EffectiveScope,
			FailureClass: row.FailureClass, AttemptCount: row.AttemptCount, DurationMillis: row.DurationMillis,
			UpdatedAt: row.UpdatedAt, PublicationID: row.PublicationID, TestEncodeID: row.TestEncodeID, LibraryID: row.LibraryID,
			RetryEligible: jellyfinOutcomeRetryEligible(row, settings, revision),
		})
	}
	c.JSON(http.StatusOK, outcomes)
}

func (handler JellyfinHandler) ManualRefresh(c *gin.Context) {
	var input struct {
		LibraryID uint `json:"libraryId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.LibraryID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "libraryId is required"})
		return
	}
	now := time.Now().UTC()
	key := "manual:" + strconv.FormatInt(now.UnixNano(), 10)
	if err := recordJellyfinTrigger(handler.db, jellyfinTrigger{Reason: jellyfinReasonManual, SourceKey: key, LibraryID: input.LibraryID}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to record Jellyfin refresh request"})
		return
	}
	var outcome models.JellyfinIntegrationOutcome
	if err := handler.db.First(&outcome, "source_key = ?", key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to load Jellyfin refresh request"})
		return
	}
	c.JSON(http.StatusAccepted, jellyfinOutcomeView{ID: outcome.ID, Reason: outcome.Reason, Status: outcome.Status, EffectiveScope: outcome.EffectiveScope, AttemptCount: outcome.AttemptCount, UpdatedAt: outcome.UpdatedAt, LibraryID: outcome.LibraryID})
}

func (handler JellyfinHandler) Retry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid integration outcome ID"})
		return
	}
	var outcome models.JellyfinIntegrationOutcome
	if err := handler.db.First(&outcome, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "integration outcome was not found"})
		return
	}
	settings, revision := loadJellyfinSettings(handler.db)
	if !jellyfinOutcomeRetryEligible(outcome, settings, revision) {
		c.JSON(http.StatusConflict, gin.H{"error": "Jellyfin outcome is not currently eligible for retry"})
		return
	}
	if !jellyfinSettingsEnabledForReason(settings, outcome.Reason) || settings.APIToken == "" || settings.BaseURL == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "Jellyfin settings are not ready"})
		return
	}
	mappedLibraryID := ""
	for _, mapping := range settings.Mappings {
		if mapping.MVForgeLibraryID == outcome.LibraryID {
			mappedLibraryID = mapping.JellyfinLibraryID
			break
		}
	}
	if mappedLibraryID == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "no Jellyfin Library mapping is configured for this outcome"})
		return
	}
	now := time.Now().UTC()
	var updated models.JellyfinIntegrationOutcome
	err = handler.db.Transaction(func(tx *gorm.DB) error {
		var current models.JellyfinIntegrationOutcome
		if err := tx.First(&current, outcome.ID).Error; err != nil {
			return err
		}
		if !jellyfinOutcomeRetryEligible(current, settings, revision) {
			return gorm.ErrInvalidData
		}
		if err := tx.Model(&current).Updates(map[string]any{
			"status": jellyfinStatusPending, "config_revision": revision, "attempt_count": 0,
			"failure_class": "", "effective_target": jellyfinTargetServerWide,
			"mapped_library_id": mappedLibraryID,
			"due_at":            now, "first_triggered_at": now, "next_attempt_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.First(&updated, current.ID).Error
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "unable to queue Jellyfin retry"})
		return
	}
	signalJellyfinCoordinator()
	c.JSON(http.StatusAccepted, jellyfinOutcomeView{ID: updated.ID, Reason: updated.Reason, Status: updated.Status, AttemptCount: updated.AttemptCount, UpdatedAt: updated.UpdatedAt, LibraryID: updated.LibraryID})
}

func jellyfinOutcomeRetryEligible(outcome models.JellyfinIntegrationOutcome, settings jellyfinSettings, revision int64) bool {
	if !jellyfinSettingsEnabledForReason(settings, outcome.Reason) || settings.APIToken == "" || settings.BaseURL == "" {
		return false
	}
	mapped := false
	for _, mapping := range settings.Mappings {
		if mapping.MVForgeLibraryID == outcome.LibraryID {
			mapped = mapping.JellyfinLibraryID != ""
			break
		}
	}
	if !mapped {
		return false
	}
	if outcome.Status == jellyfinStatusRetryableFailure {
		return true
	}
	return outcome.Status == jellyfinStatusPermanentError && outcome.FailureClass != "invalid_event" && revision != outcome.ConfigRevision
}
