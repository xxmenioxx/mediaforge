package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func OpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"openapi": "3.0.3",
		"info": gin.H{
			"title":       "MVForge API",
			"version":     "1.0.13",
			"description": "Manual media workflow orchestration API.",
		},
		"servers": []gin.H{
			{"url": "http://localhost:8080"},
		},
		"tags":       openAPITags(),
		"paths":      openAPIPaths(),
		"components": openAPIComponents(),
	})
}

func SwaggerUI(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, swaggerHTML)
}

func openAPITags() []gin.H {
	return []gin.H{
		{
			"name":        "System",
			"description": "Health checks, runtime information, and software versions.",
		},
		{
			"name":        "Libraries",
			"description": "Source and destination library configuration.",
		},
		{
			"name":        "Paths",
			"description": "Safe browsing for Docker-mounted MVForge folders.",
		},
		{
			"name":        "Assets",
			"description": "Filesystem inventory, grouped assets, and browser preview streams.",
		},
		{
			"name":        "Test Encodes",
			"description": "Short real-pipeline encodes for physical playback testing, independent from Queue and Publisher.",
		},
		{
			"name":        "Scanner",
			"description": "Media probing, snapshots, streams, codecs, tracks, and metadata.",
		},
		{
			"name":        "Advisor",
			"description": "Deterministic conversion recommendations before queueing work.",
		},
		{
			"name":        "Profiles",
			"description": "Conversion profiles and worker-ready encoding presets.",
		},
		{
			"name":        "Queue",
			"description": "Manual queue creation, individual jobs, and folder batch metadata.",
		},
		{
			"name":        "Workers",
			"description": "Worker lifecycle, claiming jobs, status updates, and dry-run execution.",
		},
		{
			"name":        "Validation",
			"description": "Post-conversion output checks before publishing.",
		},
		{
			"name":        "Publisher",
			"description": "Controlled publishing of validated outputs to destination libraries.",
		},
		{
			"name":        "Logs",
			"description": "Diagnostic log files for jobs, workers, validation, and publishing.",
		},
		{
			"name":        "Settings",
			"description": "Persisted application settings, including paths, workers, validation, and cancellation policy.",
		},
		{
			"name":        "Imports",
			"description": "One-off import and migration helpers.",
		},
	}
}

