package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// TaskType represents the type of scheduled task
type TaskType string

const (
	TaskTypeEventProcessing TaskType = "event_processing"
	TaskTypePendingFaces    TaskType = "pending_faces"
	TaskTypeLogPurge        TaskType = "log_purge"
	TaskTypeManual          TaskType = "manual"
)

// TaskStatus represents the status of a task execution
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskExecution represents a single task execution record
type TaskExecution struct {
	ID          string                 `json:"id"`
	TaskType    TaskType               `json:"taskType"`
	Status      TaskStatus             `json:"status"`
	StartedAt   time.Time              `json:"startedAt"`
	CompletedAt *time.Time             `json:"completedAt,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Result      map[string]interface{} `json:"result,omitempty"`
	TriggeredBy string                 `json:"triggeredBy"` // "scheduled" or "manual"
}

// QueuedEvent represents an event waiting to be processed
type QueuedEvent struct {
	ID        string                 `json:"id"`
	EventID   string                 `json:"eventId"`
	Type      string                 `json:"type"`
	Camera    string                 `json:"camera"`
	Label     string                 `json:"label"`
	Score     float32                `json:"score"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Status    TaskStatus             `json:"status"`
	Attempts  int                    `json:"attempts"`
	MaxAttempts int                  `json:"maxAttempts"`
}

// TaskScheduler manages scheduled tasks and their execution history
type TaskScheduler struct {
	config           *FNDTaskSchedulerConfiguration
	eventQueue       []QueuedEvent
	executionHistory []TaskExecution
	eventManager     *FNDFrigateEventManager
	notifyManager    *FNDNotificationManager
	api              *FNDFrigateApi
	
	// Task execution control
	ctx              context.Context
	cancel           context.CancelFunc
	tickers          map[TaskType]*time.Ticker
	runningTasks     map[string]bool
	taskMutex        sync.RWMutex
	queueMutex       sync.RWMutex
	historyMutex     sync.RWMutex
	
	// File paths
	historyFilePath  string
	queueFilePath    string
}

// NewTaskScheduler creates a new task scheduler instance
func NewTaskScheduler(config *FNDTaskSchedulerConfiguration, eventManager *FNDFrigateEventManager, notifyManager *FNDNotificationManager, api *FNDFrigateApi) *TaskScheduler {
	scheduler := &TaskScheduler{
		config:        config,
		eventQueue:    make([]QueuedEvent, 0),
		executionHistory: make([]TaskExecution, 0),
		eventManager:  eventManager,
		notifyManager: notifyManager,
		api:           api,
		tickers:       make(map[TaskType]*time.Ticker),
		runningTasks:  make(map[string]bool),
	}
	
	scheduler.ctx, scheduler.cancel = context.WithCancel(context.Background())
	
	// Set up file paths - store in fnd_conf directory
	scheduler.historyFilePath = "fnd_conf/task_history.json"
	scheduler.queueFilePath = "fnd_conf/event_queue.json"
	
	// Load existing data
	scheduler.loadExecutionHistory()
	scheduler.loadEventQueue()
	
	return scheduler
}

// Start starts the task scheduler
func (ts *TaskScheduler) Start() {
	LogInfo("TASK_SCHEDULER", "Starting task scheduler", "")
	
	// Start event processing task
	if ts.config.EnableEventQueue {
		interval := time.Duration(ts.config.EventProcessingInterval) * time.Minute
		ts.tickers[TaskTypeEventProcessing] = time.NewTicker(interval)
		LogInfo("TASK_SCHEDULER", "Event processing task scheduled", fmt.Sprintf("Interval: %v", interval))
	}
	
	// Start pending faces task (if different from background task)
	if ts.config.PendingFacesInterval > 0 {
		interval := time.Duration(ts.config.PendingFacesInterval) * time.Hour
		ts.tickers[TaskTypePendingFaces] = time.NewTicker(interval)
		LogInfo("TASK_SCHEDULER", "Pending faces task scheduled", fmt.Sprintf("Interval: %v", interval))
	}
	
	// Start log purge task
	if ts.config.LogPurgeInterval > 0 {
		interval := time.Duration(ts.config.LogPurgeInterval) * time.Hour
		ts.tickers[TaskTypeLogPurge] = time.NewTicker(interval)
		LogInfo("TASK_SCHEDULER", "Log purge task scheduled", fmt.Sprintf("Interval: %v", interval))
	}
	
	// Start the main scheduler loop
	go ts.schedulerLoop()
}

