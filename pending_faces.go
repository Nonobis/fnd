package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PendingFaceEvent represents a pending face event waiting for analysis
type PendingFaceEvent struct {
	ID          string    `json:"id"`
	EventID     string    `json:"eventId"`
	Camera      string    `json:"camera"`
	ImagePath   string    `json:"imagePath"`
	Timestamp   time.Time `json:"timestamp"`
	Processed   bool      `json:"processed"`
	ProcessedAt time.Time `json:"processedAt,omitempty"`
	Notes       string    `json:"notes,omitempty"`
}

// PendingFacesManager manages pending face events for later analysis
type PendingFacesManager struct {
	config           *FNDFacialRecognitionConfiguration
	pendingEvents    []PendingFaceEvent
	pendingEventsPath string
	m                sync.RWMutex
}

// NewPendingFacesManager creates a new pending faces manager
func NewPendingFacesManager(config *FNDFacialRecognitionConfiguration) *PendingFacesManager {
	manager := &PendingFacesManager{
		config:           config,
		pendingEvents:    make([]PendingFaceEvent, 0),
		pendingEventsPath: "fnd_conf/pending_events.json",
	}

	// Load existing pending events
	if err := manager.loadPendingEvents(); err != nil {
		LogWarn("PENDING_FACES", "Failed to load pending events", err.Error())
	}

	LogInfo("PENDING_FACES", "Pending faces manager created", fmt.Sprintf("Storage path: %s", manager.pendingEventsPath))
	return manager
}

// StorePersonEvent stores a person event image for later facial analysis
func (m *PendingFacesManager) StorePersonEvent(eventID, camera string, imageData []byte) error {
	if m.config.Enabled {
		LogDebug("PENDING_FACES", "Facial recognition enabled, skipping pending storage", fmt.Sprintf("Event ID: %s", eventID))
		return nil
	}

	LogInfo("PENDING_FACES", "Storing person event for later analysis", fmt.Sprintf("Event ID: %s, Camera: %s", eventID, camera))

	m.m.Lock()
	defer m.m.Unlock()

	// Generate unique ID for pending event
	pendingID := uuid.New().String()

	// Create organized directory structure: pending_faces/camera/YYYY-MM-DD/
	now := time.Now()
	dateDir := now.Format("2006-01-02")
	imageDir := filepath.Join("fnd_conf", "pending_faces", camera, dateDir)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		LogError("PENDING_FACES", "Failed to create directory", err.Error())
		return err
	}

	// Save image with timestamp
	imageFileName := fmt.Sprintf("%s_%s.jpg", now.Format("15-04-05"), pendingID)
	imagePath := filepath.Join(imageDir, imageFileName)

	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		LogError("PENDING_FACES", "Failed to save image", err.Error())
		return err
	}

	// Create pending event record
	pendingEvent := PendingFaceEvent{
		ID:        pendingID,
		EventID:   eventID,
		Camera:    camera,
		ImagePath: imagePath,
		Timestamp: now,
		Processed: false,
	}

	// Add to pending events list
	m.pendingEvents = append(m.pendingEvents, pendingEvent)

	// Save to file
	if err := m.savePendingEvents(); err != nil {
		LogError("PENDING_FACES", "Failed to save pending events", err.Error())
		return err
	}

	LogInfo("PENDING_FACES", "Person event stored successfully", fmt.Sprintf("Event ID: %s, Pending ID: %s, Image: %s", eventID, pendingID, imagePath))
	return nil
}

// GetPendingEvents returns all pending face events
func (m *PendingFacesManager) GetPendingEvents() []PendingFaceEvent {
	m.m.RLock()
	defer m.m.RUnlock()

	events := make([]PendingFaceEvent, len(m.pendingEvents))
	copy(events, m.pendingEvents)
	return events
}

// GetPendingEventsByCamera returns pending events for a specific camera
func (m *PendingFacesManager) GetPendingEventsByCamera(camera string) []PendingFaceEvent {
	m.m.RLock()
	defer m.m.RUnlock()

	var events []PendingFaceEvent
	for _, event := range m.pendingEvents {
		if event.Camera == camera && !event.Processed {
			events = append(events, event)
		}
	}
	return events
}

// GetPendingEventByID returns a specific pending event by ID
func (m *PendingFacesManager) GetPendingEventByID(pendingID string) *PendingFaceEvent {
	m.m.RLock()
	defer m.m.RUnlock()

	for _, event := range m.pendingEvents {
		if event.ID == pendingID {
			return &event
		}
	}
	return nil
}

