package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"encoding/json"

	"kvm_console/logger"

	"kvm_console/model"
)

// ErrTaskCanceled 任务取消错误
var ErrTaskCanceled = errors.New("任务已被用户取消")

// ErrTaskNotFound 任务不存在错误
var ErrTaskNotFound = errors.New("任务不存在")

// ErrTaskAccessDenied 任务访问权限不足错误
var ErrTaskAccessDenied = errors.New("无权访问该任务")

// TaskFunc 任务执行函数类型
// 接收 context（用于取消信号）、任务对象和进度回调，返回结果和错误
type TaskFunc func(ctx context.Context, task *model.Task, progress func(int, string)) (string, error)

// TaskEvent 任务事件（用于 SSE 推送）
type TaskEvent struct {
	TaskID   uint   `json:"task_id"`
	Type     string `json:"type"`     // 任务类型
	Status   string `json:"status"`   // 任务状态
	Progress int    `json:"progress"` // 进度 0-100
	Message  string `json:"message"`  // 状态消息
}

// ===================== 内存任务存储 =====================

var (
	taskStore    = make(map[uint]*model.Task)        // 任务存储（内存）
	taskCancelFn = make(map[uint]context.CancelFunc) // 运行中任务的取消函数
	taskStoreMu  sync.RWMutex
	taskIDSeq    uint64 // 自增 ID 序列
)

// nextTaskID 生成下一个任务 ID
func nextTaskID() uint {
	return uint(atomic.AddUint64(&taskIDSeq, 1))
}

// storeTask 存储任务
func storeTask(task *model.Task) {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()
	taskStore[task.ID] = task
}

// getTask 获取任务
func getTask(id uint) (*model.Task, bool) {
	taskStoreMu.RLock()
	defer taskStoreMu.RUnlock()
	task, ok := taskStore[id]
	return task, ok
}

// HasActiveTask 判断是否存在等待中或运行中的指定类型任务。
func HasActiveTask(taskType string, match func(params string) bool) bool {
	taskStoreMu.RLock()
	defer taskStoreMu.RUnlock()
	for _, task := range taskStore {
		if task.Type != taskType {
			continue
		}
		if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRunning {
			continue
		}
		if match == nil || match(task.Params) {
			return true
		}
	}
	return false
}

// GetActiveTask 返回指定类型最新的等待中或运行中任务副本。
func GetActiveTask(taskType string) (*model.Task, bool) {
	taskStoreMu.RLock()
	defer taskStoreMu.RUnlock()
	var found *model.Task
	for _, task := range taskStore {
		if task.Type != taskType || (task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRunning) {
			continue
		}
		if found == nil || task.ID > found.ID {
			copyValue := *task
			found = &copyValue
		}
	}
	return found, found != nil
}

// updateTask 更新任务字段
func updateTask(id uint, updater func(task *model.Task)) {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()
	if task, ok := taskStore[id]; ok {
		updater(task)
		task.UpdatedAt = time.Now()
	}
}

// deleteTask 删除任务
func deleteTask(id uint) {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()
	delete(taskStore, id)
}

// storeCancelFn 存储取消函数
func storeCancelFn(taskID uint, cancel context.CancelFunc) {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()
	taskCancelFn[taskID] = cancel
}

// removeCancelFn 移除取消函数
func removeCancelFn(taskID uint) {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()
	delete(taskCancelFn, taskID)
}

// ===================== SSE 事件中心 =====================

var (
	sseClients   = make(map[chan TaskEvent]bool)
	sseClientsMu sync.RWMutex
)

// RegisterSSEClient 注册 SSE 客户端
func RegisterSSEClient(ch chan TaskEvent) {
	sseClientsMu.Lock()
	defer sseClientsMu.Unlock()
	sseClients[ch] = true
	logger.App.Info("SSE客户端已连接", "connections", len(sseClients))
}

// UnregisterSSEClient 注销 SSE 客户端
func UnregisterSSEClient(ch chan TaskEvent) {
	sseClientsMu.Lock()
	defer sseClientsMu.Unlock()
	delete(sseClients, ch)
	close(ch)
	logger.App.Info("SSE客户端已断开", "connections", len(sseClients))
}

// broadcastEvent 广播任务事件到所有 SSE 客户端
func broadcastEvent(event TaskEvent) {
	sseClientsMu.RLock()
	defer sseClientsMu.RUnlock()
	for ch := range sseClients {
		select {
		case ch <- event:
		default:
			// 客户端缓冲区满，跳过
		}
	}
}

// ===================== 任务处理器 =====================

var handlers = make(map[string]TaskFunc)
var handlersMu sync.RWMutex

// RegisterHandler 注册任务处理器
func RegisterHandler(taskType string, handler TaskFunc) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers[taskType] = handler
	logger.App.Info("注册任务处理器", "type", taskType)
}

// ===================== 任务队列核心 =====================

var taskChan = make(chan uint, 100)

// Start 启动任务队列消费者和自动清理
func Start(workerCount int) {
	for i := 0; i < workerCount; i++ {
		go worker(i)
	}
	// 启动 24 小时自动清理协程
	go autoCleanup()
	logger.App.Info("任务队列已启动", "workers", workerCount)
}

