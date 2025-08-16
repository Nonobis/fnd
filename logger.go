package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Log levels constants are now defined in constants.go

const (
	LOG_FILE_NAME   = "fnd.log"
	LOG_BUFFER_SIZE = 100
	// New constants for improved performance
	INDEX_CACHE_SIZE = 10000 // Keep index of last 10k entries
	ROTATION_SIZE_MB = 10    // Rotate log file when it reaches 10MB
	MAX_LOG_FILES    = 5     // Keep only 5 rotated log files
	// Cache settings
	RECENT_LOGS_CACHE_SIZE = 100 // Cache only last 100 entries in memory
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
	ID        string    `json:"id"`
}

// LogIndex represents a lightweight index entry for efficient searching
type LogIndex struct {
	Timestamp  time.Time `json:"timestamp"`
	FileOffset int64     `json:"fileOffset"`
	Level      string    `json:"level"`
	Component  string    `json:"component"`
	LineLength int       `json:"lineLength"`
	LineNumber int       `json:"lineNumber"`
}

// Logger manages application logging with file storage and live updates
type Logger struct {
	logFile        *os.File
	index          []LogIndex // Lightweight index instead of full entries
	recentCache    []LogEntry // Small cache for recent entries only
	liveChannel    chan LogEntry
	subscribers    map[string]chan LogEntry
	filePath       string
	config         *FNDLoggingConfiguration
	mutex          sync.RWMutex
	subscribeMutex sync.RWMutex
	// New fields for improved performance
	fileSize      int64
	rotationMutex sync.Mutex
	stats         LogStats
	// Shutdown management
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownWg   sync.WaitGroup
	lastRotation time.Time
	// Index management
	indexMutex    sync.RWMutex
	lastIndexSize int64
}

// LogStats represents logging statistics
type LogStats struct {
	IndexEntries      int    `json:"indexEntries"`
	RecentCacheSize   int    `json:"recentCacheSize"`
	FileSizeHuman     string `json:"fileSizeHuman"`
	ActiveSubscribers int    `json:"activeSubscribers"`
	MemoryUsageHuman  string `json:"memoryUsageHuman"`
	TotalLogFiles     int    `json:"totalLogFiles"`
	LastRotation      string `json:"lastRotation"`
}

// Global logger instance
var appLogger *Logger

// InitializeLogger creates and initializes the global logger
func InitializeLogger(config *FNDLoggingConfiguration) error {
	logPath := filepath.Join(CONFIGURATION_FOLDER, LOG_FILE_NAME)

	ctx, cancel := context.WithCancel(context.Background())
	logger := &Logger{
		index:        make([]LogIndex, 0, INDEX_CACHE_SIZE),
		recentCache:  make([]LogEntry, 0, RECENT_LOGS_CACHE_SIZE),
		liveChannel:  make(chan LogEntry, LOG_BUFFER_SIZE),
		subscribers:  make(map[string]chan LogEntry),
		filePath:     logPath,
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
		lastRotation: time.Now(),
	}

	// Create log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		CaptureError(err, map[string]interface{}{
			"component": "logger",
			"action":    "create_log_file",
			"log_path":  logPath,
		})
		return fmt.Errorf("failed to create log file: %w", err)
	}
	logger.logFile = file

	// Get initial file size
	if fileInfo, err := file.Stat(); err == nil {
		logger.fileSize = fileInfo.Size()
	}

	// Build index from existing log file
	logger.buildIndex()

	// Start background goroutines
	logger.shutdownWg.Add(2)
	go logger.handleLiveUpdates()
	go logger.rotationWorker()

	appLogger = logger

	// Log initialization
	LogInfo(COMPONENT_SYSTEM, "Logger initialized", fmt.Sprintf("Log file: %s, Index entries: %d", logPath, len(logger.index)))

	return nil
}

