package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Log levels constants
const (
	LOG_LEVEL_DEBUG = "DEBUG"
	LOG_LEVEL_INFO  = "INFO"
	LOG_LEVEL_WARN  = "WARN"
	LOG_LEVEL_ERROR = "ERROR"
)

const (
	LOG_FILE_NAME   = "fnd.log"
	LOG_BUFFER_SIZE = 100
	// New constants for improved performance
	MEMORY_CACHE_SIZE = 1000 // Keep only last 1000 entries in memory
	ROTATION_SIZE_MB  = 10   // Rotate log file when it reaches 10MB
	MAX_LOG_FILES     = 5    // Keep only 5 rotated log files
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

// Logger manages application logging with file storage and live updates
type Logger struct {
	logFile        *os.File
	entries        []LogEntry
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
}

// LogStats represents logging statistics
type LogStats struct {
	EntriesInMemory   int    `json:"entriesInMemory"`
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

	logger := &Logger{
		entries:     make([]LogEntry, 0, MEMORY_CACHE_SIZE),
		liveChannel: make(chan LogEntry, LOG_BUFFER_SIZE),
		subscribers: make(map[string]chan LogEntry),
		filePath:    logPath,
		config:      config,
	}

	// Create log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	logger.logFile = file

	// Get initial file size
	if fileInfo, err := file.Stat(); err == nil {
		logger.fileSize = fileInfo.Size()
	}

	// Load recent logs only (not all)
	logger.loadRecentLogs()

	// Start background goroutine for handling live updates
	go logger.handleLiveUpdates()

	// Start background goroutine for log rotation
	go logger.rotationWorker()

	appLogger = logger

	// Log initialization
	LogInfo("SYSTEM", "Logger initialized", fmt.Sprintf("Log file: %s, Memory cache: %d entries", logPath, MEMORY_CACHE_SIZE))

	return nil
}

// loadRecentLogs reads only recent log entries from file (last 1000)
func (l *Logger) loadRecentLogs() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Try to read recent log entries from file
	if data, err := os.ReadFile(l.filePath); err == nil && len(data) > 0 {
		var allEntries []LogEntry

		// Parse each line as a JSON log entry
		scanner := bufio.NewScanner(bufio.NewReader(bufio.NewReader(strings.NewReader(string(data)))))
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				var entry LogEntry
				if err := json.Unmarshal([]byte(line), &entry); err == nil {
					allEntries = append(allEntries, entry)
				}
			}
		}

		// Keep only the last MEMORY_CACHE_SIZE entries
		if len(allEntries) > MEMORY_CACHE_SIZE {
			l.entries = allEntries[len(allEntries)-MEMORY_CACHE_SIZE:]
		} else {
			l.entries = allEntries
		}

		LogInfo("SYSTEM", "Recent logs loaded", fmt.Sprintf("Loaded %d entries from file", len(l.entries)))
	}
}

// rotationWorker handles log file rotation in background
func (l *Logger) rotationWorker() {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.checkAndRotate()
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
	l.logFile.Close()

	// Rename current file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	rotatedPath := l.filePath + "." + timestamp
	if err := os.Rename(l.filePath, rotatedPath); err != nil {
		LogError("SYSTEM", "Failed to rotate log file", err.Error())
		return
	}

	// Create new log file
	file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		LogError("SYSTEM", "Failed to create new log file", err.Error())
		return
	}
	l.logFile = file
	l.fileSize = 0

	// Clean old rotated files
	l.cleanOldLogFiles()

	LogInfo("SYSTEM", "Log file rotated", fmt.Sprintf("New file: %s", l.filePath))
}

// cleanOldLogFiles removes old rotated log files
func (l *Logger) cleanOldLogFiles() {
	dir := filepath.Dir(l.filePath)
	baseName := filepath.Base(l.filePath)

	files, err := os.ReadDir(dir)
	if err != nil {
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
			os.Remove(file)
		}
	}
}