// Submit 提交任务
func Submit(taskType, params, createdBy string) (*model.Task, error) {
	task := &model.Task{
		ID:        nextTaskID(),
		Type:      taskType,
		Status:    model.TaskStatusPending,
		Params:    params,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 存入内存
	storeTask(task)

	// 广播新任务事件
	broadcastEvent(TaskEvent{
		TaskID:   task.ID,
		Type:     task.Type,
		Status:   model.TaskStatusPending,
		Progress: 0,
		Message:  "任务已提交",
	})

	// 发送到任务通道
	taskChan <- task.ID
	logger.App.Info("任务已提交", "id", task.ID, "type", taskType)
	return task, nil
}

// SubmitWithStruct 提交任务（结构体参数）
func SubmitWithStruct(taskType string, params interface{}, createdBy string) (*model.Task, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return Submit(taskType, string(paramsJSON), createdBy)
}

// worker 任务消费者
func worker(id int) {
	for taskID := range taskChan {
		processTask(id, taskID)
	}
}

// processTask 处理单个任务
func processTask(workerID int, taskID uint) {
	task, ok := getTask(taskID)
	if !ok {
		logger.App.Warn("Worker获取任务失败", "worker", workerID, "id", taskID, "error", "任务不存在")
		return
	}

	// 检查是否已取消
	if task.Status == model.TaskStatusCanceled {
		logger.App.Info("任务已取消跳过", "worker", workerID, "id", taskID)
		return
	}

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	storeCancelFn(taskID, cancel)
	defer func() {
		cancel()
		removeCancelFn(taskID)
	}()

	// 更新状态为运行中
	updateTask(taskID, func(t *model.Task) {
		t.Status = model.TaskStatusRunning
		t.Message = "任务开始执行"
	})

	broadcastEvent(TaskEvent{
		TaskID:   taskID,
		Type:     task.Type,
		Status:   model.TaskStatusRunning,
		Progress: 0,
		Message:  "任务开始执行",
	})

	logger.App.Info("开始执行任务", "worker", workerID, "id", taskID, "type", task.Type)

	// 查找处理器
	handlersMu.RLock()
	handler, exists := handlers[task.Type]
	handlersMu.RUnlock()

	if !exists {
		updateTask(taskID, func(t *model.Task) {
			t.Status = model.TaskStatusFailed
			t.Message = "未找到任务处理器: " + task.Type
		})
		broadcastEvent(TaskEvent{
			TaskID:   taskID,
			Type:     task.Type,
			Status:   model.TaskStatusFailed,
			Progress: 0,
			Message:  "未找到任务处理器: " + task.Type,
		})
		logger.App.Warn("Worker未找到处理器", "worker", workerID, "type", task.Type)
		return
	}

	// 进度回调（同时更新内存和广播 SSE，并检查取消状态）
	progressFn := func(progress int, message string) {
		updateTask(taskID, func(t *model.Task) {
			t.Progress = progress
			t.Message = message
		})
		broadcastEvent(TaskEvent{
			TaskID:   taskID,
			Type:     task.Type,
			Status:   model.TaskStatusRunning,
			Progress: progress,
			Message:  message,
		})
	}

	// 执行任务
	startTime := time.Now()
	result, err := handler(ctx, task, progressFn)
	duration := time.Since(startTime)

	// 判断是否是取消导致的错误
	if err != nil && (err == ErrTaskCanceled || ctx.Err() == context.Canceled) {
		updateTask(taskID, func(t *model.Task) {
			t.Status = model.TaskStatusCanceled
			t.Result = result
			t.Message = "任务已被用户取消"
		})
		broadcastEvent(TaskEvent{
			TaskID:   taskID,
			Type:     task.Type,
			Status:   model.TaskStatusCanceled,
			Progress: task.Progress,
			Message:  "任务已被用户取消",
		})
		logger.App.Info("任务已取消", "worker", workerID, "id", taskID, "duration", duration)
	} else if err != nil {
		updateTask(taskID, func(t *model.Task) {
			t.Status = model.TaskStatusFailed
			t.Result = result
			t.Progress = 100
			t.Message = "任务失败: " + err.Error()
		})
		broadcastEvent(TaskEvent{
			TaskID:   taskID,
			Type:     task.Type,
			Status:   model.TaskStatusFailed,
			Progress: 100,
			Message:  "任务失败: " + err.Error(),
		})
		logger.App.Error("任务失败", "worker", workerID, "id", taskID, "duration", duration, "error", err)
	} else {
		updateTask(taskID, func(t *model.Task) {
			t.Status = model.TaskStatusSuccess
			t.Result = result
			t.Progress = 100
			t.Message = "任务完成"
		})
		broadcastEvent(TaskEvent{
			TaskID:   taskID,
			Type:     task.Type,
			Status:   model.TaskStatusSuccess,
			Progress: 100,
			Message:  "任务完成",
		})
		logger.App.Info("任务完成", "worker", workerID, "id", taskID, "duration", duration)
	}
}

// ===================== 查询接口 =====================

// GetTask 获取任务信息
func GetTask(taskID uint) (*model.Task, error) {
	task, ok := getTask(taskID)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrTaskNotFound, taskID)
	}
	return task, nil
}