// Stop stops the task scheduler
func (ts *TaskScheduler) Stop() {
	LogInfo("TASK_SCHEDULER", "Stopping task scheduler", "")
	
	ts.cancel()
	
	// Stop all tickers
	for taskType, ticker := range ts.tickers {
		ticker.Stop()
		LogInfo("TASK_SCHEDULER", "Stopped ticker", fmt.Sprintf("Task type: %s", taskType))
	}
	
	// Save data before stopping
	ts.saveExecutionHistory()
	ts.saveEventQueue()
}

// schedulerLoop is the main scheduler loop
func (ts *TaskScheduler) schedulerLoop() {
	LogInfo("TASK_SCHEDULER", "Scheduler loop started", "")
	
	for {
		select {
		case <-ts.ctx.Done():
			LogInfo("TASK_SCHEDULER", "Scheduler loop stopped", "")
			return
			
		case <-ts.tickers[TaskTypeEventProcessing].C:
			ts.executeTask(TaskTypeEventProcessing, "scheduled")
			
		case <-ts.tickers[TaskTypePendingFaces].C:
			ts.executeTask(TaskTypePendingFaces, "scheduled")
			
		case <-ts.tickers[TaskTypeLogPurge].C:
			ts.executeTask(TaskTypeLogPurge, "scheduled")
		}
	}
}

// executeTask executes a specific task
func (ts *TaskScheduler) executeTask(taskType TaskType, triggeredBy string) {
	executionID := fmt.Sprintf("%s_%d", taskType, time.Now().Unix())
	
	// Check if task is already running
	ts.taskMutex.Lock()
	if ts.runningTasks[executionID] {
		ts.taskMutex.Unlock()
		LogWarn("TASK_SCHEDULER", "Task already running, skipping", fmt.Sprintf("Task: %s, ID: %s", taskType, executionID))
		return
	}
	ts.runningTasks[executionID] = true
	ts.taskMutex.Unlock()
	
	// Create execution record
	execution := TaskExecution{
		ID:          executionID,
		TaskType:    taskType,
		Status:      TaskStatusRunning,
		StartedAt:   time.Now(),
		TriggeredBy: triggeredBy,
	}
	
	// Add to history
	ts.addExecutionToHistory(execution)
	
	LogInfo("TASK_SCHEDULER", "Starting task execution", fmt.Sprintf("Task: %s, ID: %s, Triggered by: %s", taskType, executionID, triggeredBy))
	
	// Execute the task
	var result map[string]interface{}
	var err error
	
	switch taskType {
	case TaskTypeEventProcessing:
		result, err = ts.processEventQueue()
	case TaskTypePendingFaces:
		result, err = ts.processPendingFaces()
	case TaskTypeLogPurge:
		result, err = ts.purgeOldLogs()
	default:
		err = fmt.Errorf("unknown task type: %s", taskType)
	}
	
	// Update execution record
	completedAt := time.Now()
	duration := completedAt.Sub(execution.StartedAt)
	
	if err != nil {
		execution.Status = TaskStatusFailed
		execution.Error = err.Error()
		LogError("TASK_SCHEDULER", "Task execution failed", fmt.Sprintf("Task: %s, ID: %s, Error: %s", taskType, executionID, err.Error()))
	} else {
		execution.Status = TaskStatusCompleted
		execution.Result = result
		LogInfo("TASK_SCHEDULER", "Task execution completed", fmt.Sprintf("Task: %s, ID: %s, Duration: %v", taskType, executionID, duration))
	}
	
	execution.CompletedAt = &completedAt
	execution.Duration = duration
	
	// Update history
	ts.updateExecutionInHistory(execution)
	
	// Remove from running tasks
	ts.taskMutex.Lock()
	delete(ts.runningTasks, executionID)
	ts.taskMutex.Unlock()
}