func openAPIPaths() gin.H {
	return gin.H{
		"/health": gin.H{
			"get": gin.H{
				"tags":        []string{"System"},
				"summary":     "Check API health",
				"operationId": "getHealth",
				"responses": gin.H{
					"200": gin.H{
						"description": "API is healthy",
					},
				},
			},
		},
		"/api/libraries": gin.H{
			"get": gin.H{
				"tags":        []string{"Libraries"},
				"summary":     "List libraries",
				"operationId": "listLibraries",
				"responses": gin.H{
					"200": jsonResponse("Libraries", gin.H{
						"type":  "array",
						"items": ref("Library"),
					}),
				},
			},
			"post": gin.H{
				"tags":        []string{"Libraries"},
				"summary":     "Create a library",
				"operationId": "createLibrary",
				"requestBody": requestBody(ref("LibraryInput")),
				"responses": gin.H{
					"201": jsonResponse("Created library", ref("Library")),
					"400": jsonResponse("Invalid request", ref("Error")),
				},
			},
		},
		"/api/libraries/{id}": gin.H{
			"post": gin.H{
				"tags":        []string{"Libraries"},
				"summary":     "Update a library",
				"operationId": "updateLibrary",
				"parameters": []gin.H{
					pathIDParameter(),
				},
				"requestBody": requestBody(ref("LibraryInput")),
				"responses": gin.H{
					"200": jsonResponse("Updated library", ref("Library")),
					"400": jsonResponse("Invalid request", ref("Error")),
					"404": jsonResponse("Library not found", ref("Error")),
				},
			},
		},
		"/api/paths/browse": gin.H{
			"get": gin.H{
				"tags":        []string{"Paths"},
				"summary":     "Browse folders under a configured MVForge root",
				"operationId": "browsePaths",
				"parameters": []gin.H{
					{
						"name":        "root",
						"in":          "query",
						"description": "Mounted root to browse.",
						"required":    false,
						"schema": gin.H{
							"type":    "string",
							"enum":    []string{"raw", "library", "staging"},
							"default": "raw",
						},
					},
				},
				"responses": gin.H{
					"200": jsonResponse("Browsable folders", ref("PathBrowseResponse")),
					"400": jsonResponse("Unsupported root", ref("Error")),
				},
			},
		},
		"/api/assets": gin.H{
			"get": gin.H{
				"tags":        []string{"Assets"},
				"summary":     "List library assets",
				"operationId": "listAssets",
				"responses": gin.H{
					"200": jsonResponse("Asset inventory", ref("AssetInventory")),
				},
			},
		},
		"/api/assets/effective-configurations": gin.H{
			"post": gin.H{
				"tags":        []string{"Assets"},
				"summary":     "Resolve effective configuration for multiple assets",
				"operationId": "resolveEffectiveAssetConfigurations",
				"requestBody": requestBody(gin.H{
					"type": "object", "required": []string{"assetIds"},
					"properties": gin.H{"assetIds": gin.H{"type": "array", "items": gin.H{"type": "integer"}, "minItems": 1, "maxItems": 1000}},
				}),
				"responses": gin.H{
					"200": jsonResponse("Effective configurations keyed by asset ID", gin.H{"type": "object"}),
					"400": jsonResponse("Invalid asset IDs", ref("Error")),
				},
			},
		},
		"/api/asset-scope-configurations/logical-groups/batch": gin.H{
			"post": gin.H{
				"tags":        []string{"Assets"},
				"summary":     "Transactionally update selected LogicalGroup configuration dimensions",
				"operationId": "configureLogicalGroupsBatch",
				"requestBody": requestBody(ref("ConfigureLogicalGroupsBatchInput")),
				"responses": gin.H{
					"200": jsonResponse("Updated LogicalGroup scopes", ref("ConfigureLogicalGroupsBatchResponse")),
					"400": jsonResponse("Invalid batch configuration", ref("Error")),
				},
			},
		},
		"/api/assets/track-maintenance/inventory": gin.H{
			"get": gin.H{"tags": []string{"Assets"}, "summary": "Inspect embedded tracks and maintenance availability", "operationId": "trackMaintenanceInventory", "parameters": []gin.H{{"name": "path", "in": "query", "required": true, "schema": gin.H{"type": "string"}}}, "responses": gin.H{"200": jsonResponse("Track inventory", gin.H{"type": "object"})}},
		},
		"/api/assets/track-maintenance/remove": gin.H{
			"post": gin.H{"tags": []string{"Assets"}, "summary": "Safely remove selected MKV tracks", "operationId": "removeAssetTracks", "requestBody": requestBody(gin.H{"type": "object"}), "responses": gin.H{"202": jsonResponse("Maintenance accepted", gin.H{"type": "object"})}},
		},
		"/api/assets/track-maintenance/edit": gin.H{
			"post": gin.H{"tags": []string{"Assets"}, "summary": "Edit MKV track metadata and dispositions", "operationId": "editAssetTrack", "requestBody": requestBody(gin.H{"type": "object"}), "responses": gin.H{"202": jsonResponse("Maintenance accepted", gin.H{"type": "object"})}},
		},
		"/api/assets/track-maintenance/add-aac": gin.H{
			"post": gin.H{"tags": []string{"Assets"}, "summary": "Create an additional AAC audio track", "operationId": "addAssetAACTrack", "requestBody": requestBody(gin.H{"type": "object"}), "responses": gin.H{"202": jsonResponse("Maintenance accepted", gin.H{"type": "object"})}},
		},
		"/api/test-encodes": gin.H{
			"get": gin.H{
				"tags": []string{"Test Encodes"}, "summary": "List Test Encodes", "operationId": "listTestEncodes",
				"parameters": []gin.H{{"name": "sourcePath", "in": "query", "required": false, "schema": gin.H{"type": "string"}}},
				"responses":  gin.H{"200": jsonResponse("Test Encodes", gin.H{"type": "array", "items": ref("TestEncode")})},
			},
			"post": gin.H{
				"tags": []string{"Test Encodes"}, "summary": "Generate a new Test Encode", "operationId": "createTestEncode",
				"requestBody": requestBody(ref("TestEncodeInput")),
				"responses":   gin.H{"202": jsonResponse("Test Encode accepted", ref("TestEncode")), "422": jsonResponse("Configuration cannot be resolved", ref("Error"))},
			},
		},
		"/api/test-encodes/{id}": gin.H{
			"get":    gin.H{"tags": []string{"Test Encodes"}, "summary": "Get Test Encode details", "operationId": "getTestEncode", "parameters": []gin.H{pathIDParameter()}, "responses": gin.H{"200": jsonResponse("Test Encode", ref("TestEncode")), "404": jsonResponse("Not found", ref("Error"))}},
			"delete": gin.H{"tags": []string{"Test Encodes"}, "summary": "Delete a completed Test Encode and its registered files", "operationId": "deleteTestEncode", "parameters": []gin.H{pathIDParameter()}, "responses": gin.H{"200": jsonResponse("Deleted", gin.H{"type": "object"}), "409": jsonResponse("Active Test Encode", ref("Error"))}},
		},
		"/api/test-encodes/{id}/cancel": gin.H{
			"post": gin.H{"tags": []string{"Test Encodes"}, "summary": "Cancel an active Test Encode", "operationId": "cancelTestEncode", "parameters": []gin.H{pathIDParameter()}, "responses": gin.H{"202": jsonResponse("Cancellation accepted", gin.H{"type": "object"}), "409": jsonResponse("Test Encode is not active", ref("Error"))}},
		},
		"/api/test-encodes/{id}/keep": gin.H{
			"post": gin.H{"tags": []string{"Test Encodes"}, "summary": "Enable or disable automatic expiration", "operationId": "keepTestEncode", "parameters": []gin.H{pathIDParameter()}, "requestBody": requestBody(gin.H{"type": "object", "required": []string{"keep"}, "properties": gin.H{"keep": gin.H{"type": "boolean"}}}), "responses": gin.H{"200": jsonResponse("Updated Test Encode", ref("TestEncode"))}},
		},
		"/api/assets/preview": gin.H{
			"get": gin.H{
				"tags":        []string{"Assets"},
				"summary":     "Stream an asset for browser preview",
				"operationId": "previewAsset",
				"parameters": []gin.H{
					{
						"name":     "path",
						"in":       "query",
						"required": true,
						"schema":   gin.H{"type": "string"},
					},
				},
				"responses": gin.H{
					"200": gin.H{"description": "Media stream"},
					"206": gin.H{"description": "Partial media stream"},
					"400": jsonResponse("Invalid request", ref("Error")),
					"403": jsonResponse("Path outside configured libraries", ref("Error")),
					"404": jsonResponse("Media not found", ref("Error")),
				},
			},
		},
		"/api/assets/preview/compatible": gin.H{
			"get": gin.H{
				"tags":        []string{"Assets"},
				"summary":     "Stream a browser-compatible transcoded asset preview",
				"operationId": "previewCompatibleAsset",
				"parameters": []gin.H{
					{
						"name":     "path",
						"in":       "query",
						"required": true,
						"schema":   gin.H{"type": "string"},
					},
				},
				"responses": gin.H{
					"200": gin.H{"description": "Transcoded MP4 preview stream"},
					"400": jsonResponse("Invalid request", ref("Error")),
					"403": jsonResponse("Path outside configured libraries", ref("Error")),
					"404": jsonResponse("Media not found", ref("Error")),
				},
			},
		},
		"/api/assets/quality-recommendation": gin.H{
			"post": gin.H{
				"tags":        []string{"Assets", "Profiles"},
				"summary":     "Translate a quality intent using active worker capabilities",
				"operationId": "recommendEncoderQuality",
				"requestBody": requestBody(ref("QualityRecommendationInput")),
				"responses": gin.H{
					"200": jsonResponse("Effective encoder recommendation", ref("QualityRecommendationResponse")),
					"400": jsonResponse("Unsupported encoder or invalid draft", ref("Error")),
					"403": jsonResponse("Path outside configured libraries", ref("Error")),
				},
			},
		},
		"/api/assets/preview/audio": gin.H{
			"get": gin.H{
				"tags":        []string{"Assets"},
				"summary":     "Stream a short audio preview with an optional audio profile",
				"operationId": "previewAudioProfileSample",
				"parameters": []gin.H{
					{
						"name":     "path",
						"in":       "query",
						"required": true,
						"schema":   gin.H{"type": "string"},
					},
					{
						"name":   "profileKey",
						"in":     "query",
						"schema": gin.H{"type": "string"},
					},
					{
						"name":        "filters",
						"in":          "query",
						"description": "Optional ad-hoc FFmpeg audio filter chain for unsaved profile previews.",
						"schema":      gin.H{"type": "string"},
					},
					{
						"name":        "start",
						"in":          "query",
						"description": "Start time as HH:MM:SS or seconds.",
						"schema":      gin.H{"type": "string", "default": "00:00:00"},
					},
					{
						"name":   "seconds",
						"in":     "query",
						"schema": gin.H{"type": "integer", "default": 20, "minimum": 5, "maximum": 120},
					},
				},
				"responses": gin.H{
					"200": gin.H{"description": "Transcoded audio preview stream"},
					"400": jsonResponse("Invalid request", ref("Error")),
					"403": jsonResponse("Path outside configured libraries", ref("Error")),
					"404": jsonResponse("Media not found", ref("Error")),
				},
			},
		},
		"/api/advisor/evaluate": gin.H{
			"post": gin.H{
				"tags":        []string{"Advisor"},
				"summary":     "Evaluate whether a conversion is worth doing",
				"operationId": "evaluateAdvisor",
				"requestBody": requestBody(ref("AdvisorRequest")),
				"responses": gin.H{
					"200": jsonResponse("Advisor recommendation", ref("AdvisorResponse")),
					"400": jsonResponse("Invalid request", ref("Error")),
					"404": jsonResponse("Profile not found", ref("Error")),
				},
			},
		},
		"/api/advisor/suggest": gin.H{
			"post": gin.H{
				"tags":        []string{"Advisor"},
				"summary":     "Suggest profiles and individually applicable findings for an asset",
				"operationId": "suggestProfile",
				"requestBody": requestBody(ref("ProfileSuggestionRequest")),
				"responses": gin.H{
					"200": jsonResponse("Profile suggestion", ref("ProfileSuggestion")),
					"400": jsonResponse("Invalid request", ref("Error")),
					"422": jsonResponse("Asset could not be analyzed", ref("Error")),
				},
			},
		},
		"/api/profiles": gin.H{
			"get": gin.H{
				"tags":        []string{"Profiles"},
				"summary":     "List conversion profiles",
				"operationId": "listProfiles",
				"responses": gin.H{
					"200": jsonResponse("Profiles", gin.H{
						"type":  "array",
						"items": ref("Profile"),
					}),
				},
			},
			"post": gin.H{
				"tags":        []string{"Profiles"},
				"summary":     "Create a conversion profile",
				"operationId": "createProfile",
				"requestBody": requestBody(ref("ProfileInput")),
				"responses": gin.H{
					"201": jsonResponse("Created profile", ref("Profile")),
					"400": jsonResponse("Invalid request", ref("Error")),
				},
			},
		},
		"/api/profiles/{id}": gin.H{
			"post": gin.H{
				"tags":        []string{"Profiles"},
				"summary":     "Update a conversion profile",
				"operationId": "updateProfile",
				"parameters": []gin.H{
					pathIDParameter(),
				},
				"requestBody": requestBody(ref("ProfileInput")),
				"responses": gin.H{
					"200": jsonResponse("Updated profile", ref("Profile")),
					"400": jsonResponse("Invalid request", ref("Error")),
					"404": jsonResponse("Profile not found", ref("Error")),
				},
			},
		},
		"/api/profile-assignments": gin.H{
			"get": gin.H{
				"tags": []string{"Profiles"}, "summary": "List asset and path profile assignments", "operationId": "listProfileAssignments",
				"responses": gin.H{"200": jsonResponse("Profile assignments", gin.H{"type": "array", "items": ref("ProfileAssignment")})},
			},
			"post": gin.H{
				"tags": []string{"Profiles"}, "summary": "Assign, disable, or inherit one profile dimension", "operationId": "updateProfileAssignment",
				"requestBody": requestBody(ref("ProfileAssignmentInput")),
				"responses":   gin.H{"200": jsonResponse("Updated assignment", ref("ProfileAssignment")), "400": jsonResponse("Invalid assignment", ref("Error"))},
			},
		},
		"/api/queue/jobs": gin.H{
			"get": gin.H{
				"tags":        []string{"Queue"},
				"summary":     "List queue jobs",
				"operationId": "listQueueJobs",
				"responses": gin.H{
					"200": jsonResponse("Queue jobs", gin.H{
						"type":  "array",
						"items": ref("QueueJob"),
					}),
				},
			},
			"post": gin.H{
				"tags":        []string{"Queue"},
				"summary":     "Create a queue job",
				"operationId": "createQueueJob",
				"requestBody": requestBody(ref("QueueJobInput")),
				"responses": gin.H{
					"201": jsonResponse("Created queue job", ref("QueueJob")),
					"400": jsonResponse("Invalid request", ref("Error")),
				},
			},
		},
		"/api/queue/selected-assets": gin.H{
			"post": gin.H{
				"tags":        []string{"Queue"},
				"summary":     "Plan or queue selected assets by authoritative Asset IDs",
				"operationId": "queueSelectedAssets",
				"requestBody": requestBody(ref("QueueSelectedAssetsInput")),
				"responses": gin.H{
					"200": jsonResponse("Authoritative per-asset Queue results", ref("QueueSelectedAssetsResponse")),
					"400": jsonResponse("Invalid selection", ref("Error")),
					"500": jsonResponse("Queue planning failed", ref("Error")),
				},
			},
		},
		"/api/queue/jobs/{id}": gin.H{
			"post": gin.H{
				"tags":        []string{"Queue"},
				"summary":     "Update queue job controls",
				"operationId": "updateQueueJob",
				"parameters": []gin.H{
					pathIDParameter(),
				},
				"requestBody": requestBody(ref("QueueJobUpdateInput")),
				"responses": gin.H{
					"200": jsonResponse("Updated queue job", ref("QueueJob")),
					"400": jsonResponse("Invalid request", ref("Error")),
					"404": jsonResponse("Job not found", ref("Error")),
				},
			},
		},
		"/api/queue/jobs/{id}/execution-plans": gin.H{
			"get": gin.H{
				"tags":        []string{"Queue"},
				"summary":     "List versioned execution plans for a queue job",
				"operationId": "listExecutionPlans",
				"parameters":  []gin.H{pathIDParameter()},
				"responses": gin.H{
					"200": jsonResponse("Execution plans", gin.H{"type": "array", "items": ref("ExecutionPlan")}),
					"404": jsonResponse("Job not found", ref("Error")),
				},
			},
		},
		"/api/validation/jobs/{id}": gin.H{
			"post": gin.H{
				"tags":        []string{"Validation"},
				"summary":     "Validate a completed job output",
				"operationId": "validateJobOutput",
				"parameters": []gin.H{
					pathIDParameter(),
				},
				"responses": gin.H{
					"200": jsonResponse("Validation result", ref("ValidationResult")),
					"404": jsonResponse("Job not found", ref("Error")),
				},
			},
		},
		"/api/publisher/jobs/{id}/publish": gin.H{
			"post": gin.H{
				"tags":        []string{"Publisher"},
				"summary":     "Publish a validated job output",
				"operationId": "publishJobOutput",
				"parameters": []gin.H{
					pathIDParameter(),
				},
				"requestBody": requestBody(gin.H{
					"type": "object",
					"properties": gin.H{
						"overwrite": gin.H{"type": "boolean", "example": false},
					},
				}),
				"responses": gin.H{
					"200": jsonResponse("Publish result", ref("PublishResult")),
					"400": jsonResponse("Invalid publish request", ref("Error")),
					"404": jsonResponse("Job, library, profile, or output not found", ref("Error")),
					"409": jsonResponse("Destination exists", ref("Error")),
				},
			},
		},
		"/api/logs/files": gin.H{
			"get": gin.H{
				"tags":        []string{"Logs"},
				"summary":     "List diagnostic log files",
				"operationId": "listLogFiles",
				"responses": gin.H{
					"200": jsonResponse("Log files", gin.H{
						"type":  "array",
						"items": ref("LogFile"),
					}),
				},
			},
		},
		"/api/logs/files/{name}": gin.H{
			"get": gin.H{
				"tags":        []string{"Logs"},
				"summary":     "Read a diagnostic log file",
				"operationId": "readLogFile",
				"parameters": []gin.H{
					{
						"name":     "name",
						"in":       "path",
						"required": true,
						"schema":   gin.H{"type": "string", "example": "jobs.log"},
					},
				},
				"responses": gin.H{
					"200": jsonResponse("Log file content", ref("LogFileContent")),
					"400": jsonResponse("Invalid log file name", ref("Error")),
					"404": jsonResponse("Log file not found", ref("Error")),
				},
			},
		},
		"/api/settings": gin.H{
			"get": gin.H{
				"tags":        []string{"Settings"},
				"summary":     "List app settings",
				"operationId": "listSettings",
				"responses": gin.H{
					"200": jsonResponse("Settings", gin.H{
						"type":  "array",
						"items": ref("AppSetting"),
					}),
				},
			},
		},
		"/api/settings/{key}": gin.H{
			"post": gin.H{
				"tags":        []string{"Settings"},
				"summary":     "Update an app setting",
				"operationId": "updateSetting",
				"parameters": []gin.H{
					{
						"name":     "key",
						"in":       "path",
						"required": true,
						"schema":   gin.H{"type": "string"},
					},
				},
				"requestBody": requestBody(ref("SettingInput")),
				"responses": gin.H{
					"200": jsonResponse("Updated setting", ref("AppSetting")),
					"400": jsonResponse("Invalid request", ref("Error")),
				},
			},
		},
		"/api/import/mwp": gin.H{
			"post": gin.H{
				"tags":        []string{"Imports"},
				"summary":     "Import Media Worker Pipeline profiles, library types, and settings",
				"operationId": "importMWP",
				"responses": gin.H{
					"200": jsonResponse("MWP import summary", gin.H{
						"type": "object",
						"properties": gin.H{
							"profilesCreated":     gin.H{"type": "integer"},
							"profilesUpdated":     gin.H{"type": "integer"},
							"libraryTypesCreated": gin.H{"type": "integer"},
							"libraryTypesUpdated": gin.H{"type": "integer"},
							"settingsUpdated": gin.H{
								"type":  "array",
								"items": gin.H{"type": "string"},
							},
						},
					}),
				},
			},
		},
		"/api/system/versions": gin.H{
			"get": gin.H{
				"tags":        []string{"System"},
				"summary":     "List software versions used by the app",
				"operationId": "listSoftwareVersions",
				"responses": gin.H{
					"200": jsonResponse("Software versions", ref("SoftwareVersions")),
				},
			},
		},
		"/api/workers/claim": gin.H{
			"post": gin.H{
				"tags":        []string{"Workers"},
				"summary":     "Claim the next queued job",
				"operationId": "claimWorkerJob",
				"requestBody": requestBody(ref("ClaimJobInput")),
				"responses": gin.H{
					"200": jsonResponse("Claimed queue job", ref("QueueJob")),
					"404": jsonResponse("No queued jobs available", ref("Error")),
				},
			},
		},
		"/api/workers/jobs/{id}/status": gin.H{
			"post": gin.H{
				"tags":        []string{"Workers"},
				"summary":     "Update a worker job status",
				"operationId": "updateWorkerJobStatus",
				"parameters": []gin.H{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   gin.H{"type": "integer"},
					},
				},
				"requestBody": requestBody(ref("UpdateJobStatusInput")),
				"responses": gin.H{
					"200": jsonResponse("Updated queue job", ref("QueueJob")),
					"400": jsonResponse("Invalid request", ref("Error")),
					"404": jsonResponse("Job not found", ref("Error")),
				},
			},
		},
		"/api/workers/jobs/{id}/dry-run": gin.H{
			"post": gin.H{
				"tags":        []string{"Workers"},
				"summary":     "Complete a running job with a dry-run conversion plan",
				"operationId": "dryRunWorkerJob",
				"parameters": []gin.H{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   gin.H{"type": "integer"},
					},
				},
				"responses": gin.H{
					"200": jsonResponse("Dry-run completed queue job", ref("QueueJob")),
					"400": jsonResponse("Invalid job state", ref("Error")),
					"404": jsonResponse("Job, library, or profile not found", ref("Error")),
				},
			},
		},
		"/api/workers/jobs/{id}/execute": gin.H{
			"post": gin.H{
				"tags":        []string{"Workers"},
				"summary":     "Execute a running job with FFmpeg into staging",
				"operationId": "executeWorkerJob",
				"parameters": []gin.H{
					{
						"name":     "id",
						"in":       "path",
						"required": true,
						"schema":   gin.H{"type": "integer"},
					},
				},
				"requestBody": requestBody(ref("ExecuteJobInput")),
				"responses": gin.H{
					"200": jsonResponse("Executed queue job", ref("QueueJob")),
					"400": jsonResponse("Invalid job state or unreadable media", ref("Error")),
					"404": jsonResponse("Job, library, or profile not found", ref("Error")),
					"409": jsonResponse("Staging output already exists", ref("Error")),
				},
			},
		},
		"/api/scan": gin.H{
			"post": gin.H{
				"tags":        []string{"Scanner"},
				"summary":     "Scan a media path",
				"operationId": "scanMedia",
				"requestBody": requestBody(ref("ScanRequest")),
				"responses": gin.H{
					"202": jsonResponse("Scan accepted", ref("ScanResult")),
					"400": jsonResponse("Invalid request", ref("Error")),
				},
			},
		},
	}
}

