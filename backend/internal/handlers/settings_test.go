package handlers

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSettingsUpdateUpsertsExistingKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-upsert?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "trackProfilePathAssignments", Value: models.JSONMap{"assignments": models.JSONMap{"old": "profile"}}}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/trackProfilePathAssignments", strings.NewReader(`{"value":{"assignments":{"new":"profile"}}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var count int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "trackProfilePathAssignments").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings count=%d", count)
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "trackProfilePathAssignments").Error; err != nil {
		t.Fatal(err)
	}
	assignments, ok := setting.Value["assignments"].(map[string]any)
	if !ok || assignments["new"] != "profile" {
		t.Fatalf("unexpected value: %#v", setting.Value)
	}
}

func TestSettingsRejectsInvalidAnalysisPolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-analysis-policy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/analysisPolicy", strings.NewReader(`{"value":{"mode":"balanced","concurrentAssets":8}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "between 1 and 4") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSettingsPersistsCanonicalCustomAnalysisPolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-custom-analysis-policy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	body := `{"value":{"mode":"custom","adaptiveAnalysis":true,"earlyConfidenceEnabled":true,"earlyConfidenceThreshold":0.97,"initialWindows":3,"maximumWindows":5,"windowSeconds":25,"positions":[0.08,0.27,0.5,0.73,0.92],"interlaceValidation":"always","cropDepth":"full","reuseSnapshots":true,"incrementalRefresh":true,"concurrentAssets":2}}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/analysisPolicy", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	policy := analysisPolicy(db)
	if policy.Mode != "custom" || policy.InitialWindows != 3 || policy.MaximumWindows != 5 || policy.EarlyConfidenceThreshold != 0.97 || policy.InterlaceValidation != "always" || policy.CropDepth != "full" || policy.ConcurrentAssets != 2 {
		t.Fatalf("unexpected persisted policy: %#v", policy)
	}
	if !slices.Equal(policy.Positions, []float64{0.08, 0.27, 0.5, 0.73, 0.92}) {
		t.Fatalf("positions were not persisted canonically: %v", policy.Positions)
	}
}

func TestSettingsRejectsDisablingAssignedAudioProfile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-assigned-profile?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProfileAssignment{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: "path", TargetPath: "/media/raw/Show", MediaType: "audio", Selection: "profile", ProfileKey: "show-audio"}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/audioEnhancementProfiles", strings.NewReader(`{"value":{"profiles":[{"key":"show-audio","scope":"path","disabled":true}]}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "cannot be disabled") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSettingsNormalizesPathTrackProfilesButPreservesAssetSelections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-track-profile-scope?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	body := `{"value":{"profiles":[{"key":"path-rules","scope":"path","audioMode":"languages","subtitleDisposition":"keep_and_extract","subtitleRules":[{"language":"spa","action":"keep"},{"streamIndex":4,"action":"remove"}],"attachmentPolicy":"auto","chapterPolicy":"keep","keepAudioStreams":[1],"audioMetadata":{"1":{"default":true}},"subtitleTransforms":[{"streamIndex":4,"format":"srt"}]},{"key":"asset-exact","scope":"asset","subtitleDisposition":"remove","subtitleRules":[{"streamIndex":7,"action":"extract"}],"attachmentPolicy":"remove","chapterPolicy":"remove","keepAudioStreams":[2],"keepSubtitleStreams":[]}]}}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/trackProfiles", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "trackProfiles").Error; err != nil {
		t.Fatal(err)
	}
	profiles := settingProfileValues(setting.Value["profiles"])
	if len(profiles) != 2 {
		t.Fatalf("profiles=%#v", setting.Value["profiles"])
	}
	pathProfile := settingProfileObject(profiles[0])
	for _, key := range []string{"keepAudioStreams", "audioMetadata", "subtitleTransforms"} {
		if _, exists := pathProfile[key]; exists {
			t.Fatalf("Path profile persisted asset-only field %q: %#v", key, pathProfile)
		}
	}
	if pathProfile["subtitleDisposition"] != "keep_and_extract" || pathProfile["attachmentPolicy"] != "auto" || pathProfile["chapterPolicy"] != "keep" {
		t.Fatalf("Path semantic policies were not preserved: %#v", pathProfile)
	}
	if rules := workerSliceValue(pathProfile["subtitleRules"]); len(rules) != 1 || workerStringValue(settingProfileObject(rules[0])["language"]) != "spa" {
		t.Fatalf("Path profile did not retain only semantic subtitle rules: %#v", pathProfile["subtitleRules"])
	}
	assetProfile := settingProfileObject(profiles[1])
	if indexes := workerSliceValue(assetProfile["keepAudioStreams"]); len(indexes) != 1 || streamIndexValue(indexes[0]) != 2 {
		t.Fatalf("Asset audio selection was lost: %#v", assetProfile)
	}
	if indexes, exists := assetProfile["keepSubtitleStreams"]; !exists || len(workerSliceValue(indexes)) != 0 {
		t.Fatalf("Asset remove-all selection was lost: %#v", assetProfile)
	}
	if assetProfile["subtitleDisposition"] != "remove" || assetProfile["attachmentPolicy"] != "remove" || assetProfile["chapterPolicy"] != "remove" {
		t.Fatalf("Asset policies were not preserved: %#v", assetProfile)
	}
	if rules := workerSliceValue(assetProfile["subtitleRules"]); len(rules) != 1 || streamIndexValue(settingProfileObject(rules[0])["streamIndex"]) != 7 {
		t.Fatalf("Asset stream-specific subtitle rule was lost: %#v", assetProfile["subtitleRules"])
	}
}