// GetTaskList 获取任务列表（向后兼容）
func GetTaskList(page, pageSize int) ([]model.Task, int64, error) {
	return GetTaskListFiltered(page, pageSize, "", "")
}

// GetTaskListFiltered 获取任务列表（支持状态和类型筛选）
func GetTaskListFiltered(page, pageSize int, status, taskType string) ([]model.Task, int64, error) {
	return GetTaskListFilteredForUser(page, pageSize, status, taskType, "", "admin")
}

func canAccessTask(task *model.Task, username, role string) bool {
	if task == nil {
		return false
	}
	if role == "admin" {
		return true
	}
	if username == "" {
		return false
	}
	return task.CreatedBy == username
}

// GetTaskForUser 获取指定用户可访问的任务详情。
func GetTaskForUser(taskID uint, username, role string) (*model.Task, error) {
	taskStoreMu.RLock()
	defer taskStoreMu.RUnlock()

	task, ok := taskStore[taskID]
	if !ok {
		return nil, ErrTaskNotFound
	}
	if !canAccessTask(task, username, role) {
		return nil, ErrTaskAccessDenied
	}
	return task, nil
}

// GetTaskListFilteredForUser 获取指定用户可访问的任务列表（支持状态和类型筛选）。
func GetTaskListFilteredForUser(page, pageSize int, status, taskType, username, role string) ([]model.Task, int64, error) {
	taskStoreMu.RLock()
	defer taskStoreMu.RUnlock()

	// 筛选
	var filtered []*model.Task
	for _, task := range taskStore {
		if !canAccessTask(task, username, role) {
			continue
		}
		if status != "" && task.Status != status {
			continue
		}
		if taskType != "" && task.Type != taskType {
			continue
		}
		filtered = append(filtered, task)
	}

	// 按创建时间倒序排列
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := int64(len(filtered))

	// 分页
	offset := (page - 1) * pageSize
	if offset >= len(filtered) {
		return []model.Task{}, total, nil
	}
	end := offset + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}

	// 返回副本，避免外部修改
	result := make([]model.Task, 0, end-offset)
	for _, t := range filtered[offset:end] {
		result = append(result, *t)
	}

	return result, total, nil
}

// ===================== 操作接口 =====================

// CancelTask 取消任务（支持等待中和运行中）
func CancelTask(taskID uint) error {
	return CancelTaskForUser(taskID, "", "admin")
}

// CancelTaskForUser 取消指定用户可访问的任务（支持等待中和运行中）。
func CancelTaskForUser(taskID uint, username, role string) error {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()

	task, ok := taskStore[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	if !canAccessTask(task, username, role) {
		return ErrTaskAccessDenied
	}

	switch task.Status {
	case model.TaskStatusPending:
		// 等待中的任务：直接标记取消
		task.Status = model.TaskStatusCanceled
		task.Message = "任务已取消"
		task.UpdatedAt = time.Now()

	case model.TaskStatusRunning:
		// 运行中的任务：触发 context 取消信号
		if cancelFn, exists := taskCancelFn[taskID]; exists {
			cancelFn()
		}
		// 状态会在 processTask 中检测到取消后更新
		// 先标记消息让前端即时看到
		task.Message = "正在取消任务..."
		task.UpdatedAt = time.Now()

	default:
		return fmt.Errorf("任务已结束，无法取消（当前状态: %s）", task.Status)
	}

	broadcastEvent(TaskEvent{
		TaskID:   taskID,
		Type:     task.Type,
		Status:   task.Status,
		Progress: task.Progress,
		Message:  task.Message,
	})

	return nil
}

// ClearFinishedTasks 清理已完成/失败/取消的任务
func ClearFinishedTasks() (int64, error) {
	return ClearFinishedTasksForUser("", "admin")
}

// ClearFinishedTasksForUser 清理指定用户可访问的已结束任务。
func ClearFinishedTasksForUser(username, role string) (int64, error) {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()

	var count int64
	for id, task := range taskStore {
		if !canAccessTask(task, username, role) {
			continue
		}
		if task.Status == model.TaskStatusSuccess ||
			task.Status == model.TaskStatusFailed ||
			task.Status == model.TaskStatusCanceled {
			delete(taskStore, id)
			count++
		}
	}

	return count, nil
}

// ===================== 24小时自动清理 =====================

// autoCleanup 每小时检查一次，删除超过 24 小时的任务
func autoCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cleanupExpiredTasks()
	}
}

// cleanupExpiredTasks 清理超过 24 小时的任务
func cleanupExpiredTasks() {
	taskStoreMu.Lock()
	defer taskStoreMu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	count := 0
	for id, task := range taskStore {
		// 只清理已结束的任务（不清理正在运行或等待中的）
		if task.CreatedAt.Before(cutoff) &&
			task.Status != model.TaskStatusPending &&
			task.Status != model.TaskStatusRunning {
			delete(taskStore, id)
			count++
		}
	}

	if count > 0 {
		logger.App.Info("自动清理过期任务", "deleted", count)
	}
}