// buildIndex creates a lightweight index of log entries from file
func (l *Logger) buildIndex() {
	l.indexMutex.Lock()
	defer l.indexMutex.Unlock()

	// Clear existing index
	l.index = make([]LogIndex, 0, INDEX_CACHE_SIZE)

	// Try to read log entries from file and build index
	file, err := os.Open(l.filePath)
	if err != nil {
		// File doesn't exist yet, which is normal for new installations
		return
	}
	defer file.Close()

	var offset int64
	lineNumber := 0
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			var entry LogEntry
			if err := json.Unmarshal([]byte(line), &entry); err == nil {
				// Create lightweight index entry
				indexEntry := LogIndex{
					Timestamp:  entry.Timestamp,
					FileOffset: offset,
					Level:      entry.Level,
					Component:  entry.Component,
					LineLength: len(line),
					LineNumber: lineNumber,
				}
				l.index = append(l.index, indexEntry)

				// Keep only recent entries in memory cache
				if len(l.recentCache) < RECENT_LOGS_CACHE_SIZE {
					l.recentCache = append(l.recentCache, entry)
				} else {
					// Replace oldest entry
					l.recentCache = append(l.recentCache[1:], entry)
				}
			}
		}
		offset += int64(len(line) + 1) // +1 for newline
		lineNumber++
	}

	// Limit index size to prevent memory bloat
	if len(l.index) > INDEX_CACHE_SIZE {
		l.index = l.index[len(l.index)-INDEX_CACHE_SIZE:]
	}

	l.lastIndexSize = l.fileSize
	LogInfo(COMPONENT_SYSTEM, "Log index built", fmt.Sprintf("Index entries: %d, Recent cache: %d", len(l.index), len(l.recentCache)))
}

// readLogEntryAtOffset reads a specific log entry from file at given offset
func (l *Logger) readLogEntryAtOffset(offset int64) (*LogEntry, error) {
	file, err := os.Open(l.filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Seek to offset
	if _, err := file.Seek(offset, 0); err != nil {
		return nil, err
	}

	// Read one line
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			var entry LogEntry
			if err := json.Unmarshal([]byte(line), &entry); err == nil {
				return &entry, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to read log entry at offset %d", offset)
}

// rotationWorker handles log file rotation in background
func (l *Logger) rotationWorker() {
	defer l.shutdownWg.Done()

	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.checkAndRotate()
		case <-l.ctx.Done():
			return
		}
	}
}

// checkAndRotate checks if log file needs rotation
func (l *Logger) checkAndRotate() {
	l.rotationMutex.Lock()
	defer l.rotationMutex.Unlock()

	// Check current file size
	if fileInfo, err := l.logFile.Stat(); err == nil {
		currentSize := fileInfo.Size()
		if currentSize > int64(ROTATION_SIZE_MB*1024*1024) {
			l.rotateLogFile()
		}
	}
}

// rotateLogFile rotates the current log file
func (l *Logger) rotateLogFile() {
	// Close current file
	if err := l.logFile.Close(); err != nil {
		LogError(COMPONENT_SYSTEM, "Failed to close log file during rotation", err.Error())
		return
	}

	// Rename current file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	rotatedPath := l.filePath + "." + timestamp
	if err := os.Rename(l.filePath, rotatedPath); err != nil {
		LogError(COMPONENT_SYSTEM, "Failed to rotate log file", err.Error())
		return
	}

	// Create new log file
	file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		LogError(COMPONENT_SYSTEM, "Failed to create new log file", err.Error())
		return
	}
	l.logFile = file
	l.fileSize = 0
	l.lastRotation = time.Now()

	// Clean old rotated files
	l.cleanOldLogFiles()

	// Rebuild index for new file
	l.buildIndex()

	LogInfo(COMPONENT_SYSTEM, "Log file rotated", fmt.Sprintf("New file: %s", l.filePath))
}

