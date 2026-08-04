package routes

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/handlers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func New(db *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(productionLogger(), productionRecovery())
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
	assets := handlers.NewAssetHandler(db)
	advisor := handlers.NewAdvisorHandler(db)
	paths := handlers.NewPathHandler(db)
	profiles := handlers.NewProfileHandler(db)
	publisher := handlers.NewPublisherHandler(db)
	queue := handlers.NewQueueHandler(db)
	executionPlans := handlers.NewExecutionPlanHandler(db)
	scanner := handlers.NewScannerHandler(db)
	settings := handlers.NewSettingsHandler(db)
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
		api.POST("/assets/sync", assets.Sync)
		api.POST("/assets/recover", assets.Recover)
		api.POST("/assets/delete-converted", assets.DeleteConverted)
		api.POST("/assets/return-published-as-is", assets.ReturnPublishedAsIs)
		api.POST("/assets/extract-subtitles", assets.ExtractSubtitles)
		api.GET("/assets/external-subtitles", assets.ExternalSubtitles)
		api.GET("/assets/external-subtitles/content", assets.ExternalSubtitleContent)
		api.PUT("/assets/external-subtitles", assets.UpdateExternalSubtitle)
		api.DELETE("/assets/external-subtitles", assets.DeleteExternalSubtitle)
		api.POST("/assets/migrate-path", assets.MigratePath)
		api.POST("/assets/publish-as-is", assets.PublishAsIs)
		api.POST("/assets/reconcile-publication", assets.ConfirmPublicationReconciliation)
		api.POST("/assets/review", assets.UpdateReview)
		api.POST("/assets/metadata", assets.UpdateMetadata)
		api.POST("/assets/conversion", assets.UpdateConversion)
		api.GET("/assets/preview", assets.Preview)
		api.GET("/assets/preview/compatible", assets.CompatiblePreview)
		api.POST("/assets/preview/estimate", assets.SampleEstimate)
		api.POST("/assets/quality-recommendation", assets.QualityRecommendation)
		api.GET("/assets/preview/inspect", assets.CompatiblePreviewInspection)
		api.GET("/assets/preview/metrics", assets.CompatiblePreviewMetrics)
		api.GET("/assets/preview/audio", assets.AudioPreview)
		api.POST("/analysis/backfill-as-is", handlers.BackfillAnalysisFromAsIsReports(db))
		api.GET("/paths/browse", paths.Browse)
		api.POST("/advisor/evaluate", advisor.Evaluate)
		api.POST("/advisor/suggest", advisor.Suggest)
		api.GET("/profiles", profiles.List)
		api.POST("/profiles", profiles.Create)
		api.POST("/profiles/:id", profiles.Update)
		api.POST("/profiles/:id/disabled", profiles.SetDisabled)
		api.DELETE("/profiles/:id", profiles.Delete)
		api.POST("/publisher/jobs/:id/publish", publisher.PublishJob)
		api.GET("/queue/jobs", queue.List)
		api.GET("/queue/jobs/:id/artifacts", handlers.JobArtifacts(db))
		api.GET("/queue/jobs/:id/execution-plans", executionPlans.ListForJob)
		api.POST("/queue/jobs/:id/execution-plans/:planId/approve", executionPlans.Approve)
		api.POST("/queue/jobs/:id/execution-plans/:planId/reject", executionPlans.Reject)
		api.POST("/queue/jobs", queue.Create)
		api.POST("/queue/jobs/:id", queue.Update)
		api.DELETE("/queue/jobs/:id", queue.Dismiss)
		api.DELETE("/queue/batches/:batchId", queue.DismissBatch)
		api.GET("/settings", settings.List)
		api.POST("/settings/:key", settings.Update)
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
		api.POST("/scan", scanner.Scan)
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