func openAPIComponents() gin.H {
	return gin.H{
		"schemas": gin.H{
			"Error": gin.H{
				"type":     "object",
				"required": []string{"error"},
				"properties": gin.H{
					"error": gin.H{"type": "string"},
				},
			},
			"LogicalGroupConfigurationChange": gin.H{
				"type": "object", "required": []string{"mode"},
				"properties": gin.H{
					"mode":           gin.H{"type": "string", "enum": []string{"no_change", "inherit", "value", "disabled"}},
					"videoProfileId": gin.H{"type": "integer"}, "profileKey": gin.H{"type": "string"},
					"category": gin.H{"type": "string"}, "destinationLibraryId": gin.H{"type": "integer"},
				},
			},
			"ConfigureLogicalGroupsBatchInput": gin.H{
				"type": "object", "required": []string{"logicalGroupPaths", "video", "audio", "tracks", "category", "destination"},
				"properties": gin.H{
					"logicalGroupPaths": gin.H{"type": "array", "minItems": 1, "maxItems": 500, "items": gin.H{"type": "string"}},
					"video":             ref("LogicalGroupConfigurationChange"), "audio": ref("LogicalGroupConfigurationChange"), "tracks": ref("LogicalGroupConfigurationChange"),
					"category": ref("LogicalGroupConfigurationChange"), "destination": ref("LogicalGroupConfigurationChange"),
				},
			},
			"ConfigureLogicalGroupsBatchResponse": gin.H{
				"type": "object", "required": []string{"logicalGroupPaths", "changedDimensions"},
				"properties": gin.H{
					"logicalGroupPaths": gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"changedDimensions": gin.H{"type": "array", "items": gin.H{"type": "string", "enum": []string{"video", "audio", "tracks", "category", "destination"}}},
				},
			},
			"TestEncodeInput": gin.H{
				"type": "object", "required": []string{"sourcePath", "libraryId", "configurationSource", "startMode", "durationSeconds"},
				"properties": gin.H{
					"sourcePath": gin.H{"type": "string"}, "libraryId": gin.H{"type": "integer"},
					"configurationSource": gin.H{"type": "string", "enum": []string{"effective_asset", "lab_draft"}},
					"profileId":           gin.H{"type": "integer"}, "audioProfileKey": gin.H{"type": "string"}, "trackProfileKey": gin.H{"type": "string"},
					"processingMode": gin.H{"type": "string", "enum": []string{"full_encode", "audio_only"}}, "resolveAssignments": gin.H{"type": "boolean"},
					"labProfile": gin.H{"type": "object", "additionalProperties": true}, "labAudioProfile": gin.H{"type": "object", "additionalProperties": true}, "labTrackOverride": gin.H{"type": "object", "additionalProperties": true},
					"startMode": gin.H{"type": "string", "enum": []string{"representative", "beginning", "middle", "custom"}}, "startSeconds": gin.H{"type": "number"}, "durationSeconds": gin.H{"type": "integer", "minimum": 5, "maximum": 120},
				},
			},
			"TestEncode": gin.H{
				"type": "object", "required": []string{"id", "sourcePath", "configurationSource", "status", "durationSeconds", "keep", "stale"},
				"properties": gin.H{
					"id": gin.H{"type": "integer"}, "sourceAssetId": gin.H{"type": "integer"}, "sourcePath": gin.H{"type": "string"}, "libraryId": gin.H{"type": "integer"},
					"configurationSource": gin.H{"type": "string"}, "requestedConfiguration": gin.H{"type": "object", "additionalProperties": true}, "effectiveConfiguration": gin.H{"type": "object", "additionalProperties": true}, "configurationHash": gin.H{"type": "string"},
					"startSeconds": gin.H{"type": "number"}, "durationSeconds": gin.H{"type": "integer"}, "status": gin.H{"type": "string"}, "phase": gin.H{"type": "string"}, "progress": gin.H{"type": "integer"},
					"workerName": gin.H{"type": "string"}, "effectiveEncoder": gin.H{"type": "string"}, "ffmpegCommand": gin.H{"type": "string"}, "outputPath": gin.H{"type": "string"}, "outputSizeBytes": gin.H{"type": "integer", "format": "int64"},
					"subtitleArtifacts": gin.H{"type": "array", "items": gin.H{"type": "object"}}, "validationReport": gin.H{"type": "object", "additionalProperties": true},
					"keep": gin.H{"type": "boolean"}, "expiresAt": gin.H{"type": "string", "format": "date-time", "nullable": true}, "stale": gin.H{"type": "boolean"}, "staleReason": gin.H{"type": "string"}, "errorMessage": gin.H{"type": "string"},
					"createdAt": gin.H{"type": "string", "format": "date-time"}, "completedAt": gin.H{"type": "string", "format": "date-time", "nullable": true},
				},
			},
			"Library": mergeSchema(ref("LibraryInput"), gin.H{
				"id":        gin.H{"type": "integer", "example": 1},
				"createdAt": gin.H{"type": "string", "format": "date-time"},
				"updatedAt": gin.H{"type": "string", "format": "date-time"},
			}),
			"LibraryInput": gin.H{
				"type":     "object",
				"required": []string{"name", "destinationPath", "type"},
				"properties": gin.H{
					"name":            gin.H{"type": "string", "example": "Movies"},
					"sourcePath":      gin.H{"type": "string", "example": "/media/raw"},
					"destinationPath": gin.H{"type": "string", "example": "/media/library/movies"},
					"type":            gin.H{"type": "string", "example": "movies"},
					"validationRules": gin.H{
						"type":                 "object",
						"additionalProperties": true,
						"properties": gin.H{
							"episodeNamingEnabled": gin.H{"type": "boolean", "example": false},
							"extrasPathEnabled":    gin.H{"type": "boolean", "example": false},
							"extrasPathName":       gin.H{"type": "string", "example": "extras"},
						},
					},
					"defaultProfileId": gin.H{
						"type":     "integer",
						"nullable": true,
						"example":  1,
					},
				},
			},
			"PathBrowseResponse": gin.H{
				"type":     "object",
				"required": []string{"root", "rootKey", "paths"},
				"properties": gin.H{
					"root":    gin.H{"type": "string", "example": "/media/raw"},
					"rootKey": gin.H{"type": "string", "enum": []string{"raw", "library", "staging"}},
					"paths": gin.H{
						"type":  "array",
						"items": ref("PathEntry"),
					},
				},
			},
			"PathEntry": gin.H{
				"type":     "object",
				"required": []string{"path", "relativePath", "name"},
				"properties": gin.H{
					"path":         gin.H{"type": "string", "example": "/media/raw/movies"},
					"relativePath": gin.H{"type": "string", "example": "movies"},
					"name":         gin.H{"type": "string", "example": "movies"},
				},
			},
			"AssetInventory": gin.H{
				"type":     "object",
				"required": []string{"unprocessed", "library", "converted", "unverified", "accepted", "unprocessedGroups", "libraryGroups", "convertedGroups", "acceptedGroups"},
				"properties": gin.H{
					"unprocessed": gin.H{
						"type":  "array",
						"items": ref("Asset"),
					},
					"converted": gin.H{
						"type":  "array",
						"items": ref("Asset"),
					},
					"library": gin.H{
						"type":  "array",
						"items": ref("Asset"),
					},
					"unverified": gin.H{
						"type":  "array",
						"items": ref("Asset"),
					},
					"accepted": gin.H{
						"type":  "array",
						"items": ref("Asset"),
					},
					"unprocessedGroups": gin.H{
						"type":  "array",
						"items": ref("AssetGroup"),
					},
					"convertedGroups": gin.H{
						"type":  "array",
						"items": ref("AssetGroup"),
					},
					"libraryGroups": gin.H{
						"type":  "array",
						"items": ref("AssetGroup"),
					},
					"acceptedGroups": gin.H{
						"type":  "array",
						"items": ref("AssetGroup"),
					},
				},
			},
			"Asset": gin.H{
				"type": "object",
				"properties": gin.H{
					"libraryId":    gin.H{"type": "integer", "example": 1},
					"libraryName":  gin.H{"type": "string", "example": "Movies"},
					"path":         gin.H{"type": "string", "example": "/media/raw/movies/movie.mkv"},
					"relativePath": gin.H{"type": "string", "example": "The Matrix/title00.mkv"},
					"groupPath":    gin.H{"type": "string", "example": "The Matrix"},
					"fileName":     gin.H{"type": "string", "example": "movie.mkv"},
					"extension":    gin.H{"type": "string", "example": ".mkv"},
					"sizeBytes":    gin.H{"type": "integer", "format": "int64", "example": 7340032000},
					"modifiedAt":   gin.H{"type": "string", "format": "date-time"},
					"status":       gin.H{"type": "string", "enum": []string{"unprocessed", "unverified", "accepted", "converted", "archive"}},
				},
			},
			"AssetGroup": gin.H{
				"type": "object",
				"properties": gin.H{
					"id":           gin.H{"type": "string", "example": "unprocessed:1:The Matrix"},
					"libraryId":    gin.H{"type": "integer", "example": 1},
					"libraryName":  gin.H{"type": "string", "example": "Movies"},
					"path":         gin.H{"type": "string", "example": "/media/raw/movies/The Matrix"},
					"relativePath": gin.H{"type": "string", "example": "The Matrix"},
					"status":       gin.H{"type": "string", "enum": []string{"unprocessed", "unverified", "accepted", "converted", "archive"}},
					"fileCount":    gin.H{"type": "integer", "example": 3},
					"sizeBytes":    gin.H{"type": "integer", "format": "int64", "example": 7340032000},
					"modifiedAt":   gin.H{"type": "string", "format": "date-time"},
					"assets": gin.H{
						"type":  "array",
						"items": ref("Asset"),
					},
				},
			},
			"AdvisorRequest": gin.H{
				"type":     "object",
				"required": []string{"mediaPath", "profileId"},
				"properties": gin.H{
					"mediaPath": gin.H{"type": "string", "example": "/media/raw/movies/movie.mkv"},
					"profileId": gin.H{"type": "integer", "example": 2},
				},
			},
			"ProfileSuggestionRequest": gin.H{
				"type":     "object",
				"required": []string{"mediaPath"},
				"properties": gin.H{
					"mediaPath": gin.H{"type": "string", "example": "/media/raw/anime/series/episode.mkv"},
				},
			},
			"AdvisorFinding": gin.H{
				"type": "object",
				"properties": gin.H{
					"id":              gin.H{"type": "string"},
					"category":        gin.H{"type": "string"},
					"title":           gin.H{"type": "string"},
					"detail":          gin.H{"type": "string"},
					"severity":        gin.H{"type": "string", "enum": []string{"information", "recommended", "review", "unsafe"}},
					"confidence":      gin.H{"type": "string", "enum": []string{"low", "medium", "high"}},
					"actionable":      gin.H{"type": "boolean"},
					"defaultSelected": gin.H{"type": "boolean"},
					"patch":           gin.H{"type": "object", "additionalProperties": true},
					"evidence":        gin.H{"type": "array", "items": gin.H{"type": "string"}},
				},
			},
			"ProfileCandidate": gin.H{
				"type": "object",
				"properties": gin.H{
					"profile": ref("Profile"),
					"score":   gin.H{"type": "integer"},
					"reasons": gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"source":  gin.H{"type": "string", "enum": []string{"assigned_asset", "assigned_path"}},
				},
			},
			"ProfileSuggestion": gin.H{
				"type": "object",
				"properties": gin.H{
					"matchType":        gin.H{"type": "string", "enum": []string{"create", "existing", "assigned_asset", "assigned_path"}},
					"summary":          gin.H{"type": "string"},
					"scan":             ref("ScanResult"),
					"suggestedProfile": ref("Profile"),
					"candidates":       gin.H{"type": "array", "items": ref("ProfileCandidate")},
					"proposedProfile":  ref("ProfileInput"),
					"insights":         ref("AnalysisInsights"),
					"findings":         gin.H{"type": "array", "items": ref("AdvisorFinding")},
				},
			},
			"AnalysisInsights": gin.H{
				"type": "object",
				"properties": gin.H{
					"recommendedCrf":       gin.H{"type": "integer"},
					"estimatedMinBytes":    gin.H{"type": "integer", "format": "int64"},
					"estimatedMaxBytes":    gin.H{"type": "integer", "format": "int64"},
					"estimatedSavingsLow":  gin.H{"type": "integer"},
					"estimatedSavingsHigh": gin.H{"type": "integer"},
					"recommendations":      gin.H{"type": "array", "items": gin.H{"type": "string"}},
				},
			},
			"AdvisorResponse": gin.H{
				"type": "object",
				"properties": gin.H{
					"recommendation": gin.H{"type": "string", "enum": []string{"worth_it", "maybe", "not_recommended"}},
					"score":          gin.H{"type": "integer", "example": 72},
					"summary":        gin.H{"type": "string"},
					"reasons":        gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"warnings":       gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"estimated":      ref("AdvisorEstimation"),
					"scan":           ref("ScanResult"),
					"profile":        ref("Profile"),
				},
			},
			"AdvisorEstimation": gin.H{
				"type": "object",
				"properties": gin.H{
					"currentSizeBytes": gin.H{"type": "integer", "format": "int64"},
					"targetCodec":      gin.H{"type": "string", "example": "x265"},
					"targetContainer":  gin.H{"type": "string", "example": "mkv"},
				},
			},
			"Profile": mergeSchema(ref("ProfileInput"), gin.H{
				"id":        gin.H{"type": "integer", "example": 1},
				"createdAt": gin.H{"type": "string", "format": "date-time"},
				"updatedAt": gin.H{"type": "string", "format": "date-time"},
			}),
			"QualityRecommendationInput": gin.H{
				"type":     "object",
				"required": []string{"profile"},
				"properties": gin.H{
					"path":    gin.H{"type": "string", "description": "Optional readable asset used for adaptive calculations."},
					"profile": ref("ProfileInput"),
				},
			},
			"EncoderRecommendation": gin.H{
				"type": "object",
				"properties": gin.H{
					"encoder":              gin.H{"type": "string", "enum": []string{"hevc_qsv", "hevc_videotoolbox"}},
					"requestedRateControl": gin.H{"type": "string"},
					"effectiveRateControl": gin.H{"type": "string"},
					"rateControlFallback":  gin.H{"type": "string"},
					"targetBitrate":        gin.H{"type": "integer", "format": "int64"},
					"globalQuality":        gin.H{"type": "integer"},
					"profile":              gin.H{"type": "string"},
					"pixelFormat":          gin.H{"type": "string"},
					"estimateConfidence":   gin.H{"type": "string", "enum": []string{"low", "medium", "high"}},
					"warnings":             gin.H{"type": "array", "items": gin.H{"type": "string"}},
				},
			},
			"QualityRecommendationResponse": gin.H{
				"type": "object",
				"properties": gin.H{
					"requestedProfile":         ref("Profile"),
					"effectiveProfile":         ref("Profile"),
					"recommendation":           ref("EncoderRecommendation"),
					"capabilitySource":         gin.H{"type": "string", "enum": []string{"active_runtime_snapshot", "live_backend_probe"}},
					"ffmpegVideoArguments":     gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"estimatedOutputMinBytes":  gin.H{"type": "integer", "format": "int64"},
					"estimatedOutputMaxBytes":  gin.H{"type": "integer", "format": "int64"},
					"estimatedSavingsMinBytes": gin.H{"type": "integer", "format": "int64"},
					"estimatedSavingsMaxBytes": gin.H{"type": "integer", "format": "int64"},
				},
			},
			"ProfileInput": gin.H{
				"type": "object",
				"required": []string{
					"name",
					"container",
					"videoCodec",
					"audioCodec",
					"qualityMode",
					"qualityValue",
				},
				"properties": gin.H{
					"name":               gin.H{"type": "string", "example": "Blu-ray Archive"},
					"scope":              gin.H{"type": "string", "enum": []string{"asset", "path"}, "default": "asset"},
					"description":        gin.H{"type": "string", "example": "HEVC archive profile"},
					"container":          gin.H{"type": "string", "example": "mkv"},
					"videoCodec":         gin.H{"type": "string", "example": "x265"},
					"audioCodec":         gin.H{"type": "string", "example": "copy"},
					"qualityMode":        gin.H{"type": "string", "example": "crf"},
					"qualityValue":       gin.H{"type": "integer", "example": 20},
					"optimizationIntent": gin.H{"type": "string", "enum": []string{"maximum_savings", "balanced", "conservative", "maximum_quality", "archive"}},
					"preserveHdr":        gin.H{"type": "boolean", "example": true},
					"preserveSubtitles":  gin.H{"type": "boolean", "example": true},
					"preserveChapters":   gin.H{"type": "boolean", "example": true},
					"workerConfig":       gin.H{"type": "object", "additionalProperties": true},
				},
			},
			"QueueJob": mergeSchema(ref("QueueJobInput"), gin.H{
				"id":           gin.H{"type": "integer", "example": 1},
				"batchId":      gin.H{"type": "string", "example": "batch-1-1710000000000-ab12cd"},
				"batchName":    gin.H{"type": "string", "example": "Movies/The Matrix"},
				"status":       gin.H{"type": "string", "enum": []string{"queued", "running", "completed", "failed", "canceled"}},
				"progress":     gin.H{"type": "integer", "example": 45},
				"workerName":   gin.H{"type": "string", "example": "local-worker"},
				"outputPath":   gin.H{"type": "string", "example": "/media/library/movie.mkv"},
				"errorMessage": gin.H{"type": "string"},
				"validationStatus": gin.H{
					"type": "string",
					"enum": []string{"", "pending", "passed", "warning", "failed"},
				},
				"validationScore":      gin.H{"type": "integer", "example": 100},
				"validationReport":     gin.H{"type": "object", "additionalProperties": true},
				"audioProfileSnapshot": gin.H{"type": "object", "additionalProperties": true},
				"trackProfileSnapshot": gin.H{"type": "object", "additionalProperties": true},
				"profileResolution":    gin.H{"type": "object", "additionalProperties": true},
				"publishedPath":        gin.H{"type": "string", "example": "/media/library/movie.mkv"},
				"publishedAt":          gin.H{"type": "string", "format": "date-time", "nullable": true},
				"startedAt":            gin.H{"type": "string", "format": "date-time", "nullable": true},
				"finishedAt":           gin.H{"type": "string", "format": "date-time", "nullable": true},
				"createdAt":            gin.H{"type": "string", "format": "date-time"},
				"updatedAt":            gin.H{"type": "string", "format": "date-time"},
			}),
			"QueueJobInput": gin.H{
				"type":     "object",
				"required": []string{"mediaPath", "libraryId", "profileId"},
				"properties": gin.H{
					"mediaPath":                 gin.H{"type": "string", "example": "/media/raw/movie.mkv"},
					"publishMode":               gin.H{"type": "string", "enum": []string{"standard", "replace_library_asset"}, "default": "standard"},
					"batchId":                   gin.H{"type": "string", "example": "batch-1-1710000000000-ab12cd"},
					"batchName":                 gin.H{"type": "string", "example": "Movies/The Matrix"},
					"libraryId":                 gin.H{"type": "integer", "example": 1},
					"profileId":                 gin.H{"type": "integer", "example": 2},
					"audioProfileKey":           gin.H{"type": "string", "example": "dialogue-clarity"},
					"trackProfileKey":           gin.H{"type": "string", "example": "japanese-spanish"},
					"resolveProfileAssignments": gin.H{"type": "boolean", "description": "Resolve asset-over-path assignments and freeze their snapshots before queueing."},
					"priority":                  gin.H{"type": "integer", "example": 5},
					"notes":                     gin.H{"type": "string", "example": "Manual test job"},
				},
			},
			"QueueSelectedAssetsInput": gin.H{
				"type": "object", "required": []string{"assetIds"},
				"properties": gin.H{
					"assetIds": gin.H{"type": "array", "minItems": 1, "maxItems": 1000, "items": gin.H{"type": "integer"}},
					"commit":   gin.H{"type": "boolean"},
				},
			},
			"QueueSelectedAssetResult": gin.H{
				"type": "object", "required": []string{"assetId", "outcome"},
				"properties": gin.H{
					"assetId": gin.H{"type": "integer"},
					"outcome": gin.H{"type": "string", "enum": []string{"eligible", "queued", "skipped", "failed"}},
					"reason":  gin.H{"type": "string", "enum": []string{"not_found", "missing", "needs_review", "already_queued", "active_maintenance", "invalid_configuration", "reservation_conflict", "queue_creation_failed"}},
					"message": gin.H{"type": "string"}, "batchId": gin.H{"type": "string"}, "batchName": gin.H{"type": "string"}, "jobId": gin.H{"type": "integer"},
				},
			},
			"QueueSelectedAssetsResponse": gin.H{
				"type": "object", "required": []string{"summary", "results", "batches"},
				"properties": gin.H{
					"summary": gin.H{"type": "object", "properties": gin.H{
						"selected": gin.H{"type": "integer"}, "eligible": gin.H{"type": "integer"}, "queued": gin.H{"type": "integer"}, "skipped": gin.H{"type": "integer"}, "failed": gin.H{"type": "integer"}, "titleCount": gin.H{"type": "integer"}, "sizeBytes": gin.H{"type": "integer", "format": "int64"},
					}},
					"results": gin.H{"type": "array", "items": ref("QueueSelectedAssetResult")},
					"batches": gin.H{"type": "array", "items": gin.H{"type": "object", "properties": gin.H{"batchId": gin.H{"type": "string"}, "batchName": gin.H{"type": "string"}, "jobCount": gin.H{"type": "integer"}}}},
				},
			},
			"ProfileAssignment": mergeSchema(ref("ProfileAssignmentInput"), gin.H{
				"id": gin.H{"type": "integer"}, "createdAt": gin.H{"type": "string", "format": "date-time"}, "updatedAt": gin.H{"type": "string", "format": "date-time"},
			}),
			"ProfileAssignmentInput": gin.H{
				"type": "object", "required": []string{"targetType", "targetPath", "mediaType", "selection"},
				"properties": gin.H{
					"targetType":     gin.H{"type": "string", "enum": []string{"asset", "path"}},
					"targetPath":     gin.H{"type": "string"},
					"mediaType":      gin.H{"type": "string", "enum": []string{"video", "audio", "tracks"}},
					"selection":      gin.H{"type": "string", "enum": []string{"profile", "disabled", "inherit"}},
					"videoProfileId": gin.H{"type": "integer"},
					"profileKey":     gin.H{"type": "string"},
				},
			},
			"ExecutionPlan": gin.H{
				"type": "object",
				"properties": gin.H{
					"id":              gin.H{"type": "integer"},
					"jobId":           gin.H{"type": "integer"},
					"version":         gin.H{"type": "integer"},
					"status":          gin.H{"type": "string", "enum": []string{"pending_evaluation", "ready", "waiting", "dispatched", "superseded"}},
					"profileVersion":  gin.H{"type": "integer"},
					"constraints":     gin.H{"type": "object", "additionalProperties": true},
					"codecFamily":     gin.H{"type": "string"},
					"selectedEncoder": gin.H{"type": "string"},
					"runtimeProfile":  gin.H{"type": "string"},
					"workspaceMode":   gin.H{"type": "string"},
					"waitingState":    gin.H{"type": "string"},
					"decisionReasons": gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"decisionSources": gin.H{"type": "object", "additionalProperties": true},
					"warnings":        gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"reservation":     gin.H{"type": "object", "additionalProperties": true},
					"evaluation":      gin.H{"type": "object", "additionalProperties": true},
					"supersededAt":    gin.H{"type": "string", "format": "date-time", "nullable": true},
					"createdAt":       gin.H{"type": "string", "format": "date-time"},
					"updatedAt":       gin.H{"type": "string", "format": "date-time"},
				},
			},
			"QueueJobUpdateInput": gin.H{
				"type": "object",
				"properties": gin.H{
					"priority": gin.H{"type": "integer", "minimum": 1, "maximum": 10, "example": 3},
					"status":   gin.H{"type": "string", "enum": []string{"queued", "running", "completed", "failed", "canceled"}},
					"notes":    gin.H{"type": "string", "example": "Manual queue update"},
				},
			},
			"AppSetting": gin.H{
				"type":     "object",
				"required": []string{"key", "value"},
				"properties": gin.H{
					"key":       gin.H{"type": "string", "example": "workers"},
					"value":     gin.H{"type": "object", "additionalProperties": true},
					"createdAt": gin.H{"type": "string", "format": "date-time"},
					"updatedAt": gin.H{"type": "string", "format": "date-time"},
				},
			},
			"SettingInput": gin.H{
				"type":     "object",
				"required": []string{"value"},
				"properties": gin.H{
					"value": gin.H{"type": "object", "additionalProperties": true},
				},
			},
			"SoftwareVersions": gin.H{
				"type":     "object",
				"required": []string{"components"},
				"properties": gin.H{
					"components": gin.H{
						"type":  "array",
						"items": ref("SoftwareComponent"),
					},
				},
			},
			"SoftwareComponent": gin.H{
				"type":     "object",
				"required": []string{"name", "version", "source"},
				"properties": gin.H{
					"name":    gin.H{"type": "string", "example": "FFmpeg"},
					"version": gin.H{"type": "string", "example": "ffmpeg version 8.1.2"},
					"source":  gin.H{"type": "string", "example": "container"},
				},
			},
			"ClaimJobInput": gin.H{
				"type": "object",
				"properties": gin.H{
					"workerName": gin.H{"type": "string", "example": "local-worker"},
				},
			},
			"ExecuteJobInput": gin.H{
				"type": "object",
				"properties": gin.H{
					"overwrite": gin.H{"type": "boolean", "example": false},
				},
			},
			"UpdateJobStatusInput": gin.H{
				"type":     "object",
				"required": []string{"status"},
				"properties": gin.H{
					"status":       gin.H{"type": "string", "enum": []string{"queued", "running", "completed", "failed", "canceled"}},
					"progress":     gin.H{"type": "integer", "example": 45},
					"outputPath":   gin.H{"type": "string", "example": "/media/library/movie.mkv"},
					"errorMessage": gin.H{"type": "string", "example": "Conversion failed"},
				},
			},
			"ValidationResult": gin.H{
				"type": "object",
				"properties": gin.H{
					"jobId":    gin.H{"type": "integer", "example": 12},
					"status":   gin.H{"type": "string", "enum": []string{"passed", "warning", "failed"}},
					"score":    gin.H{"type": "integer", "example": 100},
					"warnings": gin.H{"type": "array", "items": gin.H{"type": "string"}},
					"checks": gin.H{
						"type":  "array",
						"items": ref("ValidationCheck"),
					},
					"report": gin.H{"type": "object", "additionalProperties": true},
				},
			},
			"ValidationCheck": gin.H{
				"type": "object",
				"properties": gin.H{
					"key":     gin.H{"type": "string", "example": "output_exists"},
					"label":   gin.H{"type": "string", "example": "Output file exists"},
					"status":  gin.H{"type": "string", "enum": []string{"passed", "failed"}},
					"message": gin.H{"type": "string"},
				},
			},
			"PublishResult": gin.H{
				"type": "object",
				"properties": gin.H{
					"jobId":         gin.H{"type": "integer", "example": 12},
					"status":        gin.H{"type": "string", "example": "published"},
					"sourcePath":    gin.H{"type": "string", "example": "/media/staging/movie.mkv"},
					"publishedPath": gin.H{"type": "string", "example": "/media/library/movies/movie.mkv"},
					"message":       gin.H{"type": "string"},
				},
			},
			"LogFile": gin.H{
				"type": "object",
				"properties": gin.H{
					"name":       gin.H{"type": "string", "example": "job-1.log"},
					"sizeBytes":  gin.H{"type": "integer", "format": "int64", "example": 2048},
					"modifiedAt": gin.H{"type": "string", "format": "date-time"},
				},
			},
			"LogFileContent": gin.H{
				"type": "object",
				"properties": gin.H{
					"name":      gin.H{"type": "string", "example": "job-1.log"},
					"content":   gin.H{"type": "string"},
					"truncated": gin.H{"type": "boolean"},
				},
			},
			"ScanRequest": gin.H{
				"type":     "object",
				"required": []string{"path"},
				"properties": gin.H{
					"path":  gin.H{"type": "string", "example": "/media/raw/movie.mkv"},
					"force": gin.H{"type": "boolean", "example": false},
				},
			},
			"MediaStream": gin.H{
				"type": "object",
				"properties": gin.H{
					"index":              gin.H{"type": "integer", "example": 1},
					"type":               gin.H{"type": "string", "example": "audio"},
					"codec":              gin.H{"type": "string", "example": "aac"},
					"codecLong":          gin.H{"type": "string", "example": "AAC (Advanced Audio Coding)"},
					"profile":            gin.H{"type": "string", "example": "LC"},
					"language":           gin.H{"type": "string", "example": "eng"},
					"title":              gin.H{"type": "string", "example": "English 5.1"},
					"duration":           gin.H{"type": "number", "format": "double"},
					"bitrate":            gin.H{"type": "integer", "format": "int64"},
					"default":            gin.H{"type": "boolean"},
					"forced":             gin.H{"type": "boolean"},
					"comment":            gin.H{"type": "boolean"},
					"hearingImpaired":    gin.H{"type": "boolean"},
					"width":              gin.H{"type": "integer"},
					"height":             gin.H{"type": "integer"},
					"pixFmt":             gin.H{"type": "string"},
					"colorTransfer":      gin.H{"type": "string"},
					"colorPrimaries":     gin.H{"type": "string"},
					"bitsPerRawSample":   gin.H{"type": "string"},
					"avgFrameRate":       gin.H{"type": "string"},
					"realFrameRate":      gin.H{"type": "string"},
					"sampleAspectRatio":  gin.H{"type": "string"},
					"displayAspectRatio": gin.H{"type": "string"},
					"hdr":                gin.H{"type": "boolean"},
					"fieldOrder":         gin.H{"type": "string"},
					"channels":           gin.H{"type": "integer"},
					"channelLayout":      gin.H{"type": "string"},
					"sampleRate":         gin.H{"type": "integer"},
					"bitDepth":           gin.H{"type": "integer"},
					"level":              gin.H{"type": "integer", "description": "Codec level identifier reported by FFprobe (for example, HEVC Level 5.0 is 150)."},
				},
			},
			"ScanResult": gin.H{
				"type":     "object",
				"required": []string{"id", "path", "fileName", "container", "sizeBytes", "duration", "videoCodec"},
				"properties": gin.H{
					"id":                           gin.H{"type": "integer", "example": 1},
					"path":                         gin.H{"type": "string", "example": "/media/raw/movie.mkv"},
					"fileName":                     gin.H{"type": "string", "example": "movie.mkv"},
					"container":                    gin.H{"type": "string", "example": "matroska,webm"},
					"sizeBytes":                    gin.H{"type": "integer", "format": "int64", "example": 7340032000},
					"duration":                     gin.H{"type": "number", "format": "double", "example": 7240.42},
					"bitrate":                      gin.H{"type": "integer", "format": "int64", "example": 8123456},
					"videoCodec":                   gin.H{"type": "string", "example": "hevc"},
					"width":                        gin.H{"type": "integer", "example": 3840},
					"height":                       gin.H{"type": "integer", "example": 2160},
					"hdr":                          gin.H{"type": "boolean", "example": true},
					"audioTracks":                  gin.H{"type": "integer", "example": 2},
					"subtitleTracks":               gin.H{"type": "integer", "example": 3},
					"chapters":                     gin.H{"type": "integer", "example": 18},
					"videoStreams":                 gin.H{"type": "array", "items": ref("MediaStream")},
					"audioStreams":                 gin.H{"type": "array", "items": ref("MediaStream")},
					"subtitleStreams":              gin.H{"type": "array", "items": ref("MediaStream")},
					"interlaceAnalysis":            gin.H{"type": "object", "additionalProperties": true},
					"cadenceAnalysis":              gin.H{"type": "object", "description": "Presentation cadence, declared versus effective picture rate, confidence, and conservative output recommendation.", "additionalProperties": true},
					"cadenceRecommendation":        gin.H{"type": "object", "description": "Semantic cadence operation and resolved output frame-rate recommendation derived from cadence analysis.", "additionalProperties": true},
					"cropAnalysis":                 gin.H{"type": "object", "additionalProperties": true},
					"frameStructureAnalysis":       gin.H{"type": "object", "additionalProperties": true},
					"frameStructureRecommendation": gin.H{"type": "object", "description": "Encoder-neutral compatible, balanced, and maximum-compression GOP/B-frame recommendations derived from the source snapshot.", "additionalProperties": true},
					"hevcLevelRecommendation":      gin.H{"type": "object", "description": "Minimum HEVC Main Tier Level derived from picture size, sample rate, and available bitrate evidence.", "additionalProperties": true},
					"rawProbe":                     gin.H{"type": "object", "additionalProperties": true},
					"createdAt":                    gin.H{"type": "string", "format": "date-time"},
					"updatedAt":                    gin.H{"type": "string", "format": "date-time"},
				},
			},
		},
	}
}

