package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/middleware"
	"kvm_console/model"
	"kvm_console/service"
)

// GetSchedulerList 获取调度器概览。
func GetSchedulerList(c *gin.Context) {
	list, err := service.ListSchedulers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取调度器列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    list,
	})
}

// GetSchedulerEventList 获取调度事件列表。
func GetSchedulerEventList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	start, err := parseSchedulerEventTime(c.Query("start"), false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "开始时间格式无效"})
		return
	}
	end, err := parseSchedulerEventTime(c.Query("end"), true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "结束时间格式无效"})
		return
	}

	list, total, err := model.ListSchedulerEvents(model.SchedulerEventFilter{
		Page:         page,
		PageSize:     pageSize,
		SchedulerKey: strings.TrimSpace(c.Query("scheduler_key")),
		Status:       strings.TrimSpace(c.Query("status")),
		VMName:       strings.TrimSpace(c.Query("vm_name")),
		Start:        start,
		End:          end,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取调度事件失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"list":      list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// SSESchedulerEvents 实时推送调度事件。
func SSESchedulerEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	eventChan := make(chan service.SchedulerEventMessage, 50)
	service.RegisterSchedulerSSEClient(eventChan)
	defer service.UnregisterSchedulerSSEClient(eventChan)

	clientGone := c.Request.Context().Done()
	sessionTicker := time.NewTicker(15 * time.Second)
	defer sessionTicker.Stop()
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
			c.SSEvent("scheduler_event", event)
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

func parseSchedulerEventTime(value string, endOfDay bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			if layout == "2006-01-02" && endOfDay {
				parsed = parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			}
			return &parsed, nil
		}
	}
	return nil, strconv.ErrSyntax
}
