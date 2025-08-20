package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	LOG_FILE     = "fnd.log"
	MAX_LOG_SIZE = 10 * 1024 * 1024 // 10MB
)

type Logger struct {
	file *os.File
	log  *log.Logger
}

var globalLogger *Logger

func initLogger() error {
	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %v", err)
	}

	logPath := filepath.Join(logsDir, LOG_FILE)
	
	// Check if log file exists and rotate if needed
	if err := rotateLogIfNeeded(logPath); err != nil {
		return fmt.Errorf("failed to rotate log: %v", err)
	}

	// Open log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	// Create logger that writes to both file and console
	multiWriter := io.MultiWriter(os.Stdout, file)
	logger := log.New(multiWriter, "", log.LstdFlags)

	globalLogger = &Logger{
		file: file,
		log:  logger,
	}

	return nil
}

func rotateLogIfNeeded(logPath string) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, no need to rotate
		}
		return err
	}

	// Check if file size exceeds maximum
	if info.Size() > MAX_LOG_SIZE {
		// Create backup filename with timestamp
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		backupPath := logPath + "." + timestamp
		
		// Rename current log file to backup
		if err := os.Rename(logPath, backupPath); err != nil {
			return fmt.Errorf("failed to rotate log file: %v", err)
		}
	}

	return nil
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.log.Printf("[INFO] "+format, v...)
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.log.Printf("[ERROR] "+format, v...)
}

func (l *Logger) Debug(format string, v ...interface{}) {
	l.log.Printf("[DEBUG] "+format, v...)
}

func (l *Logger) Warn(format string, v ...interface{}) {
	l.log.Printf("[WARN] "+format, v...)
}

func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Convenience functions
func LogInfo(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Info(format, v...)
	}
}

func LogError(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Error(format, v...)
	}
}

func LogDebug(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Debug(format, v...)
	}
}

func LogWarn(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Warn(format, v...)
	}
}
