package main

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// TemplateProcessor handles notification template processing
type TemplateProcessor struct {
	config *NotificationTemplates
}

// NewTemplateProcessor creates a new template processor
func NewTemplateProcessor(config *NotificationTemplates) *TemplateProcessor {
	LogDebug("TEMPLATE", "Creating new template processor", "")
	
	processor := &TemplateProcessor{
		config: config,
	}
	
	LogDebug("TEMPLATE", "Template processor created", fmt.Sprintf("Global template: %t, Per-service templates: %d", 
		config.Global.Title != "" || config.Global.Message != "", len(config.PerService)))
	return processor
}

// ProcessTemplate processes a notification template with variables
func (tp *TemplateProcessor) ProcessTemplate(serviceName string, variables TemplateVariables) (string, string, error) {
	LogDebug("TEMPLATE", "Processing template", fmt.Sprintf("Service: %s, Camera: %s, Object: %s", serviceName, variables.Camera, variables.Object))
	
	var title, message string
	var err error

	// Try to get service-specific template first
	if serviceTemplate, exists := tp.config.PerService[serviceName]; exists {
		LogDebug("TEMPLATE", "Using service-specific template", fmt.Sprintf("Service: %s", serviceName))
		title, err = tp.processTemplateString(serviceTemplate.Title, variables)
		if err != nil {
			LogError("TEMPLATE", "Failed to process service-specific title template", fmt.Sprintf("Service: %s, Error: %s", serviceName, err.Error()))
			return "", "", fmt.Errorf("failed to process title template for %s: %w", serviceName, err)
		}
		message, err = tp.processTemplateString(serviceTemplate.Message, variables)
		if err != nil {
			LogError("TEMPLATE", "Failed to process service-specific message template", fmt.Sprintf("Service: %s, Error: %s", serviceName, err.Error()))
			return "", "", fmt.Errorf("failed to process message template for %s: %w", serviceName, err)
		}
		LogDebug("TEMPLATE", "Service-specific template processed successfully", fmt.Sprintf("Service: %s, Title length: %d, Message length: %d", serviceName, len(title), len(message)))
		return title, message, nil
	}

	// Fall back to global template
	LogDebug("TEMPLATE", "Using global template", fmt.Sprintf("Service: %s", serviceName))
	title, err = tp.processTemplateString(tp.config.Global.Title, variables)
	if err != nil {
		LogError("TEMPLATE", "Failed to process global title template", err.Error())
		return "", "", fmt.Errorf("failed to process global title template: %w", err)
	}
	message, err = tp.processTemplateString(tp.config.Global.Message, variables)
	if err != nil {
		LogError("TEMPLATE", "Failed to process global message template", err.Error())
		return "", "", fmt.Errorf("failed to process global message template: %w", err)
	}
	LogDebug("TEMPLATE", "Global template processed successfully", fmt.Sprintf("Service: %s, Title length: %d, Message length: %d", serviceName, len(title), len(message)))
	return title, message, nil

	// Fall back to default format
	LogDebug("TEMPLATE", "Using default template format", fmt.Sprintf("Service: %s", serviceName))
	title = "FND Notification"
	message = fmt.Sprintf("Camera: %s, Object: %s, Date: %s", variables.Camera, variables.Object, variables.Date)
	if variables.HasVideo && variables.VideoURL != "" {
		message += fmt.Sprintf("\n🎥 Video: %s", variables.VideoURL)
	}
	LogDebug("TEMPLATE", "Default template processed", fmt.Sprintf("Service: %s, Title length: %d, Message length: %d", serviceName, len(title), len(message)))
	return title, message, nil
}

