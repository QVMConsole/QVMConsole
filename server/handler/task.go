package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/middleware"
	"kvm_console/taskqueue"
)

// GetTaskList 获取任务列表（支持筛选）
func GetTaskList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	taskType := c.Query("type")
	username, role := currentUserAndRole(c)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tasks, total, err := taskqueue.GetTaskListFilteredForUser(page, pageSize, status, taskType, username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取任务列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"list":      tasks,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetTaskDetail 获取任务详情
func GetTaskDetail(c *gin.Context) {
	idStr := c.Param("id")
	username, role := currentUserAndRole(c)
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的任务 ID",
		})
		return
	}

	task, err := taskqueue.GetTaskForUser(uint(id), username, role)
	if err != nil {
		if errors.Is(err, taskqueue.ErrTaskAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权查看该任务",
			})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "任务不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    task,
	})
}

// SSETaskProgress SSE 实时推送任务进度
func SSETaskProgress(c *gin.Context) {
	username, role := currentUserAndRole(c)

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// 创建事件通道
	eventChan := make(chan taskqueue.TaskEvent, 50)
	taskqueue.RegisterSSEClient(eventChan)

	// 客户端断开时清理
	clientGone := c.Request.Context().Done()
	sessionTicker := time.NewTicker(15 * time.Second)
	defer sessionTicker.Stop()

	// 确保退出时注销客户端
	defer taskqueue.UnregisterSSEClient(eventChan)

	// 发送初始连接确认事件
	firstRun := true
	c.Stream(func(w io.Writer) bool {
		if firstRun {
			firstRun = false
			c.SSEvent("connected", map[string]string{"status": "ok"})
			return true
		}
		select {
		case event, ok := <-eventChan:
			if !ok {
				return false
			}
			if _, err := taskqueue.GetTaskForUser(event.TaskID, username, role); err != nil {
				return true
			}
			// 发送 SSE 事件
			c.SSEvent("task_progress", event)
			return true
		case <-clientGone:
			return false
		case <-sessionTicker.C:
			if !middleware.StreamingSessionValid(c) {
				c.SSEvent("session_expired", gin.H{"message": "登录会话因长时间未操作已失效，请重新登录"})
				return false
			}
			return true
		}
	})
}

// CancelTask 取消等待中的任务
func CancelTask(c *gin.Context) {
	idStr := c.Param("id")
	username, role := currentUserAndRole(c)
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的任务 ID",
		})
		return
	}

	if err := taskqueue.CancelTaskForUser(uint(id), username, role); err != nil {
		switch {
		case errors.Is(err, taskqueue.ErrTaskAccessDenied):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "无权取消该任务",
			})
		case errors.Is(err, taskqueue.ErrTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": "任务不存在",
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "任务已取消",
	})
}

// ClearFinishedTasks 清理已完成的任务
func ClearFinishedTasks(c *gin.Context) {
	if !requireHighRiskVerification(c, "clear_finished_tasks") {
		return
	}
	username, role := currentUserAndRole(c)
	count, err := taskqueue.ClearFinishedTasksForUser(username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "清理失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("已清理 %d 条任务记录", count),
		"data": gin.H{
			"cleared": count,
		},
	})
}