// cleanOldLogFiles removes old rotated log files
func (l *Logger) cleanOldLogFiles() {
	dir := filepath.Dir(l.filePath)
	baseName := filepath.Base(l.filePath)

	files, err := os.ReadDir(dir)
	if err != nil {
		LogWarn(COMPONENT_SYSTEM, "Failed to read directory for log cleanup", err.Error())
		return
	}

	var rotatedFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), baseName+".") {
			rotatedFiles = append(rotatedFiles, filepath.Join(dir, file.Name()))
		}
	}

	// Sort by modification time (oldest first)
	sort.Slice(rotatedFiles, func(i, j int) bool {
		infoI, _ := os.Stat(rotatedFiles[i])
		infoJ, _ := os.Stat(rotatedFiles[j])
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Remove oldest files if we have too many
	if len(rotatedFiles) > MAX_LOG_FILES {
		for _, file := range rotatedFiles[:len(rotatedFiles)-MAX_LOG_FILES] {
			if err := os.Remove(file); err != nil {
				LogWarn(COMPONENT_SYSTEM, "Failed to remove old log file", fmt.Sprintf("File: %s, Error: %s", file, err.Error()))
			}
		}
	}
}

// handleLiveUpdates processes live log updates
func (l *Logger) handleLiveUpdates() {
	defer l.shutdownWg.Done()

	for {
		select {
		case entry, ok := <-l.liveChannel:
			if !ok {
				return // Channel closed
			}
			l.subscribeMutex.RLock()
			for _, subscriber := range l.subscribers {
				select {
				case subscriber <- entry:
				default:
					// Skip if subscriber channel is full
				}
			}
			l.subscribeMutex.RUnlock()
		case <-l.ctx.Done():
			return
		}
	}
}

// Subscribe adds a new subscriber for live log updates
func (l *Logger) Subscribe(id string) chan LogEntry {
	l.subscribeMutex.Lock()
	defer l.subscribeMutex.Unlock()

	ch := make(chan LogEntry, LOG_BUFFER_SIZE)
	l.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber
func (l *Logger) Unsubscribe(id string) {
	l.subscribeMutex.Lock()
	defer l.subscribeMutex.Unlock()

	if ch, exists := l.subscribers[id]; exists {
		close(ch)
		delete(l.subscribers, id)
	}
}

// addLogEntry adds a new log entry
func (l *Logger) addLogEntry(level, component, message, details string) {
	// Check if log level should be filtered
	if !l.shouldLog(level) {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Component: component,
		Message:   message,
		Details:   details,
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	// Add to recent cache (limited size)
	l.recentCache = append(l.recentCache, entry)
	if len(l.recentCache) > RECENT_LOGS_CACHE_SIZE {
		l.recentCache = l.recentCache[1:]
	}

	// Write to file if enabled
	if l.config.EnableFile && l.logFile != nil {
		jsonData, _ := json.Marshal(entry)
		written, _ := l.logFile.WriteString(string(jsonData) + "\n")
		l.logFile.Sync()
		l.fileSize += int64(written)

		// Add to index
		l.indexMutex.Lock()
		indexEntry := LogIndex{
			Timestamp:  entry.Timestamp,
			FileOffset: l.fileSize - int64(written+1), // +1 for newline
			Level:      entry.Level,
			Component:  entry.Component,
			LineLength: len(jsonData),
			LineNumber: len(l.index),
		}
		l.index = append(l.index, indexEntry)

		// Limit index size
		if len(l.index) > INDEX_CACHE_SIZE {
			l.index = l.index[1:]
		}
		l.indexMutex.Unlock()
	}

	// Write to console if enabled
	if l.config.EnableConsole {
		fmt.Printf("[%s] %s - %s: %s\n",
			entry.Timestamp.Format("15:04:05"),
			entry.Level,
			entry.Component,
			entry.Message)
		if entry.Details != "" {
			fmt.Printf("  Details: %s\n", entry.Details)
		}
	}

	// Send to live channel
	select {
	case l.liveChannel <- entry:
	default:
		// Skip if channel is full
	}
}

// shouldLog determines if a log entry should be recorded based on the configured log level
func (l *Logger) shouldLog(level string) bool {
	levelValue := l.getLevelValue(level)
	// Log levels: DEBUG=0, INFO=1, WARN=2, ERROR=3
	// We want to show logs with level >= configured level
	// So DEBUG(0) shows all, INFO(1) shows INFO+, WARN(2) shows WARN+, ERROR(3) shows only ERROR

	return levelValue >= l.config.LogLevel
}

// getLevelValue converts string level to numeric value
func (l *Logger) getLevelValue(level string) int {
	switch level {
	case LOG_LEVEL_DEBUG:
		return LOG_LEVEL_DEBUG_VALUE
	case LOG_LEVEL_INFO:
		return LOG_LEVEL_INFO_VALUE
	case LOG_LEVEL_WARN:
		return LOG_LEVEL_WARN_VALUE
	case LOG_LEVEL_ERROR:
		return LOG_LEVEL_ERROR_VALUE
	default:
		return LOG_LEVEL_INFO_VALUE
	}
}

// GetRecentLogs returns recent log entries from cache
func (l *Logger) GetRecentLogs(limit int) []LogEntry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if limit <= 0 || limit > len(l.recentCache) {
		limit = len(l.recentCache)
	}

	start := len(l.recentCache) - limit
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, limit)
	copy(result, l.recentCache[start:])
	return result
}

// GetLogsByLevel returns logs filtered by level using index
func (l *Logger) GetLogsByLevel(level string, limit int) []LogEntry {
	l.indexMutex.RLock()
	defer l.indexMutex.RUnlock()

	var results []LogEntry
	count := 0

	// Search from most recent to oldest
	for i := len(l.index) - 1; i >= 0 && count < limit; i-- {
		if l.index[i].Level == level {
			if entry, err := l.readLogEntryAtOffset(l.index[i].FileOffset); err == nil {
				results = append([]LogEntry{*entry}, results...)
				count++
			}
		}
	}

	return results
}

// GetLogsByComponent returns logs filtered by component using index
func (l *Logger) GetLogsByComponent(component string, limit int) []LogEntry {
	l.indexMutex.RLock()
	defer l.indexMutex.RUnlock()

	var results []LogEntry
	count := 0

	// Search from most recent to oldest
	for i := len(l.index) - 1; i >= 0 && count < limit; i-- {
		if l.index[i].Component == component {
			if entry, err := l.readLogEntryAtOffset(l.index[i].FileOffset); err == nil {
				results = append([]LogEntry{*entry}, results...)
				count++
			}
		}
	}

	return results
}

// SearchLogs searches logs using index for efficient filtering
func (l *Logger) SearchLogs(query string, level string, component string, limit int) []LogEntry {
	l.indexMutex.RLock()
	defer l.indexMutex.RUnlock()

	var results []LogEntry
	count := 0

	// Search from most recent to oldest
	for i := len(l.index) - 1; i >= 0 && count < limit; i-- {
		indexEntry := l.index[i]

		// Apply filters
		if level != "" && indexEntry.Level != level {
			continue
		}
		if component != "" && indexEntry.Component != component {
			continue
		}

		// Read entry and check query
		if entry, err := l.readLogEntryAtOffset(indexEntry.FileOffset); err == nil {
			if query == "" || l.matchesSearch(*entry, query) {
				results = append([]LogEntry{*entry}, results...)
				count++
			}
		}
	}

	return results
}

// matchesSearch checks if a log entry matches search criteria
func (l *Logger) matchesSearch(entry LogEntry, query string) bool {
	if query == "" {
		return true
	}

	queryLower := strings.ToLower(query)
	messageLower := strings.ToLower(entry.Message)
	detailsLower := strings.ToLower(entry.Details)

	return strings.Contains(messageLower, queryLower) || strings.Contains(detailsLower, queryLower)
}

// GetLogStats returns comprehensive logging statistics
func (l *Logger) GetLogStats() LogStats {
	l.indexMutex.RLock()
	indexEntries := len(l.index)
	l.indexMutex.RUnlock()

	l.mutex.RLock()
	recentCacheSize := len(l.recentCache)
	l.mutex.RUnlock()

	l.subscribeMutex.RLock()
	activeSubscribers := len(l.subscribers)
	l.subscribeMutex.RUnlock()

	// Calculate file size
	var fileSizeHuman string
	if fileInfo, err := os.Stat(l.filePath); err == nil {
		size := fileInfo.Size()
		if size < 1024 {
			fileSizeHuman = fmt.Sprintf("%d B", size)
		} else if size < 1024*1024 {
			fileSizeHuman = fmt.Sprintf("%.1f KB", float64(size)/1024)
		} else {
			fileSizeHuman = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
		}
	} else {
		fileSizeHuman = "N/A"
	}

	// Estimate memory usage (much smaller now)
	estimatedSize := indexEntries*50 + recentCacheSize*200 // Index entries are much smaller
	var memoryUsageHuman string
	if estimatedSize < 1024 {
		memoryUsageHuman = fmt.Sprintf("%d B", estimatedSize)
	} else if estimatedSize < 1024*1024 {
		memoryUsageHuman = fmt.Sprintf("%.1f KB", float64(estimatedSize)/1024)
	} else {
		memoryUsageHuman = fmt.Sprintf("%.1f MB", float64(estimatedSize)/(1024*1024))
	}

	// Count rotated log files
	dir := filepath.Dir(l.filePath)
	baseName := filepath.Base(l.filePath)
	files, _ := os.ReadDir(dir)
	totalLogFiles := 1 // Current file
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), baseName+".") {
			totalLogFiles++
		}
	}

	return LogStats{
		IndexEntries:      indexEntries,
		RecentCacheSize:   recentCacheSize,
		FileSizeHuman:     fileSizeHuman,
		ActiveSubscribers: activeSubscribers,
		MemoryUsageHuman:  memoryUsageHuman,
		TotalLogFiles:     totalLogFiles,
		LastRotation:      l.lastRotation.Format("2006-01-02 15:04:05"),
	}
}