func TestSettingsRejectsInvalidTrackDomainPolicies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-track-domain-invalid?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	for name, body := range map[string]string{
		"subtitle":     `{"value":{"profiles":[{"key":"invalid","subtitleDisposition":"copy"}]}}`,
		"attachment":   `{"value":{"profiles":[{"key":"invalid","attachmentPolicy":"fonts_only"}]}}`,
		"chapter":      `{"value":{"profiles":[{"key":"invalid","chapterPolicy":"auto"}]}}`,
		"rule":         `{"value":{"profiles":[{"key":"invalid","subtitleRules":[{"language":"eng","action":"copy"}]}]}}`,
		"empty rule":   `{"value":{"profiles":[{"key":"invalid","subtitleRules":[{"language":"","action":"remove"}]}]}}`,
		"version":      `{"value":{"profiles":[{"key":"invalid","trackDispositionVersion":2}]}}`,
		"version type": `{"value":{"profiles":[{"key":"invalid","trackDispositionVersion":"1"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/settings/trackProfiles", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestNormalizeTrackProfilesPreservesPerProfileMigrationMarker(t *testing.T) {
	value := normalizeTrackProfilesForStorage(models.JSONMap{"profiles": models.JSONList{
		models.JSONMap{"key": "legacy", "subtitleDisposition": "keep", "attachmentPolicy": "auto", "chapterPolicy": "keep"},
		models.JSONMap{"key": "canonical", "trackDispositionVersion": 1, "subtitleDisposition": "remove", "attachmentPolicy": "remove", "chapterPolicy": "remove"},
	}})
	profiles := settingProfileValues(value["profiles"])
	legacy, canonical := settingProfileObject(profiles[0]), settingProfileObject(profiles[1])
	if _, exists := legacy["trackDispositionVersion"]; exists {
		t.Fatalf("legacy profile was silently migrated: %#v", legacy)
	}
	if intValueSetting(canonical["trackDispositionVersion"], 0) != 1 {
		t.Fatalf("canonical marker was lost: %#v", canonical)
	}
}

func TestMVForgePreferencesValidationRequiresDraftOnlyPreferenceFields(t *testing.T) {
	valid := models.JSONMap{
		"qualityGoal": "balanced", "executionPreference": "hardware", "preferredVideoEncoder": "hevc_qsv",
		"preferredLanguages": models.JSONList{"jpn", "spa", "eng"},
	}
	if err := validateMVForgePreferences(valid); err != nil {
		t.Fatalf("valid preferences rejected: %v", err)
	}
	invalid := models.JSONMap{"qualityGoal": "balanced", "executionPreference": "automatic", "preferredVideoEncoder": "auto", "preferredLanguages": models.JSONList{"jpn"}}
	if err := validateMVForgePreferences(invalid); err == nil {
		t.Fatal("unsupported execution preference was accepted")
	}
	mismatched := models.JSONMap{"qualityGoal": "balanced", "executionPreference": "hardware", "preferredVideoEncoder": "libx265", "preferredLanguages": models.JSONList{"jpn"}}
	if err := validateMVForgePreferences(mismatched); err == nil {
		t.Fatal("software encoder was accepted as a hardware preference")
	}
}

func TestOriginalRetentionPolicyValidation(t *testing.T) {
	valid := models.JSONMap{
		"keepOriginalsDays": 30, "autoDeleteEnabled": true,
		"processedOriginalsPath": "/media/originals_archive/processed-originals",
	}
	if err := validateOriginalRetentionPolicy(valid); err != nil {
		t.Fatalf("valid retention policy rejected: %v", err)
	}
	for name, value := range map[string]models.JSONMap{
		"negative days":  {"keepOriginalsDays": -1, "processedOriginalsPath": "/archive"},
		"relative path":  {"keepOriginalsDays": 30, "processedOriginalsPath": "archive"},
		"delete forever": {"keepOriginalsDays": 0, "autoDeleteEnabled": true, "processedOriginalsPath": "/archive"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateOriginalRetentionPolicy(value); err == nil {
				t.Fatal("invalid retention policy was accepted")
			}
		})
	}
}