// QueueEvent adds an event to the processing queue
func (ts *TaskScheduler) QueueEvent(event QueuedEvent) error {
	ts.queueMutex.Lock()
	defer ts.queueMutex.Unlock()
	
	// Check queue size limit
	if len(ts.eventQueue) >= ts.config.MaxEventQueueSize {
		LogWarn("TASK_SCHEDULER", "Event queue full, dropping oldest event", fmt.Sprintf("Queue size: %d, Max: %d", len(ts.eventQueue), ts.config.MaxEventQueueSize))
		// Remove oldest event
		ts.eventQueue = ts.eventQueue[1:]
	}
	
	// Set default values
	if event.MaxAttempts == 0 {
		event.MaxAttempts = 3
	}
	event.Status = TaskStatusPending
	event.Timestamp = time.Now()
	
	ts.eventQueue = append(ts.eventQueue, event)
	
	LogDebug("TASK_SCHEDULER", "Event queued", fmt.Sprintf("Event ID: %s, Queue size: %d", event.EventID, len(ts.eventQueue)))
	
	// Save queue to file
	return ts.saveEventQueue()
}

// processEventQueue processes all queued events
func (ts *TaskScheduler) processEventQueue() (map[string]interface{}, error) {
	LogInfo("TASK_SCHEDULER", "Processing event queue", "")
	
	ts.queueMutex.Lock()
	events := make([]QueuedEvent, len(ts.eventQueue))
	copy(events, ts.eventQueue)
	ts.queueMutex.Unlock()
	
	if len(events) == 0 {
		LogInfo("TASK_SCHEDULER", "No events to process", "")
		return map[string]interface{}{
			"processed": 0,
			"failed":    0,
			"remaining": 0,
		}, nil
	}
	
	processed := 0
	failed := 0
	
	for _, event := range events {
		if event.Status != TaskStatusPending {
			continue
		}
		
		LogDebug("TASK_SCHEDULER", "Processing queued event", fmt.Sprintf("Event ID: %s, Camera: %s, Label: %s", event.EventID, event.Camera, event.Label))
		
		// Process the event
		err := ts.processQueuedEvent(&event)
		
		if err != nil {
			event.Attempts++
			if event.Attempts >= event.MaxAttempts {
				event.Status = TaskStatusFailed
				failed++
				LogError("TASK_SCHEDULER", "Event processing failed permanently", fmt.Sprintf("Event ID: %s, Attempts: %d, Error: %s", event.EventID, event.Attempts, err.Error()))
			} else {
				LogWarn("TASK_SCHEDULER", "Event processing failed, will retry", fmt.Sprintf("Event ID: %s, Attempts: %d/%d, Error: %s", event.EventID, event.Attempts, event.MaxAttempts, err.Error()))
			}
		} else {
			event.Status = TaskStatusCompleted
			processed++
			LogInfo("TASK_SCHEDULER", "Event processed successfully", fmt.Sprintf("Event ID: %s", event.EventID))
		}
		
		// Update the event in the queue
		ts.queueMutex.Lock()
		for j, queuedEvent := range ts.eventQueue {
			if queuedEvent.ID == event.ID {
				ts.eventQueue[j] = event
				break
			}
		}
		ts.queueMutex.Unlock()
	}
	
	// Remove completed and failed events
	ts.queueMutex.Lock()
	newQueue := make([]QueuedEvent, 0)
	for _, event := range ts.eventQueue {
		if event.Status == TaskStatusPending {
			newQueue = append(newQueue, event)
		}
	}
	ts.eventQueue = newQueue
	ts.queueMutex.Unlock()
	
	// Save updated queue
	ts.saveEventQueue()
	
	result := map[string]interface{}{
		"processed": processed,
		"failed":    failed,
		"remaining": len(ts.eventQueue),
	}
	
	LogInfo("TASK_SCHEDULER", "Event queue processing completed", fmt.Sprintf("Processed: %d, Failed: %d, Remaining: %d", processed, failed, len(ts.eventQueue)))
	
	return result, nil
}