// UpdateConfiguration updates the logger configuration
func (l *Logger) UpdateConfiguration(config *FNDLoggingConfiguration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	oldLevel := l.config.LogLevel
	oldMaxEntries := l.config.MaxEntries

	l.config = config

	// Log the configuration change using the logging system
	LogInfo(COMPONENT_LOGGER, "Configuration updated", fmt.Sprintf("Level changed from %d to %d, MaxEntries changed from %d to %d",
		oldLevel, config.LogLevel, oldMaxEntries, config.MaxEntries))

	// Test if the new level is working
	LogDebug(COMPONENT_LOGGER, "This is a test DEBUG message after configuration update", "")
}

// GetConfiguration returns a copy of the current logger configuration
func (l *Logger) GetConfiguration() *FNDLoggingConfiguration {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Return a copy to prevent external modification
	configCopy := *l.config
	return &configCopy
}

// Close properly closes the logger
func (l *Logger) Close() {
	// Signal shutdown to background goroutines
	l.cancel()

	// Wait for background goroutines to finish
	l.shutdownWg.Wait()

	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.logFile != nil {
		l.logFile.Close()
	}

	close(l.liveChannel)

	l.subscribeMutex.Lock()
	for id, ch := range l.subscribers {
		close(ch)
		delete(l.subscribers, id)
	}
	l.subscribeMutex.Unlock()
}

