// 管理端任务看板接口：列表（含状态计数）、按用户成功/失败汇总、单条详情与媒体投递。
//
// 媒体接口只在管理员点击播放时被调用一次；列表与轮询路径不触碰对象存储。

package handler

import (
	"net/http"

	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

func registerAdminTaskMonitorRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.GET("/admin/tasks", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, limit, err := parsePaginationQuery(c, 50)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.AdminTasks(user, service.AdminTaskQuery{
			AnalyticsQuery: analyticsQuery(c),
			Status:         c.Query("status"),
			Keyword:        c.Query("keyword"),
			Page:           page,
			Limit:          limit,
		})
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.GET("/admin/task-metrics/users", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		stats, err := svc.AdminTaskUserStats(user, service.AdminTaskQuery{AnalyticsQuery: analyticsQuery(c)})
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"users": stats})
	})
	r.GET("/admin/tasks/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		task, err := svc.AdminTask(user, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"task": task})
	})
	r.GET("/admin/tasks/:id/media", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		delivery, err := svc.PrepareAdminTaskMediaDelivery(user, c.Param("id"), resourceAccessOptions(c), c.GetHeader("Range"))
		if err != nil {
			failService(c, err)
			return
		}
		// 任务结果一经成功即不可变，允许浏览器按资源版本缓存，重复查看同一媒体不再回源。
		serveResourceDelivery(c, delivery, "private, max-age=300", "")
	})
}