// processTemplateString processes a single template string
func (tp *TemplateProcessor) processTemplateString(templateStr string, variables TemplateVariables) (string, error) {
	LogDebug("TEMPLATE", "Processing template string", fmt.Sprintf("Template length: %d", len(templateStr)))
	
	if templateStr == "" {
		LogDebug("TEMPLATE", "Template string is empty", "")
		return "", nil
	}

	tmpl, err := template.New("notification").Parse(templateStr)
	if err != nil {
		LogError("TEMPLATE", "Failed to parse template string", fmt.Sprintf("Error: %s", err.Error()))
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, variables)
	if err != nil {
		LogError("TEMPLATE", "Failed to execute template", fmt.Sprintf("Error: %s", err.Error()))
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	result := buf.String()
	LogDebug("TEMPLATE", "Template string processed successfully", fmt.Sprintf("Result length: %d", len(result)))
	return result, nil
}

// GetAvailableVariables returns a list of available template variables
func GetAvailableVariables() map[string]string {
	LogDebug("TEMPLATE", "Getting available template variables", "")
	
	variables := map[string]string{
		"{{.Camera}}":      "Camera name",
		"{{.Object}}":      "Detected object",
		"{{.Date}}":        "Event date (DD.MM.YYYY)",
		"{{.Time}}":        "Event time (HH:MM:SS)",
		"{{.VideoURL}}":    "Video URL (if available)",
		"{{.HasVideo}}":    "Boolean indicating if video is available",
		"{{.EventID}}":     "Event ID from Frigate",
		"{{.HasSnapshot}}": "Boolean indicating if snapshot is available",
		"{{.SnapshotURL}}": "Snapshot attachment indicator",
	}
	
	LogDebug("TEMPLATE", "Available variables retrieved", fmt.Sprintf("Count: %d", len(variables)))
	return variables
}

// ValidateTemplate validates a template string
func ValidateTemplate(templateStr string) error {
	LogDebug("TEMPLATE", "Validating template", fmt.Sprintf("Template length: %d", len(templateStr)))
	
	if templateStr == "" {
		LogDebug("TEMPLATE", "Template is empty, validation passed", "")
		return nil
	}

	// Test with sample variables
	testVars := TemplateVariables{
		Camera:      "test_camera",
		Object:      "person",
		Date:        "01.01.2024",
		Time:        "12:00:00",
		VideoURL:    "http://example.com/video.mp4",
		HasVideo:    true,
		EventID:     "test_event_123",
		HasSnapshot: true,
		SnapshotURL: "[Snapshot attached]",
	}

	LogDebug("TEMPLATE", "Testing template syntax", "")
	_, err := template.New("test").Parse(templateStr)
	if err != nil {
		LogError("TEMPLATE", "Template syntax validation failed", fmt.Sprintf("Error: %s", err.Error()))
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	// Try to execute with test variables
	LogDebug("TEMPLATE", "Testing template execution", "")
	tmpl, _ := template.New("test").Parse(templateStr)
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, testVars)
	if err != nil {
		LogError("TEMPLATE", "Template execution validation failed", fmt.Sprintf("Error: %s", err.Error()))
		return fmt.Errorf("template execution error: %w", err)
	}

	LogDebug("TEMPLATE", "Template validation passed", fmt.Sprintf("Result length: %d", buf.Len()))
	return nil
}

// CreateTemplateVariables creates template variables from a notification
func CreateTemplateVariables(n FNDNotification, camera, object, eventID string) TemplateVariables {
	LogDebug("TEMPLATE", "Creating template variables", fmt.Sprintf("Camera: %s, Object: %s, EventID: %s", camera, object, eventID))
	
	now := time.Now()

	// Determine if snapshot is available
	hasSnapshot := len(n.JpegData) > 0
	snapshotURL := ""
	if hasSnapshot {
		// For templates, we indicate that a snapshot is available
		// The actual image data is handled separately in the notification sinks
		snapshotURL = "[Snapshot attached]"
	}

	variables := TemplateVariables{
		Camera:      camera,
		Object:      object,
		Date:        now.Format("02.01.2006"),
		Time:        now.Format("15:04:05"),
		VideoURL:    n.VideoURL,
		HasVideo:    n.HasVideo,
		EventID:     eventID,
		HasSnapshot: hasSnapshot,
		SnapshotURL: snapshotURL,
	}
	
	LogDebug("TEMPLATE", "Template variables created", fmt.Sprintf("HasVideo: %t, HasSnapshot: %t, VideoURL: %s", variables.HasVideo, variables.HasSnapshot, variables.VideoURL))
	return variables
}
