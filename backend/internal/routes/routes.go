package routes

import (
	"net/http"

	"github.com/anuelvs/mediaforge/backend/internal/handlers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func New(db *gorm.DB) *gin.Engine {
	router := gin.Default()
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
		api.POST("/assets/review", assets.UpdateReview)
		api.POST("/assets/metadata", assets.UpdateMetadata)
		api.POST("/assets/conversion", assets.UpdateConversion)
		api.GET("/assets/preview", assets.Preview)
		api.GET("/assets/preview/compatible", assets.CompatiblePreview)
		api.GET("/assets/preview/audio", assets.AudioPreview)
		api.POST("/analysis/backfill-as-is", handlers.BackfillAnalysisFromAsIsReports(db))
		api.GET("/paths/browse", paths.Browse)
		api.POST("/advisor/evaluate", advisor.Evaluate)
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
		api.GET("/settings", settings.List)
		api.POST("/settings/:key", settings.Update)
		api.POST("/import/mwp", imports.ImportMWP)
		api.GET("/system/versions", handlers.Versions)
		api.GET("/system/runtime", runtime.Latest)
		api.POST("/system/runtime/refresh", runtime.Refresh)
		api.GET("/system/scheduler-recovery", recovery.Latest)
		api.POST("/system/scheduler-recovery/run", recovery.Run)
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