// MarkEventAsProcessed marks a pending event as processed
func (m *PendingFacesManager) MarkEventAsProcessed(pendingID string, notes string) error {
	m.m.Lock()
	defer m.m.Unlock()

	for i, event := range m.pendingEvents {
		if event.ID == pendingID {
			m.pendingEvents[i].Processed = true
			m.pendingEvents[i].ProcessedAt = time.Now()
			m.pendingEvents[i].Notes = notes

			if err := m.savePendingEvents(); err != nil {
				LogError("PENDING_FACES", "Failed to save pending events", err.Error())
				return err
			}

			LogInfo("PENDING_FACES", "Event marked as processed", fmt.Sprintf("Pending ID: %s, Notes: %s", pendingID, notes))
			return nil
		}
	}

	return fmt.Errorf("pending event with ID %s not found", pendingID)
}

// DeletePendingEvent deletes a pending event and its image
func (m *PendingFacesManager) DeletePendingEvent(pendingID string) error {
	m.m.Lock()
	defer m.m.Unlock()

	for i, event := range m.pendingEvents {
		if event.ID == pendingID {
			// Remove from slice
			m.pendingEvents = append(m.pendingEvents[:i], m.pendingEvents[i+1:]...)

			// Delete image file
			if event.ImagePath != "" {
				if err := os.Remove(event.ImagePath); err != nil {
					LogWarn("PENDING_FACES", "Failed to delete image file", err.Error())
				} else {
					// Try to remove empty directories
					m.cleanupEmptyDirectories(event.ImagePath)
				}
			}

			// Save to file
			if err := m.savePendingEvents(); err != nil {
				LogError("PENDING_FACES", "Failed to save pending events", err.Error())
				return err
			}

			LogInfo("PENDING_FACES", "Pending event deleted", fmt.Sprintf("Pending ID: %s", pendingID))
			return nil
		}
	}

	return fmt.Errorf("pending event with ID %s not found", pendingID)
}

// ProcessAllPendingEventsWithAI processes all pending events with CodeProject.AI
func (m *PendingFacesManager) ProcessAllPendingEventsWithAI(facialRecognitionService *FacialRecognitionService) (int, int, error) {
	LogInfo("PENDING_FACES", "Starting automatic processing of all pending events", "")
	
	m.m.RLock()
	pendingEvents := make([]PendingFaceEvent, 0)
	for _, event := range m.pendingEvents {
		if !event.Processed {
			pendingEvents = append(pendingEvents, event)
		}
	}
	m.m.RUnlock()
	
	if len(pendingEvents) == 0 {
		LogInfo("PENDING_FACES", "No pending events to process", "")
		return 0, 0, nil
	}
	
	LogInfo("PENDING_FACES", "Found pending events to process", fmt.Sprintf("Count: %d", len(pendingEvents)))
	
	successCount := 0
	errorCount := 0
	
	for _, event := range pendingEvents {
		LogDebug("PENDING_FACES", "Processing pending event", fmt.Sprintf("Pending ID: %s, Event ID: %s, Camera: %s", event.ID, event.EventID, event.Camera))
		
		if err := m.ProcessPendingEventWithAI(event.ID, facialRecognitionService); err != nil {
			LogError("PENDING_FACES", "Failed to process pending event", fmt.Sprintf("Pending ID: %s, Error: %s", event.ID, err.Error()))
			errorCount++
		} else {
			LogInfo("PENDING_FACES", "Successfully processed pending event", fmt.Sprintf("Pending ID: %s, Event ID: %s", event.ID, event.EventID))
			successCount++
		}
	}
	
	LogInfo("PENDING_FACES", "Automatic processing completed", fmt.Sprintf("Success: %d, Errors: %d, Total: %d", successCount, errorCount, len(pendingEvents)))
	return successCount, errorCount, nil
}

