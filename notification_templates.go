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
	return &TemplateProcessor{
		config: config,
	}
}

// ProcessTemplate processes a notification template with variables
func (tp *TemplateProcessor) ProcessTemplate(serviceName string, variables TemplateVariables) (string, string, error) {
	var title, message string
	var err error

	// Try to get service-specific template first
	if serviceTemplate, exists := tp.config.PerService[serviceName]; exists {
		title, err = tp.processTemplateString(serviceTemplate.Title, variables)
		if err != nil {
			return "", "", fmt.Errorf("failed to process title template for %s: %w", serviceName, err)
		}
		message, err = tp.processTemplateString(serviceTemplate.Message, variables)
		if err != nil {
			return "", "", fmt.Errorf("failed to process message template for %s: %w", serviceName, err)
		}
		return title, message, nil
	}

	// Fall back to global template
	title, err = tp.processTemplateString(tp.config.Global.Title, variables)
	if err != nil {
		return "", "", fmt.Errorf("failed to process global title template: %w", err)
	}
	message, err = tp.processTemplateString(tp.config.Global.Message, variables)
	if err != nil {
		return "", "", fmt.Errorf("failed to process global message template: %w", err)
	}
	return title, message, nil

	// Fall back to default format
	title = "FND Notification"
	message = fmt.Sprintf("Camera: %s, Object: %s, Date: %s", variables.Camera, variables.Object, variables.Date)
	if variables.HasVideo && variables.VideoURL != "" {
		message += fmt.Sprintf("\n🎥 Video: %s", variables.VideoURL)
	}
	return title, message, nil
}

// processTemplateString processes a single template string
func (tp *TemplateProcessor) processTemplateString(templateStr string, variables TemplateVariables) (string, error) {
	if templateStr == "" {
		return "", nil
	}

	tmpl, err := template.New("notification").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, variables)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GetAvailableVariables returns a list of available template variables
func GetAvailableVariables() map[string]string {
	return map[string]string{
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
}

// ValidateTemplate validates a template string
func ValidateTemplate(templateStr string) error {
	if templateStr == "" {
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

	_, err := template.New("test").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	// Try to execute with test variables
	tmpl, _ := template.New("test").Parse(templateStr)
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, testVars)
	if err != nil {
		return fmt.Errorf("template execution error: %w", err)
	}

	return nil
}

// CreateTemplateVariables creates template variables from a notification
func CreateTemplateVariables(n FNDNotification, camera, object, eventID string) TemplateVariables {
	now := time.Now()

	// Determine if snapshot is available
	hasSnapshot := len(n.JpegData) > 0
	snapshotURL := ""
	if hasSnapshot {
		// For templates, we indicate that a snapshot is available
		// The actual image data is handled separately in the notification sinks
		snapshotURL = "[Snapshot attached]"
	}

	return TemplateVariables{
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
}