// handleLiveUpdates processes live log updates
func (l *Logger) handleLiveUpdates() {
	for entry := range l.liveChannel {
		l.subscribeMutex.RLock()
		for _, subscriber := range l.subscribers {
			select {
			case subscriber <- entry:
			default:
				// Skip if subscriber channel is full
			}
		}
		l.subscribeMutex.RUnlock()
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

	// Add to memory (limited cache)
	l.entries = append(l.entries, entry)
	if len(l.entries) > MEMORY_CACHE_SIZE {
		l.entries = l.entries[1:]
	}

	// Write to file if enabled
	if l.config.EnableFile && l.logFile != nil {
		jsonData, _ := json.Marshal(entry)
		written, _ := l.logFile.WriteString(string(jsonData) + "\n")
		l.logFile.Sync()
		l.fileSize += int64(written)
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

// GetRecentLogs returns recent log entries from memory cache
func (l *Logger) GetRecentLogs(limit int) []LogEntry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}

	start := len(l.entries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, limit)
	copy(result, l.entries[start:])
	return result
}

// GetLogsByLevel returns logs filtered by level (from memory cache only)
func (l *Logger) GetLogsByLevel(level string, limit int) []LogEntry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	var filtered []LogEntry
	for i := len(l.entries) - 1; i >= 0 && len(filtered) < limit; i-- {
		if l.entries[i].Level == level {
			filtered = append([]LogEntry{l.entries[i]}, filtered...)
		}
	}

	return filtered
}

// GetLogsByComponent returns logs filtered by component (from memory cache only)
func (l *Logger) GetLogsByComponent(component string, limit int) []LogEntry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	var filtered []LogEntry
	for i := len(l.entries) - 1; i >= 0 && len(filtered) < limit; i-- {
		if l.entries[i].Component == component {
			filtered = append([]LogEntry{l.entries[i]}, filtered...)
		}
	}

	return filtered
}

// SearchLogs searches logs in file (for older entries not in memory)
func (l *Logger) SearchLogs(query string, level string, component string, limit int) []LogEntry {
	var results []LogEntry

	// Search in memory first
	l.mutex.RLock()
	for i := len(l.entries) - 1; i >= 0 && len(results) < limit; i-- {
		entry := l.entries[i]
		if l.matchesSearch(entry, query, level, component) {
			results = append([]LogEntry{entry}, results...)
		}
	}
	l.mutex.RUnlock()

	// If we need more results, search in file
	if len(results) < limit {
		fileResults := l.searchInFile(query, level, component, limit-len(results))
		results = append(fileResults, results...)
	}

	return results
}

// searchInFile searches logs in the log file
func (l *Logger) searchInFile(query string, level string, component string, limit int) []LogEntry {
	var results []LogEntry

	file, err := os.Open(l.filePath)
	if err != nil {
		return results
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() && len(results) < limit {
		line := scanner.Text()
		if line != "" {
			var entry LogEntry
			if err := json.Unmarshal([]byte(line), &entry); err == nil {
				if l.matchesSearch(entry, query, level, component) {
					results = append(results, entry)
				}
			}
		}
	}

	return results
}

// matchesSearch checks if a log entry matches search criteria
func (l *Logger) matchesSearch(entry LogEntry, query string, level string, component string) bool {
	// Check level filter
	if level != "" && entry.Level != level {
		return false
	}

	// Check component filter
	if component != "" && entry.Component != component {
		return false
	}

	// Check query (search in message and details)
	if query != "" {
		queryLower := strings.ToLower(query)
		messageLower := strings.ToLower(entry.Message)
		detailsLower := strings.ToLower(entry.Details)

		if !strings.Contains(messageLower, queryLower) && !strings.Contains(detailsLower, queryLower) {
			return false
		}
	}

	return true
}

// GetLogStats returns comprehensive logging statistics
func (l *Logger) GetLogStats() LogStats {
	l.mutex.RLock()
	entriesInMemory := len(l.entries)
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

	// Estimate memory usage
	estimatedSize := entriesInMemory * 200 // rough estimate per log entry
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
		EntriesInMemory:   entriesInMemory,
		FileSizeHuman:     fileSizeHuman,
		ActiveSubscribers: activeSubscribers,
		MemoryUsageHuman:  memoryUsageHuman,
		TotalLogFiles:     totalLogFiles,
		LastRotation:      time.Now().Format("2006-01-02 15:04:05"),
	}
}

// UpdateConfiguration updates the logger configuration
func (l *Logger) UpdateConfiguration(config *FNDLoggingConfiguration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.config = config
}

// GetConfiguration returns the current logger configuration
func (l *Logger) GetConfiguration() *FNDLoggingConfiguration {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *l.config
	return &configCopy
}

// Close properly closes the logger
func (l *Logger) Close() {
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
		LogInfo("SYSTEM", "Logger shutting down", "")
		appLogger.Close()
		appLogger = nil
	}
}