// processQueuedEvent processes a single queued event
func (ts *TaskScheduler) processQueuedEvent(event *QueuedEvent) error {
	// Convert back to eventMessage format
	eventMsg := eventMessage{
		TypeInfo: event.Type,
		Before: struct {
			Id             string  `json:"id"`
			Camera         string  `json:"camera"`
			Label          string  `json:"label"`
			Top_Score      float32 `json:"top_score"`
			False_Positive bool    `json:"false_positive"`
			Score          float32 `json:"score"`
		}{
			Id:     event.EventID,
			Camera: event.Camera,
			Label:  event.Label,
			Score:  event.Score,
		},
	}
	
	// Process using existing event manager
	return ts.eventManager.addNewEventMessage(eventMsg)
}

// processPendingFaces processes pending faces (if not handled by background task)
func (ts *TaskScheduler) processPendingFaces() (map[string]interface{}, error) {
	LogInfo("TASK_SCHEDULER", "Processing pending faces", "")
	
	if ts.notifyManager.pendingFacesManager == nil {
		return map[string]interface{}{
			"processed": 0,
			"error":     "Pending faces manager not available",
		}, fmt.Errorf("pending faces manager not available")
	}
	
	if ts.notifyManager.facialRecognitionService == nil {
		return map[string]interface{}{
			"processed": 0,
			"error":     "Facial recognition service not available",
		}, fmt.Errorf("facial recognition service not available")
	}
	
	successCount, errorCount, err := ts.notifyManager.pendingFacesManager.ProcessAllPendingEventsWithAI(ts.notifyManager.facialRecognitionService)
	
	result := map[string]interface{}{
		"success": successCount,
		"errors":  errorCount,
		"total":   successCount + errorCount,
	}
	
	if err != nil {
		result["error"] = err.Error()
		return result, err
	}
	
	return result, nil
}

// purgeOldLogs purges old task execution history
func (ts *TaskScheduler) purgeOldLogs() (map[string]interface{}, error) {
	LogInfo("TASK_SCHEDULER", "Purging old task logs", "")
	
	retentionDays := ts.config.TaskHistoryRetentionDays
	if retentionDays <= 0 {
		retentionDays = 7 // Default to 7 days
	}
	
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	
	ts.historyMutex.Lock()
	originalCount := len(ts.executionHistory)
	
	// Count old records before purging
	oldRecords := 0
	for _, execution := range ts.executionHistory {
		if execution.StartedAt.Before(cutoffDate) {
			oldRecords++
		}
	}
	
	newHistory := make([]TaskExecution, 0)
	for _, execution := range ts.executionHistory {
		if execution.StartedAt.After(cutoffDate) {
			newHistory = append(newHistory, execution)
		}
	}
	
	ts.executionHistory = newHistory
	ts.historyMutex.Unlock()
	
	purgedCount := originalCount - len(newHistory)
	
	// Save updated history
	ts.saveExecutionHistory()
	
	result := map[string]interface{}{
		"purged":       purgedCount,
		"retained":     len(newHistory),
		"original":     originalCount,
		"oldRecords":   oldRecords,
		"cutoff":       cutoffDate.Format("2006-01-02 15:04:05"),
		"retentionDays": retentionDays,
	}
	
	if purgedCount > 0 {
		LogInfo("TASK_SCHEDULER", "Log purge completed", fmt.Sprintf("Purged: %d, Retained: %d, Original: %d", purgedCount, len(newHistory), originalCount))
	} else {
		LogInfo("TASK_SCHEDULER", "Log purge completed - no old records found", fmt.Sprintf("Retained: %d, Original: %d, Cutoff: %s", len(newHistory), originalCount, cutoffDate.Format("2006-01-02 15:04:05")))
	}
	
	return result, nil
}

// ForceExecuteTask manually triggers a task execution
func (ts *TaskScheduler) ForceExecuteTask(taskType TaskType) (string, error) {
	LogInfo("TASK_SCHEDULER", "Manually triggering task", fmt.Sprintf("Task: %s", taskType))
	
	// Generate execution ID first
	executionID := fmt.Sprintf("%s_%d", taskType, time.Now().Unix())
	
	// Execute in a goroutine to avoid blocking
	go func() {
		ts.executeTask(taskType, "manual")
		LogInfo("TASK_SCHEDULER", "Manual task execution completed", fmt.Sprintf("Task: %s, ID: %s", taskType, executionID))
	}()
	
	return executionID, nil
}

