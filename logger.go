package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	LOG_FILE_NAME     = "fnd.log"
	MAX_LOG_ENTRIES   = 1000
	LOG_BUFFER_SIZE   = 100
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Component   string    `json:"component"`
	Message     string    `json:"message"`
	Details     string    `json:"details,omitempty"`
	ID          string    `json:"id"`
}

// Logger manages application logging with file storage and live updates
type Logger struct {
	logFile       *os.File
	entries       []LogEntry
	liveChannel   chan LogEntry
	subscribers   map[string]chan LogEntry
	filePath      string
	mutex         sync.RWMutex
	subscribeMutex sync.RWMutex
}

// Global logger instance
var appLogger *Logger

// InitializeLogger creates and initializes the global logger
func InitializeLogger() error {
	logPath := filepath.Join(CONFIGURATION_FOLDER, LOG_FILE_NAME)
	
	logger := &Logger{
		entries:     make([]LogEntry, 0, MAX_LOG_ENTRIES),
		liveChannel: make(chan LogEntry, LOG_BUFFER_SIZE),
		subscribers: make(map[string]chan LogEntry),
		filePath:    logPath,
	}
	
	// Create log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	logger.logFile = file
	
	// Load existing logs if file exists
	logger.loadExistingLogs()
	
	// Start background goroutine for handling live updates
	go logger.handleLiveUpdates()
	
	appLogger = logger
	
	// Log initialization
	LogInfo("SYSTEM", "Logger initialized", fmt.Sprintf("Log file: %s", logPath))
	
	return nil
}

// loadExistingLogs reads existing log entries from file
func (l *Logger) loadExistingLogs() {
	// Try to read existing log file
	if data, err := os.ReadFile(l.filePath); err == nil && len(data) > 0 {
		// Parse each line as a JSON log entry
		lines := []byte{}
		for _, b := range data {
			if b == '\n' {
				if len(lines) > 0 {
					var entry LogEntry
					if err := json.Unmarshal(lines, &entry); err == nil {
						l.entries = append(l.entries, entry)
					}
				}
				lines = []byte{}
			} else {
				lines = append(lines, b)
			}
		}
		
		// Handle last line without newline
		if len(lines) > 0 {
			var entry LogEntry
			if err := json.Unmarshal(lines, &entry); err == nil {
				l.entries = append(l.entries, entry)
			}
		}
		
		// Keep only the last MAX_LOG_ENTRIES
		if len(l.entries) > MAX_LOG_ENTRIES {
			l.entries = l.entries[len(l.entries)-MAX_LOG_ENTRIES:]
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
	
	// Add to memory
	l.entries = append(l.entries, entry)
	if len(l.entries) > MAX_LOG_ENTRIES {
		l.entries = l.entries[1:]
	}
	
	// Write to file
	if l.logFile != nil {
		jsonData, _ := json.Marshal(entry)
		l.logFile.WriteString(string(jsonData) + "\n")
		l.logFile.Sync()
	}
	
	// Send to live channel
	select {
	case l.liveChannel <- entry:
	default:
		// Skip if channel is full
	}
}

// GetRecentLogs returns recent log entries
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

// GetLogsByLevel returns logs filtered by level
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

// GetLogsByComponent returns logs filtered by component
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
