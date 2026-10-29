package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/service"
)

func RegisterBatchVideoRoutes(api *gin.RouterGroup, svc *service.Service) {
	r := api.Group("/batch-video", func(c *gin.Context) {
		if err := svc.RequireFeature(service.FeatureBatchVideo); err != nil {
			failService(c, err)
			c.Abort()
			return
		}
		c.Next()
	})

	r.POST("/uploads", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		file, err := c.FormFile("file")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		width, _ := strconv.Atoi(c.PostForm("width"))
		height, _ := strconv.Atoi(c.PostForm("height"))
		durationMs, _ := strconv.ParseInt(c.PostForm("durationMs"), 10, 64)
		purpose := c.PostForm("purpose")
		kind := c.PostForm("kind")
		resource, err := svc.UploadBatchVideoResource(user.ID, file, kind, purpose, width, height, durationMs, c.GetHeader("X-Idempotency-Key"))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, resource)
	})

	r.POST("/uploads/discard", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input struct {
			ResourceIDs []string `json:"resourceIds"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		count, err := svc.DiscardBatchVideoUploads(user.ID, input.ResourceIDs)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"discarded": count})
	})

	r.GET("/settings", func(c *gin.Context) {
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		settings, err := svc.BatchVideoSettings()
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, settings)
	})

	r.GET("/models", func(c *gin.Context) {
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		models, err := svc.ListBatchVideoModels()
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, models)
	})

	r.GET("/projects", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.Query("page"))
		pageSize, _ := strconv.Atoi(c.Query("pageSize"))
		projects, err := svc.ListBatchVideoProjects(user.ID, c.Query("stage"), page, pageSize)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, projects)
	})

	r.POST("/projects", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.CreateBatchVideoProjectInput
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		project, err := svc.CreateBatchVideoProject(user.ID, input)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, project)
	})

	r.GET("/projects/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.BatchVideoProjectDetail(user.ID, c.Param("id"))
		if err != nil {
			fail(c, http.StatusNotFound, err)
			return
		}
		ok(c, project)
	})

	r.DELETE("/projects/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.DeleteBatchVideoProject(user.ID, c.Param("id")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	})

	r.PATCH("/projects/:id/settings", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.BatchVideoProjectSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		project, err := svc.UpdateBatchVideoProjectSettings(user.ID, c.Param("id"), input)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/material-batches", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input struct {
			Label     string                            `json:"label"`
			Materials []service.BatchVideoMaterialInput `json:"materials"`
			Documents []service.BatchVideoDocumentInput `json:"documents"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		batch, err := svc.AddBatchVideoMaterialBatch(user.ID, c.Param("id"), input.Label, input.Materials, input.Documents)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, batch)
	})

	r.DELETE("/projects/:id/material-batches", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.ClearBatchVideoMaterials(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.DELETE("/projects/:id/material-batches/:batchId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.DeleteBatchVideoMaterialBatch(user.ID, c.Param("id"), c.Param("batchId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.DELETE("/projects/:id/material-batches/:batchId/materials/:resourceId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.DeleteBatchVideoMaterial(user.ID, c.Param("id"), c.Param("batchId"), c.Param("resourceId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/analyze", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req struct {
			Scope string `json:"scope"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<10)
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			fail(c, http.StatusBadRequest, err)
			return
		}
		project, err := svc.AnalyzeBatchVideoProject(user.ID, c.Param("id"), req.Scope)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/analysis/continue", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.ContinueBatchVideoAnalysis(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/analysis/cancel", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.CancelBatchVideoAnalysis(c.Request.Context(), user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/planning/cancel", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.CancelBatchVideoPlanning(c.Request.Context(), user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/creatives/generate", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.GenerateBatchVideoCreativesInput
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.GenerateBatchVideoCreatives(user.ID, c.Param("id"), input)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, result)
	})

	r.GET("/projects/:id/creatives", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.Query("page"))
		pageSize, _ := strconv.Atoi(c.Query("pageSize"))
		creatives, total, err := svc.ListBatchVideoCreatives(user.ID, c.Param("id"), c.Query("roundId"), c.Query("reviewStatus"), c.Query("matchStatus"), page, pageSize)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"creatives": creatives, "total": total})
	})

	r.PATCH("/projects/:id/creatives/:creativeId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.UpdateBatchVideoCreativeInput
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		creative, err := svc.UpdateBatchVideoCreativeContent(user.ID, c.Param("id"), c.Param("creativeId"), input)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, creative)
	})

	r.POST("/projects/:id/creatives/:creativeId/rematch", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		creative, err := svc.RematchBatchVideoCreative(user.ID, c.Param("id"), c.Param("creativeId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, creative)
	})

	r.POST("/projects/:id/creatives/:creativeId/review", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		creative, err := svc.ReviewBatchVideoCreative(user.ID, c.Param("id"), c.Param("creativeId"), input.Status, input.Note)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, creative)
	})

	r.POST("/projects/:id/creatives/:creativeId/generate-reference", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		creative, err := svc.GenerateBatchVideoReferenceImage(user.ID, c.Param("id"), c.Param("creativeId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, creative)
	})

	r.POST("/projects/:id/creatives/review-batch", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input struct {
			IDs    []string `json:"ids"`
			Status string   `json:"status"`
			Note   string   `json:"note"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		count, err := svc.ReviewBatchVideoCreatives(user.ID, c.Param("id"), input.IDs, input.Status, input.Note)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"updated": count})
	})

	r.GET("/projects/:id/reference-assets", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		assets, err := svc.ListBatchVideoReferenceAssets(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, assets)
	})

	r.POST("/projects/:id/reference-assets", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input struct {
			Assets []service.BatchVideoReferenceAssetInput `json:"assets"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		assets, err := svc.AddBatchVideoReferenceAssets(user.ID, c.Param("id"), input.Assets)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, assets)
	})

	r.GET("/projects/:id/validation", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		state, err := svc.BatchVideoValidation(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, state)
	})

	r.POST("/projects/:id/unlock-mass", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		project, err := svc.UnlockBatchVideoMass(user.ID, c.Param("id"))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, project)
	})

	r.POST("/projects/:id/preflight", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.BatchVideoSubmitInput
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		preflight, err := svc.PreviewBatchVideoGeneration(user.ID, c.Param("id"), input)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, preflight)
	})

	r.POST("/projects/:id/submit", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.BatchVideoSubmitInput
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		count, err := svc.SubmitBatchVideoGeneration(user.ID, c.Param("id"), input)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"jobs": count})
	})

	r.POST("/projects/:id/start", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input struct {
			ScheduledAt *time.Time `json:"scheduledAt"`
		}
		if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
			fail(c, http.StatusBadRequest, err)
			return
		}
		count, err := svc.StartBatchVideoGeneration(user.ID, c.Param("id"), input.ScheduledAt)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"jobs": count})
	})

	r.POST("/projects/:id/cancel-pending", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		count, err := svc.CancelPendingBatchVideoJobs(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"cancelled": count})
	})

	r.GET("/projects/:id/jobs", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.Query("page"))
		pageSize, _ := strconv.Atoi(c.Query("pageSize"))
		jobs, err := svc.ListBatchVideoJobs(user.ID, c.Param("id"), c.Query("status"), page, pageSize)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, jobs)
	})

	r.GET("/projects/:id/jobs/:jobId/attempts", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		attempts, err := svc.ListBatchVideoJobAttempts(user.ID, c.Param("id"), c.Param("jobId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, attempts)
	})

	r.POST("/projects/:id/jobs/:jobId/retry", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		job, err := svc.RetryBatchVideoJob(user.ID, c.Param("id"), c.Param("jobId"))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, job)
	})

	r.POST("/projects/:id/retry-failed", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		count, err := svc.RetryFailedBatchVideoJobs(user.ID, c.Param("id"))
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"retried": count})
	})

	r.POST("/projects/:id/pause", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.PauseBatchVideoProject(user.ID, c.Param("id")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"status": "paused"})
	})

	r.POST("/projects/:id/resume", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.ResumeBatchVideoProject(user.ID, c.Param("id")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"status": "running"})
	})

	r.GET("/projects/:id/stats", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		stats, err := svc.BatchVideoStats(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, stats)
	})

	admin := api.Group("/admin/batch-video")
	admin.GET("/settings", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		settings, err := svc.AdminBatchVideoSettings(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, settings)
	})
	admin.PUT("/settings", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var input service.BatchVideoSettings
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		settings, err := svc.UpdateBatchVideoSettings(user, input)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, settings)
	})
}