// GetExecutionHistory returns the task execution history
func (ts *TaskScheduler) GetExecutionHistory(limit int) []TaskExecution {
	ts.historyMutex.RLock()
	defer ts.historyMutex.RUnlock()
	
	if limit <= 0 || limit > len(ts.executionHistory) {
		limit = len(ts.executionHistory)
	}
	
	// Return most recent executions
	start := len(ts.executionHistory) - limit
	if start < 0 {
		start = 0
	}
	
	result := make([]TaskExecution, limit)
	copy(result, ts.executionHistory[start:])
	
	return result
}

// GetEventQueueStats returns statistics about the event queue
func (ts *TaskScheduler) GetEventQueueStats() map[string]interface{} {
	ts.queueMutex.RLock()
	defer ts.queueMutex.RUnlock()
	
	pending := 0
	failed := 0
	completed := 0
	
	for _, event := range ts.eventQueue {
		switch event.Status {
		case TaskStatusPending:
			pending++
		case TaskStatusFailed:
			failed++
		case TaskStatusCompleted:
			completed++
		}
	}
	
	return map[string]interface{}{
		"total":     len(ts.eventQueue),
		"pending":   pending,
		"failed":    failed,
		"completed": completed,
		"maxSize":   ts.config.MaxEventQueueSize,
	}
}

// addExecutionToHistory adds an execution record to history
func (ts *TaskScheduler) addExecutionToHistory(execution TaskExecution) {
	ts.historyMutex.Lock()
	defer ts.historyMutex.Unlock()
	
	ts.executionHistory = append(ts.executionHistory, execution)
	ts.saveExecutionHistory()
}

// updateExecutionInHistory updates an execution record in history
func (ts *TaskScheduler) updateExecutionInHistory(execution TaskExecution) {
	ts.historyMutex.Lock()
	defer ts.historyMutex.Unlock()
	
	for i, existing := range ts.executionHistory {
		if existing.ID == execution.ID {
			ts.executionHistory[i] = execution
			break
		}
	}
	
	ts.saveExecutionHistory()
}

// loadExecutionHistory loads execution history from file
func (ts *TaskScheduler) loadExecutionHistory() {
	data, err := os.ReadFile(ts.historyFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			LogWarn("TASK_SCHEDULER", "Failed to load execution history", err.Error())
		}
		return
	}
	
	err = json.Unmarshal(data, &ts.executionHistory)
	if err != nil {
		LogError("TASK_SCHEDULER", "Failed to parse execution history", err.Error())
		return
	}
	
	LogInfo("TASK_SCHEDULER", "Execution history loaded", fmt.Sprintf("Records: %d", len(ts.executionHistory)))
}

// saveExecutionHistory saves execution history to file
func (ts *TaskScheduler) saveExecutionHistory() {
	data, err := json.MarshalIndent(ts.executionHistory, "", "  ")
	if err != nil {
		LogError("TASK_SCHEDULER", "Failed to marshal execution history", err.Error())
		return
	}
	
	err = os.WriteFile(ts.historyFilePath, data, 0644)
	if err != nil {
		LogError("TASK_SCHEDULER", "Failed to save execution history", err.Error())
		return
	}
}

// loadEventQueue loads event queue from file
func (ts *TaskScheduler) loadEventQueue() {
	data, err := os.ReadFile(ts.queueFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			LogWarn("TASK_SCHEDULER", "Failed to load event queue", err.Error())
		}
		return
	}
	
	err = json.Unmarshal(data, &ts.eventQueue)
	if err != nil {
		LogError("TASK_SCHEDULER", "Failed to parse event queue", err.Error())
		return
	}
	
	LogInfo("TASK_SCHEDULER", "Event queue loaded", fmt.Sprintf("Events: %d", len(ts.eventQueue)))
}

// saveEventQueue saves event queue to file
func (ts *TaskScheduler) saveEventQueue() error {
	data, err := json.MarshalIndent(ts.eventQueue, "", "  ")
	if err != nil {
		LogError("TASK_SCHEDULER", "Failed to marshal event queue", err.Error())
		return err
	}
	
	err = os.WriteFile(ts.queueFilePath, data, 0644)
	if err != nil {
		LogError("TASK_SCHEDULER", "Failed to save event queue", err.Error())
		return err
	}
	
	return nil
}
