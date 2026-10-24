package handlers

import (
	"context"
	"net/http"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

type settingsMutationContext struct {
	ctx     context.Context
	db      *gorm.DB
	secrets ProtectedSecretSource
}

type settingDefinition struct {
	Key       string
	Domain    string
	Validate  func(settingsMutationContext, models.JSONMap) error
	Normalize func(settingsMutationContext, models.JSONMap) (models.JSONMap, error)
	Project   func(settingsMutationContext, models.JSONMap) (models.JSONMap, error)
	AfterSave func(settingsMutationContext) error
}

type settingPolicyError struct {
	status  int
	message string
}

func (err settingPolicyError) Error() string { return err.message }

func settingConflict(err error) error {
	return settingPolicyError{status: http.StatusConflict, message: err.Error()}
}

func settingValidationStatus(err error) int {
	if policyError, ok := err.(settingPolicyError); ok && policyError.status != 0 {
		return policyError.status
	}
	return http.StatusBadRequest
}

var canonicalSettingDefinitions = buildSettingDefinitions()

func buildSettingDefinitions() map[string]settingDefinition {
	definitions := map[string]settingDefinition{}
	register := func(key, domain string) *settingDefinition {
		definition := settingDefinition{Key: key, Domain: domain}
		definitions[key] = definition
		return &definition
	}
	// Keep existing AppSetting names canonical even when they have no custom policy.
	for _, item := range []struct{ key, domain string }{
		{"paths", "assets"}, {"mvforgePreferences", "general"},
		{"storageRoles", "assets"}, {"workspace", "assets"}, {"assetInventory", "assets"},
		{"assetTypes", "assets"}, {"assetCategories", "assets"}, {"trackProfilePathAssignments", "assets"},
		{"workers", "pipeline"}, {"pipelineAutomation", "pipeline"}, {"validation", "pipeline"},
		{"cancellationPolicy", "operations"}, {"housekeeping", "operations"}, {testEncodePolicySettingKey, "test-encode"},
		{"originalRetentionPolicy", "archive"},
		{"subtitleSettings", "pipeline"}, {translationSettingsKey, "pipeline"},
		{jellyfinSettingsKey, "providers-integrations"}, {ollamaProviderSettingsKey, "providers-integrations"},
		{"runtimePolicy", "runtime"}, {"remoteExecutors", "runtime"}, {airesource.SettingsKey, "runtime"},
		{"directPlay", "pipeline"}, {"workingHours", "pipeline"},
		{analysisPolicySettingKey, "analysis"}, {profileSampleEstimatePolicySettingKey, "analysis"},
		{assetDefaultsSettingKey, "asset-defaults"}, {"audioEnhancementProfiles", "assets"}, {"trackProfiles", "assets"},
	} {
		register(item.key, item.domain)
	}

	definitions["workers"] = withValidation(definitions["workers"], func(_ settingsMutationContext, value models.JSONMap) error {
		return validateWorkerSettings(value)
	})
	definitions["runtimePolicy"] = withValidation(definitions["runtimePolicy"], func(_ settingsMutationContext, value models.JSONMap) error {
		return runtimeinfo.ValidateRuntimePolicyValue(value)
	})
	definitions[airesource.SettingsKey] = withNormalizer(definitions[airesource.SettingsKey], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return airesource.NormalizeSettingsValue(value)
	})
	definitions[testEncodePolicySettingKey] = withValidation(definitions[testEncodePolicySettingKey], func(_ settingsMutationContext, value models.JSONMap) error {
		return validateTestEncodePolicyValue(value)
	})
	definitions[testEncodePolicySettingKey] = withNormalizer(definitions[testEncodePolicySettingKey], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return normalizeTestEncodePolicyValue(value)
	})
	definitions[analysisPolicySettingKey] = withValidation(definitions[analysisPolicySettingKey], func(_ settingsMutationContext, value models.JSONMap) error {
		return validateAnalysisPolicy(value)
	})
	definitions[analysisPolicySettingKey] = withNormalizer(definitions[analysisPolicySettingKey], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return normalizedAnalysisPolicyValue(value), nil
	})
	definitions[profileSampleEstimatePolicySettingKey] = withNormalizer(definitions[profileSampleEstimatePolicySettingKey], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return profileSampleEstimatePolicyValue(value)
	})
	definitions["mvforgePreferences"] = withValidation(definitions["mvforgePreferences"], func(_ settingsMutationContext, value models.JSONMap) error {
		return validateMVForgePreferences(value)
	})
	definitions["mvforgePreferences"] = withNormalizer(definitions["mvforgePreferences"], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return normalizeMVForgePreferencesForStorage(value), nil
	})
	definitions[translationSettingsKey] = withNormalizer(definitions[translationSettingsKey], func(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return translationSettingsForUpdate(ctx.db, value)
	})
	definitions[translationSettingsKey] = withProjector(definitions[translationSettingsKey], func(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return translationSettingsProjection(ctx.db, value)
	})
	definitions[ollamaProviderSettingsKey] = withNormalizer(definitions[ollamaProviderSettingsKey], func(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return ollamaProviderSettingsForUpdate(ctx.db, value)
	})
	definitions[ollamaProviderSettingsKey] = withProjector(definitions[ollamaProviderSettingsKey], func(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return ollamaProviderSettingsProjection(ctx.db, value)
	})
	definitions["subtitleSettings"] = withNormalizer(definitions["subtitleSettings"], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return normalizeSubtitleSettingsForUpdate(value)
	})
	definitions["validation"] = withNormalizer(definitions["validation"], normalizeValidationPolicySetting)
	definitions["validation"] = withProjector(definitions["validation"], projectValidationPolicySetting)
	definitions["originalRetentionPolicy"] = withValidation(definitions["originalRetentionPolicy"], func(_ settingsMutationContext, value models.JSONMap) error {
		return validateOriginalRetentionPolicy(value)
	})
	definitions["trackProfiles"] = withValidation(definitions["trackProfiles"], func(ctx settingsMutationContext, value models.JSONMap) error {
		if err := validateTrackProfileDomainPolicies(value); err != nil {
			return err
		}
		if err := validateSettingProfileAssignments(ctx.db, "tracks", value); err != nil {
			return settingConflict(err)
		}
		return nil
	})
	definitions["audioEnhancementProfiles"] = withValidation(definitions["audioEnhancementProfiles"], func(ctx settingsMutationContext, value models.JSONMap) error {
		if err := validateSettingProfileAssignments(ctx.db, "audio", value); err != nil {
			return settingConflict(err)
		}
		return nil
	})
	definitions["trackProfiles"] = withNormalizer(definitions["trackProfiles"], func(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return normalizeTrackProfilesForStorage(value), nil
	})
	definitions[assetDefaultsSettingKey] = withNormalizer(definitions[assetDefaultsSettingKey], func(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return validateAssetDefaultsValue(ctx.db, value)
	})
	definitions["jellyfin"] = withNormalizer(definitions["jellyfin"], func(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
		return jellyfinSettingsForUpdateWithSecrets(ctx.db, value, ctx.secrets)
	})
	definitions["jellyfin"] = withProjector(definitions["jellyfin"], func(ctx settingsMutationContext, _ models.JSONMap) (models.JSONMap, error) {
		settings, _, err := loadJellyfinSettingsCheckedWithSecrets(ctx.db, ctx.secrets)
		if err != nil {
			return nil, err
		}
		return jellyfinSettingsProjection(settings), nil
	})
	definitions["pipelineAutomation"] = withEffect(definitions["pipelineAutomation"], func(ctx settingsMutationContext) error {
		return ctx.db.Model(&models.ExecutionPlan{}).
			Where("status = ? AND waiting_state = ? AND approval_status = ?", scheduler.ExecutionPlanWaiting, "WAITING_REVIEW", scheduler.ApprovalPending).
			Updates(map[string]any{"status": scheduler.ExecutionPlanPendingEvaluation, "waiting_state": ""}).Error
	})
	definitions["directPlay"] = withEffect(definitions["directPlay"], func(ctx settingsMutationContext) error {
		if err := ctx.db.Model(&models.ExecutionPlan{}).
			Where("status = ? AND waiting_state = ?", scheduler.ExecutionPlanWaiting, scheduler.WaitingDirectPlayReview).
			Updates(map[string]any{"status": scheduler.ExecutionPlanPendingEvaluation, "waiting_state": ""}).Error; err != nil {
			return err
		}
		return ctx.db.Model(&models.QueueJob{}).
			Where("status = ? AND published_at IS NULL", JobStatusCompleted).
			Updates(map[string]any{"validation_status": ValidationStatusPending, "validation_score": 0, "validation_report": models.JSONMap{}}).Error
	})
	definitions["runtimePolicy"] = withEffect(definitions["runtimePolicy"], func(ctx settingsMutationContext) error {
		_, _, err := runtimeinfo.DetectAndUpsert(ctx.db)
		return err
	})

	return definitions
}

func withValidation(definition settingDefinition, validate func(settingsMutationContext, models.JSONMap) error) settingDefinition {
	definition.Validate = validate
	return definition
}

func withNormalizer(definition settingDefinition, normalize func(settingsMutationContext, models.JSONMap) (models.JSONMap, error)) settingDefinition {
	definition.Normalize = normalize
	return definition
}

func withProjector(definition settingDefinition, project func(settingsMutationContext, models.JSONMap) (models.JSONMap, error)) settingDefinition {
	definition.Project = project
	return definition
}

func withEffect(definition settingDefinition, effect func(settingsMutationContext) error) settingDefinition {
	definition.AfterSave = effect
	return definition
}

func settingDefinitionFor(key string) settingDefinition {
	if definition, ok := canonicalSettingDefinitions[key]; ok {
		return definition
	}
	return settingDefinition{Key: key, Domain: "legacy"}
}

func knownSettingDefinitions() []settingDefinition {
	definitions := make([]settingDefinition, 0, len(canonicalSettingDefinitions))
	for _, definition := range canonicalSettingDefinitions {
		definitions = append(definitions, definition)
	}
	return definitions
}