func requestBody(schema gin.H) gin.H {
	return gin.H{
		"required": true,
		"content": gin.H{
			"application/json": gin.H{
				"schema": schema,
			},
		},
	}
}

func jsonResponse(description string, schema gin.H) gin.H {
	return gin.H{
		"description": description,
		"content": gin.H{
			"application/json": gin.H{
				"schema": schema,
			},
		},
	}
}

func ref(name string) gin.H {
	return gin.H{"$ref": "#/components/schemas/" + name}
}

func pathIDParameter() gin.H {
	return gin.H{
		"name":     "id",
		"in":       "path",
		"required": true,
		"schema":   gin.H{"type": "integer"},
	}
}

func mergeSchema(base gin.H, extraProperties gin.H) gin.H {
	return gin.H{
		"allOf": []gin.H{
			base,
			{
				"type":       "object",
				"properties": extraProperties,
			},
		},
	}
}

const swaggerHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>MVForge API Swagger</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
    <style>
      body { margin: 0; background: #0e1116; }
      .swagger-ui .topbar { display: none; }
    </style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      const tagOrder = [
        "System",
        "Libraries",
        "Paths",
        "Assets",
        "Scanner",
        "Advisor",
        "Profiles",
        "Queue",
        "Workers",
        "Validation",
        "Publisher",
        "Logs",
        "Settings",
        "Imports"
      ];
      window.ui = SwaggerUIBundle({
        url: "/openapi.json",
        dom_id: "#swagger-ui",
        deepLinking: true,
        persistAuthorization: true,
        tryItOutEnabled: true,
        docExpansion: "list",
        defaultModelsExpandDepth: -1,
        tagsSorter: (left, right) => tagOrder.indexOf(left) - tagOrder.indexOf(right),
        operationsSorter: "alpha"
      });
    </script>
  </body>
</html>`