// Public logging functions
func LogDebug(component, message string, details ...string) {
	if appLogger != nil {
		detail := ""
		if len(details) > 0 {
			detail = details[0]
		}
		appLogger.addLogEntry(LOG_LEVEL_DEBUG, component, message, detail)
	}
}

func LogInfo(component, message string, details ...string) {
	if appLogger != nil {
		detail := ""
		if len(details) > 0 {
			detail = details[0]
		}
		appLogger.addLogEntry(LOG_LEVEL_INFO, component, message, detail)
	}
}

func LogWarn(component, message string, details ...string) {
	if appLogger != nil {
		detail := ""
		if len(details) > 0 {
			detail = details[0]
		}
		appLogger.addLogEntry(LOG_LEVEL_WARN, component, message, detail)
	}
}

func LogError(component, message string, details ...string) {
	if appLogger != nil {
		detail := ""
		if len(details) > 0 {
			detail = details[0]
		}
		appLogger.addLogEntry(LOG_LEVEL_ERROR, component, message, detail)
	}
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	return appLogger
}

// CloseLogger properly closes the global logger
func CloseLogger() {
	if appLogger != nil {
		LogInfo(COMPONENT_SYSTEM, "Logger shutting down", "")
		appLogger.Close()
		appLogger = nil
	}
}