// ProcessPendingEventWithAI processes a pending event with CodeProject.AI
func (m *PendingFacesManager) ProcessPendingEventWithAI(pendingID string, facialRecognitionService *FacialRecognitionService) error {
	event := m.GetPendingEventByID(pendingID)
	if event == nil {
		return fmt.Errorf("pending event with ID %s not found", pendingID)
	}

	if event.Processed {
		return fmt.Errorf("event %s already processed", pendingID)
	}

	LogInfo("PENDING_FACES", "Processing pending event with AI", fmt.Sprintf("Pending ID: %s, Event ID: %s", pendingID, event.EventID))

	// Read image data
	imageData, err := os.ReadFile(event.ImagePath)
	if err != nil {
		LogError("PENDING_FACES", "Failed to read image file", err.Error())
		return err
	}

	// Perform face detection
	detectionResult, err := facialRecognitionService.DetectFaces(imageData)
	if err != nil {
		LogError("PENDING_FACES", "Face detection failed", err.Error())
		return err
	}

	// Perform face recognition
	recognitionResult, err := facialRecognitionService.RecognizeFaces(imageData)
	if err != nil {
		LogError("PENDING_FACES", "Face recognition failed", err.Error())
		return err
	}

	// Build processing notes
	var notes strings.Builder
	notes.WriteString(fmt.Sprintf("Faces detected: %d\n", detectionResult.FacesDetected))
	
	if len(recognitionResult.RecognizedFaces) > 0 {
		notes.WriteString("Recognized faces:\n")
		for _, face := range recognitionResult.RecognizedFaces {
			if face.Person != nil {
				notes.WriteString(fmt.Sprintf("- %s %s (%.1f%%)\n", 
					face.Person.FirstName, face.Person.LastName, face.Confidence*100))
			}
		}
	}
	
	if len(recognitionResult.UnknownFaces) > 0 {
		notes.WriteString(fmt.Sprintf("Unknown faces: %d\n", len(recognitionResult.UnknownFaces)))
	}

	// Mark as processed
	return m.MarkEventAsProcessed(pendingID, notes.String())
}

// GetPendingEventsStats returns statistics about pending events
func (m *PendingFacesManager) GetPendingEventsStats() map[string]interface{} {
	m.m.RLock()
	defer m.m.RUnlock()

	stats := make(map[string]interface{})
	
	total := len(m.pendingEvents)
	processed := 0
	pending := 0
	cameraStats := make(map[string]int)

	for _, event := range m.pendingEvents {
		if event.Processed {
			processed++
		} else {
			pending++
			cameraStats[event.Camera]++
		}
	}

	stats["total"] = total
	stats["processed"] = processed
	stats["pending"] = pending
	stats["byCamera"] = cameraStats

	return stats
}

// CleanupOldProcessedEvents removes processed events older than specified days
func (m *PendingFacesManager) CleanupOldProcessedEvents(daysOld int) error {
	m.m.Lock()
	defer m.m.Unlock()

	cutoffTime := time.Now().AddDate(0, 0, -daysOld)
	var toRemove []int

	for i, event := range m.pendingEvents {
		if event.Processed && event.ProcessedAt.Before(cutoffTime) {
			toRemove = append(toRemove, i)
		}
	}

	// Remove in reverse order to maintain indices
	for i := len(toRemove) - 1; i >= 0; i-- {
		index := toRemove[i]
		event := m.pendingEvents[index]

		// Delete image file
		if event.ImagePath != "" {
			if err := os.Remove(event.ImagePath); err != nil {
				LogWarn("PENDING_FACES", "Failed to delete old image file", err.Error())
			}
		}

		// Remove from slice
		m.pendingEvents = append(m.pendingEvents[:index], m.pendingEvents[index+1:]...)
	}

	if len(toRemove) > 0 {
		if err := m.savePendingEvents(); err != nil {
			LogError("PENDING_FACES", "Failed to save pending events after cleanup", err.Error())
			return err
		}
		LogInfo("PENDING_FACES", "Cleaned up old processed events", fmt.Sprintf("Removed: %d events", len(toRemove)))
	}

	return nil
}

// loadPendingEvents loads pending events from file
func (m *PendingFacesManager) loadPendingEvents() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(m.pendingEventsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(m.pendingEventsPath); os.IsNotExist(err) {
		LogDebug("PENDING_FACES", "Pending events file does not exist, creating new one", m.pendingEventsPath)
		return m.savePendingEvents()
	}

	// Read file
	data, err := os.ReadFile(m.pendingEventsPath)
	if err != nil {
		return err
	}

	// Parse JSON
	if err := json.Unmarshal(data, &m.pendingEvents); err != nil {
		return err
	}

	LogInfo("PENDING_FACES", "Pending events loaded", fmt.Sprintf("Events: %d", len(m.pendingEvents)))
	return nil
}

// savePendingEvents saves pending events to file
func (m *PendingFacesManager) savePendingEvents() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(m.pendingEventsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(m.pendingEvents, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(m.pendingEventsPath, data, 0644)
}

// cleanupEmptyDirectories removes empty directories after file deletion
func (m *PendingFacesManager) cleanupEmptyDirectories(filePath string) {
	dir := filepath.Dir(filePath)

	// Check if directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		LogDebug("PENDING_FACES", "Failed to read directory for cleanup", dir)
		return
	}

	// If directory is empty, remove it
	if len(entries) == 0 {
		if err := os.Remove(dir); err != nil {
			LogDebug("PENDING_FACES", "Failed to remove empty directory", dir)
		} else {
			LogDebug("PENDING_FACES", "Removed empty directory", dir)
		}
	}
}
