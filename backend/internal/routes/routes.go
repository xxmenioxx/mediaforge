package routes

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/handlers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func New(db *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(stripInboundCookies(), productionLogger(), productionRecovery())
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		},
		AllowMethods:  []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Range"},
		ExposeHeaders: []string{"Accept-Ranges", "Content-Length", "Content-Range", "Content-Type"},
	}))

	libraries := handlers.NewLibraryHandler(db)
	logs := handlers.NewLogHandler(db)
	resourceAdmission := airesource.NewService(db)
	ollamaHTTPClient := handlers.NewOllamaProviderHTTPClient()
	aiModelLifecycle := handlers.NewAIModelLifecycle(db, resourceAdmission, ollamaHTTPClient)
	assets := handlers.NewAssetHandlerWithLifecycle(db, aiModelLifecycle)
	advisor := handlers.NewAdvisorHandler(db)
	paths := handlers.NewPathHandler(db)
	profiles := handlers.NewProfileHandler(db)
	profileAssignments := handlers.NewProfileAssignmentHandler(db)
	assetConfiguration := handlers.NewAssetConfigurationHandler(db)
	publisher := handlers.NewPublisherHandler(db)
	queue := handlers.NewQueueHandler(db)
	executionPlans := handlers.NewExecutionPlanHandler(db)
	scanner := handlers.NewScannerHandler(db)
	settings := handlers.NewSettingsHandlerWithLifecycle(db, resourceAdmission, aiModelLifecycle)
	jellyfin := handlers.NewJellyfinHandler(db)
	validation := handlers.NewValidationHandler(db)
	imports := handlers.NewImportHandler(db)
	workers := handlers.NewWorkerHandler(db)
	runtime := handlers.NewRuntimeHandler(db)
	recovery := handlers.NewSchedulerRecoveryHandler(db)
	housekeeping := handlers.NewHousekeepingHandler(db)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})
	router.GET("/health", handlers.Health)
	router.GET("/openapi.json", handlers.OpenAPI)
	router.GET("/swagger", handlers.SwaggerUI)
	router.GET("/swagger/index.html", handlers.SwaggerUI)

	api := router.Group("/api")
	{
		api.GET("/libraries", libraries.List)
		api.POST("/libraries", libraries.Create)
		api.POST("/libraries/:id", libraries.Update)
		api.GET("/logs/files", logs.ListFiles)
		api.GET("/logs/files/:name", logs.ReadFile)
		api.GET("/assets", assets.List)
		api.GET("/assets/:assetId/thumbnail", assets.Thumbnail)
		api.GET("/source-groups", assetConfiguration.ListSourceGroups)
		api.POST("/source-groups/:id", assetConfiguration.UpdateSourceGroup)
		api.GET("/asset-scope-configurations", assetConfiguration.ListScopeConfigurations)
		api.POST("/asset-scope-configurations", assetConfiguration.UpsertScopeConfiguration)
		api.POST("/asset-scope-configurations/logical-groups/batch", assetConfiguration.ConfigureLogicalGroupsBatch)
		api.GET("/assets/effective-configuration", assetConfiguration.Effective)
		api.POST("/assets/effective-configurations", assetConfiguration.EffectiveBatch)
		api.POST("/assets/sync", assets.Sync)
		api.DELETE("/assets/missing", assets.RemoveMissing)
		api.DELETE("/assets/missing/all", assets.RemoveAllMissing)
		api.POST("/assets/recover", assets.Recover)
		api.DELETE("/assets/archive", assets.DeleteArchive)
		api.DELETE("/assets/archive/batch", assets.DeleteArchiveBatch)
		api.POST("/assets/delete-converted", assets.DeleteConverted)
		api.POST("/assets/return-published-as-is", assets.ReturnPublishedAsIs)
		api.POST("/assets/extract-subtitles", assets.ExtractSubtitles)
		api.GET("/assets/extract-subtitles", assets.ListSubtitleExtractionOperations)
		api.GET("/assets/extract-subtitles/:id", assets.GetSubtitleExtractionOperation)
		api.POST("/assets/extract-subtitles/:id/cancel", assets.CancelSubtitleExtractionOperation)
		api.POST("/assets/subtitle-restyle", assets.RestyleSubtitle)
		api.POST("/assets/subtitle-compose", assets.ComposeSubtitles)
		api.POST("/assets/subtitle-ocr/analyze", assets.AnalyzeSubtitleOCR)
		api.POST("/assets/subtitle-ocr/correct", assets.CorrectSubtitleOCR)
		api.POST("/assets/subtitle-ocr/reevaluate/preview", assets.PreviewOCRDictionaryReevaluation)
		api.POST("/assets/subtitle-ocr/reevaluate/apply", assets.ApplyOCRDictionaryReevaluation)
		api.GET("/assets/ocr-dictionary", assets.ListOCRDictionary)
		api.POST("/assets/ocr-dictionary", assets.CreateOCRDictionaryEntry)
		api.POST("/assets/ocr-dictionary/test-match", assets.TestOCRDictionaryRule)
		api.PUT("/assets/ocr-dictionary/:id", assets.UpdateOCRDictionaryEntry)
		api.PATCH("/assets/ocr-dictionary/:id/enabled", assets.SetOCRDictionaryEntryEnabled)
		api.DELETE("/assets/ocr-dictionary/:id", assets.DeleteOCRDictionaryEntry)
		api.POST("/assets/subtitle-translation/source", assets.ResolveSubtitleTranslationSource)
		api.POST("/assets/subtitle-translation/capability", assets.SubtitleTranslationCapability)
		api.POST("/assets/subtitle-translation", assets.StartSubtitleTranslation)
		api.GET("/assets/subtitle-operations", assets.ListSubtitleExtractionOperations)
		api.GET("/assets/subtitle-operations/:id", assets.GetSubtitleExtractionOperation)
		api.POST("/assets/subtitle-operations/:id/cancel", assets.CancelSubtitleExtractionOperation)
		api.POST("/assets/subtitle-operations/:id/retry", assets.RetrySubtitleOperation)
		api.GET("/assets/external-subtitles", assets.ExternalSubtitles)
		api.GET("/assets/metadata-sidecars", assets.MetadataSidecars)
		api.GET("/assets/subtitle-eligibility", assets.SubtitleEligibility)
		api.GET("/assets/external-subtitles/content", assets.ExternalSubtitleContent)
		api.PUT("/assets/external-subtitles", assets.UpdateExternalSubtitle)
		api.POST("/assets/external-subtitles/rename", assets.RenameExternalSubtitle)
		api.DELETE("/assets/external-subtitles", assets.DeleteExternalSubtitle)
		api.POST("/assets/library-rename/preview", assets.PreviewLibraryPathRename)
		api.POST("/assets/library-rename/apply", assets.ApplyLibraryPathRename)
		api.POST("/assets/migrate-path", assets.MigratePath)
		api.POST("/assets/publish-as-is", assets.PublishAsIs)
		api.POST("/assets/accept-as-is", assets.AcceptAsIs)
		api.POST("/assets/reconsider", assets.Reconsider)
		api.POST("/assets/reconcile-publication", assets.ConfirmPublicationReconciliation)
		api.POST("/assets/review", assets.UpdateReview)
		api.POST("/assets/edit", assets.Edit)
		api.POST("/assets/metadata", assets.UpdateMetadata)
		api.POST("/assets/rename", assets.Rename)
		api.POST("/assets/conversion", assets.UpdateConversion)
		api.PATCH("/assets/tracks", assets.UpdateTracks)
		api.GET("/assets/track-maintenance/inventory", assets.TrackMaintenanceInventory)
		api.POST("/assets/track-maintenance/remove", assets.StartTrackRemoval)
		api.POST("/assets/track-maintenance/edit", assets.StartTrackEdit)
		api.POST("/assets/track-maintenance/add-aac", assets.StartAddAACTrack)
		api.GET("/assets/track-maintenance/operations/:id", assets.GetMaintenanceOperation)
		api.GET("/test-encodes", assets.ListTestEncodes)
		api.GET("/test-encodes/summary", assets.TestEncodeInventorySummary)
		api.GET("/test-encodes/orphans", assets.ListOrphanedTestEncodeArtifacts)
		api.DELETE("/test-encodes/orphans/:artifactId", assets.DeleteOrphanedTestEncodeArtifact)
		api.POST("/test-encodes/delete-preview", assets.DeleteTestEncodePreview)
		api.POST("/test-encodes/delete", assets.DeleteSelectedTestEncodes)
		api.POST("/test-encodes/reconcile", assets.ReconcileTestEncodes)
		api.POST("/test-encodes", assets.CreateTestEncode)
		api.GET("/test-encodes/:id", assets.GetTestEncode)
		api.POST("/test-encodes/:id/profile-qualification/evaluate", assets.EvaluateTestEncodeProfileQualification)
		api.POST("/test-encodes/:id/cancel", assets.CancelTestEncode)
		api.POST("/test-encodes/:id/keep", assets.KeepTestEncode)
		api.DELETE("/test-encodes/:id", assets.DeleteTestEncode)
		api.GET("/assets/preview", assets.Preview)
		api.GET("/assets/preview/compatible", assets.CompatiblePreview)
		api.POST("/assets/preview/compatible/requests", assets.CreateCompatiblePreviewRequest)
		api.POST("/assets/preview/compatible/requests/:requestId/start", assets.CompatiblePreviewRequestStart)
		api.GET("/assets/preview/compatible/requests/:requestId/status", assets.CompatiblePreviewRequestStatus)
		api.POST("/assets/preview/compatible/requests/:requestId/cancel", assets.CompatiblePreviewRequestCancel)
		api.GET("/assets/preview/compatible/requests/:requestId/video", assets.CompatiblePreviewRequestVideo)
		api.GET("/assets/preview/compatible/requests/:requestId/frame", assets.CompatiblePreviewRequestFrame)
		api.GET("/assets/preview/compatible/requests/:requestId/inspect", assets.CompatiblePreviewRequestInspection)
		api.GET("/assets/preview/compatible/requests/:requestId/metrics", assets.CompatiblePreviewRequestMetrics)
		api.POST("/assets/preview/estimate", assets.SampleEstimate)
		api.POST("/assets/preview/estimate/operations", assets.StartProfileSampleEstimateOperation)
		api.GET("/assets/preview/estimate/operations/:id", assets.GetProfileSampleEstimateOperation)
		api.POST("/assets/preview/estimate/operations/:id/cancel", assets.CancelProfileSampleEstimateOperation)
		api.POST("/lab/fidelity/operations", assets.StartLabFidelityOperation)
		api.GET("/lab/fidelity/operations/:id", assets.GetLabFidelityOperation)
		api.POST("/lab/fidelity/operations/:id/cancel", assets.CancelLabFidelityOperation)
		api.POST("/assets/quality-recommendation", assets.QualityRecommendation)
		api.GET("/assets/preview/inspect", assets.CompatiblePreviewInspection)
		api.GET("/assets/preview/metrics", assets.CompatiblePreviewMetrics)
		api.GET("/assets/preview/audio", assets.AudioPreview)
		api.POST("/remote-executors/probe", assets.ProbeRemoteExecutor)
		api.POST("/remote-executors/test-conversion", assets.TestRemoteExecutorConversion)
		api.POST("/analysis/backfill-as-is", handlers.BackfillAnalysisFromAsIsReports(db))
		api.GET("/paths/browse", paths.Browse)
		api.POST("/advisor/evaluate", advisor.Evaluate)
		api.POST("/advisor/suggest", advisor.Suggest)
		api.GET("/profiles", profiles.List)
		api.GET("/profile-templates", profiles.ListTemplates)
		api.POST("/profile-templates/:key/clone", profiles.CloneTemplate)
		api.POST("/profiles", profiles.Create)
		api.POST("/profiles/:id", profiles.Update)
		api.POST("/profiles/:id/disabled", profiles.SetDisabled)
		api.DELETE("/profiles/:id", profiles.Delete)
		api.GET("/profile-assignments", profileAssignments.List)
		api.POST("/profile-assignments", profileAssignments.Upsert)
		api.POST("/track-profiles/resolve-preview", queue.PreviewTrackProfileResolution)
		api.GET("/publisher/publications", publisher.ListPublications)
		api.GET("/publisher/eligibility", publisher.ListManualEligibility)
		api.POST("/publisher/publications/:publicationId/actions", publisher.PublisherAction)
		api.POST("/publisher/jobs/:id/publish", publisher.PublishJob)
		api.DELETE("/publisher/jobs/:id", publisher.DiscardJob)
		api.GET("/queue/jobs", queue.List)
		api.GET("/queue/jobs/:id/log", logs.ReadJobLog)
		api.GET("/queue/jobs/:id/artifacts", handlers.JobArtifacts(db))
		api.GET("/queue/jobs/:id/execution-plans", executionPlans.ListForJob)
		api.POST("/queue/jobs/:id/execution-plans/:planId/approve", executionPlans.Approve)
		api.POST("/queue/jobs/:id/execution-plans/:planId/reject", executionPlans.Reject)
		api.POST("/queue/jobs", queue.Create)
		api.POST("/queue/preflights", queue.CreateBatchPreflight)
		api.POST("/queue/preflights/selected-assets", queue.PreflightSelectedAssets)
		api.GET("/queue/preflights/:id", queue.GetBatchPreflight)
		api.PUT("/queue/preflights/:id", queue.UpdateBatchPreflight)
		api.POST("/queue/preflights/:id/edit", queue.EditBatchPreflight)
		api.POST("/queue/preflights/:id/accept", queue.AcceptBatchPreflight)
		api.POST("/queue/preflights/:id/remove", queue.RemoveBatchPreflightAsset)
		api.POST("/queue/preflights/:id/discard", queue.DiscardBatchPreflight)
		api.POST("/queue/batches", queue.RequireBatchPreflight)
		api.POST("/queue/selected-assets", queue.QueueSelectedAssets)
		api.POST("/assets/queue-remaining/preview", queue.QueueRemainingPreview)
		api.POST("/assets/queue-remaining/commit", queue.QueueRemainingCommit)
		api.POST("/queue/jobs/reorder", queue.Reorder)
		api.POST("/queue/jobs/:id", queue.Update)
		api.DELETE("/queue/jobs/:id", queue.Dismiss)
		api.DELETE("/queue/batches/:batchId", queue.DismissBatch)
		api.GET("/settings", settings.List)
		api.GET("/settings/ai-resource-protection/status", settings.AIResourceProtectionStatus)
		api.POST("/settings/ai-resource-protection/preview", settings.PreviewAIResourceAdmission)
		api.POST("/settings/batch", settings.BatchUpdate)
		api.POST("/settings/:key", settings.Update)
		api.POST("/settings/ollama/test-connection", settings.TestOllamaConnection)
		api.GET("/settings/ollama/models", settings.ListOllamaModels)
		api.POST("/settings/ollama/model-readiness", settings.TestOllamaModelReadiness)
		api.GET("/settings/ollama/runtime", settings.OllamaRuntimeStatus)
		api.POST("/settings/jellyfin/test-connection", settings.TestJellyfinConnection)
		api.GET("/settings/jellyfin/state", settings.JellyfinIntegrationState)
		api.GET("/jellyfin/outcomes", jellyfin.Outcomes)
		api.POST("/jellyfin/manual-refresh", jellyfin.ManualRefresh)
		api.POST("/jellyfin/outcomes/:id/retry", jellyfin.Retry)
		api.POST("/settings/subtitle-translation/test-connection", settings.TestTranslationConnection)
		api.POST("/settings/subtitle-translation/enable", settings.EnableTranslation)
		api.POST("/import/mwp", imports.ImportMWP)
		api.GET("/system/versions", handlers.Versions)
		api.GET("/system/runtime", runtime.Latest)
		api.GET("/system/runtime/profiles", runtime.Profiles)
		api.POST("/system/runtime/refresh", runtime.Refresh)
		api.GET("/system/scheduler-recovery", recovery.Latest)
		api.POST("/system/scheduler-recovery/run", recovery.Run)
		api.GET("/system/housekeeping/preview", housekeeping.Preview)
		api.POST("/system/housekeeping/run", housekeeping.Run)
		api.POST("/workers/claim", workers.ClaimNext)
		api.GET("/workers/nodes", workers.ListNodes)
		api.POST("/workers/jobs/:id/dry-run", workers.DryRun)
		api.POST("/workers/jobs/:id/execute", workers.Execute)
		api.POST("/workers/jobs/:id/status", workers.UpdateJobStatus)
		api.POST("/validation/jobs/:id", validation.ValidateJob)
		api.POST("/validation/jobs/:id/staging-cleanup/preview", validation.PreviewStagingCleanup)
		api.POST("/validation/jobs/:id/staging-cleanup", validation.ExecuteStagingCleanup)
		api.GET("/validation/jobs/:id/reconciliation-preview", validation.PreviewHistoricalTrackReconciliation)
		api.GET("/validation/review", validation.ReviewList)
		api.GET("/validation/history", validation.HistoryList)
		api.GET("/validation/jobs/:id/runs/:runId", validation.RunDetail)
		api.POST("/validation/jobs/:id/overrides", validation.CreateOverride)
		api.POST("/scan", scanner.Scan)
		api.GET("/scan", scanner.LatestSnapshot)
		api.POST("/scan/operations", scanner.StartSnapshotOperation)
		api.GET("/scan/operations", scanner.ListSnapshotOperations)
		api.GET("/scan/operations/:id", scanner.GetSnapshotOperation)
		api.POST("/scan/operations/:id/cancel", scanner.CancelSnapshotOperation)
	}

	return router
}

func productionLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			var value [12]byte
			if _, err := rand.Read(value[:]); err == nil {
				requestID = hex.EncodeToString(value[:])
			} else {
				requestID = time.Now().UTC().Format("20060102150405.000000000")
			}
		}
		c.Set("requestId", requestID)
		c.Header("X-Request-ID", requestID)

		ctx := c.Request.Context()
		if ctx.Err() != nil {
			return
		}

		c.Next()

		level := "info"
		if c.Writer.Status() >= 500 {
			level = "error"
		} else if c.Writer.Status() >= 400 {
			level = "warn"
		}
		applog.Event(level, "http", "request_completed", map[string]any{
			"requestId":  requestID,
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"query":      c.Request.URL.RawQuery,
			"status":     c.Writer.Status(),
			"durationMs": time.Since(started).Milliseconds(),
			"clientIp":   c.ClientIP(),
			"errors":     c.Errors.Errors(),
		}, nil)
	}
}

func productionRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID, _ := c.Get("requestId")
				applog.Event("error", "http", "panic_recovered", map[string]any{
					"requestId": requestID,
					"method":    c.Request.Method,
					"path":      c.Request.URL.Path,
					"panic":     recovered,
					"stack":     string(debug.Stack()),
				}, nil)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error", "requestId": requestID})
			}
		}()
		c.Next()
	}
}
