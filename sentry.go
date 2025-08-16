package main

import (
	"fmt"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

// SentryConfig represents Sentry configuration
type SentryConfig struct {
	Enabled     bool   `json:"enabled"`
	DSN         string `json:"dsn"`
	Environment string `json:"environment"`
	Debug       bool   `json:"debug"`
}

// InitializeSentry initializes Sentry for error tracking
func InitializeSentry(config SentryConfig) error {
	if !config.Enabled {
		LogInfo("SENTRY", "Sentry disabled", "")
		return nil
	}

	if config.DSN == "" {
		LogWarn("SENTRY", "Sentry enabled but no DSN provided", "")
		return fmt.Errorf("Sentry enabled but no DSN provided")
	}

	// Configure Sentry
	err := sentry.Init(sentry.ClientOptions{
		Dsn:                config.DSN,
		Environment:        config.Environment,
		Debug:              config.Debug,
		TracesSampleRate:   0.1, // Sample 10% of transactions
		ProfilesSampleRate: 0.1, // Sample 10% of profiles
		AttachStacktrace:   true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Add custom context
			event.Tags["application"] = "FND"
			event.Tags["version"] = "0.1.12"

			// Filter out certain errors if needed
			if event.Exception != nil {
				for _, exception := range event.Exception {
					// Skip certain types of errors
					if exception.Type == "context.Canceled" ||
						exception.Type == "context.DeadlineExceeded" {
						return nil
					}
				}
			}

			return event
		},
	})

	if err != nil {
		LogError("SENTRY", "Failed to initialize Sentry", err.Error())
		return err
	}

	// Set up global error handler
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("application", "FND")
		scope.SetTag("version", "0.1.12")
		// scope.SetContext("system", map[string]interface{}{
		// 	"os":      "linux",
		// 	"runtime": "go",
		// })
	})

	LogInfo("SENTRY", "Sentry initialized successfully", fmt.Sprintf("Environment: %s, Debug: %t", config.Environment, config.Debug))
	return nil
}

// CloseSentry properly closes Sentry
func CloseSentry() {
	if sentry.CurrentHub().Client() != nil {
		// Flush any pending events
		if !sentry.Flush(2 * time.Second) {
			LogWarn("SENTRY", "Sentry flush timeout", "")
		}
		LogInfo("SENTRY", "Sentry closed", "")
	}
}

// CaptureError captures an error with Sentry
func CaptureError(err error, context map[string]interface{}) {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	// Add context if provided
	if context != nil {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			for key, value := range context {
				scope.SetContext(key, sentry.Context{
					"value": value,
				})
			}
		})
	}

	sentry.CaptureException(err)
}

// CaptureMessage captures a message with Sentry
func CaptureMessage(message string, level sentry.Level, context map[string]interface{}) {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	// Add context if provided
	if context != nil {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			for key, value := range context {
				scope.SetContext(key, sentry.Context{
					"value": value,
				})
			}
		})
	}

	sentry.CaptureMessage(message)
}

// CaptureException captures an exception with Sentry
func CaptureException(exception error, context map[string]interface{}) {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	// Add context if provided
	if context != nil {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			for key, value := range context {
				scope.SetContext(key, map[string]interface{}{"value": value})
			}
		})
	}

	sentry.CaptureException(exception)
}

// SetUser sets user context for Sentry
func SetSentryUser(userID, email, username string) {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetUser(sentry.User{
			ID:       userID,
			Email:    email,
			Username: username,
		})
	})
}

// AddSentryBreadcrumb adds a breadcrumb for debugging
func AddSentryBreadcrumb(message, category string, level sentry.Level, data map[string]interface{}) {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	breadcrumb := &sentry.Breadcrumb{
		Message:   message,
		Category:  category,
		Level:     level,
		Data:      data,
		Timestamp: time.Now(),
	}

	sentry.AddBreadcrumb(breadcrumb)
}

// GetSentryDSNFromEnv gets Sentry DSN from environment variable
func GetSentryDSNFromEnv() string {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn != "" {
		LogInfo("SENTRY", "DSN found in environment variable", "SENTRY_DSN is set")
	} else {
		LogDebug("SENTRY", "DSN not found in environment variable", "SENTRY_DSN is not set")
	}
	return dsn
}

// GetSentryEnvironmentFromEnv gets Sentry environment from environment variable
func GetSentryEnvironmentFromEnv() string {
	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env != "" {
		LogInfo("SENTRY", "Environment found in environment variable", fmt.Sprintf("SENTRY_ENVIRONMENT=%s", env))
	} else {
		env = "production"
		LogDebug("SENTRY", "Environment not found in environment variable, using default", fmt.Sprintf("SENTRY_ENVIRONMENT=%s", env))
	}
	return env
}

// SetupPanicHandler sets up a panic handler that sends to Sentry
func SetupPanicHandler() {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	// Set up panic handler
	defer func() {
		if err := recover(); err != nil {
			LogError("SYSTEM", "Panic recovered", fmt.Sprintf("%v", err))

			// Capture panic in Sentry
			sentry.CurrentHub().Recover(err)

			// Flush Sentry before exiting
			sentry.Flush(2 * time.Second)

			// Re-panic to maintain original behavior
			panic(err)
		}
	}()
}

// LogWithSentry logs a message and optionally sends to Sentry
func LogWithSentry(level, component, message, details string) {
	// Always log locally
	switch level {
	case LOG_LEVEL_DEBUG:
		LogDebug(component, message, details)
	case LOG_LEVEL_INFO:
		LogInfo(component, message, details)
	case LOG_LEVEL_WARN:
		LogWarn(component, message, details)
		// Send warnings to Sentry
		if sentry.CurrentHub().Client() != nil {
			CaptureMessage(fmt.Sprintf("[%s] %s: %s", component, message, details), sentry.LevelWarning, map[string]interface{}{
				"component": component,
				"details":   details,
			})
		}
	case LOG_LEVEL_ERROR:
		LogError(component, message, details)
		// Send errors to Sentry
		if sentry.CurrentHub().Client() != nil {
			CaptureMessage(fmt.Sprintf("[%s] %s: %s", component, message, details), sentry.LevelError, map[string]interface{}{
				"component": component,
				"details":   details,
			})
		}
	}
}
