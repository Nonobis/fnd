package main

import (
	"context"
	"fmt"
	"time"
)

type BackgroundTask struct {
	ctx                context.Context
	cancel             context.CancelFunc
	api                *FNDFrigateApi
	conf               *FNDConfiguration
	notify             *FNDNotificationManager
	configuration_path string
}

func RunBackgroundTask(api *FNDFrigateApi,
	conf *FNDConfiguration,
	notify *FNDNotificationManager,
	configuration_path string) *BackgroundTask {
	bg := BackgroundTask{
		api:                api,
		conf:               conf,
		notify:             notify,
		configuration_path: configuration_path,
	}

	bg.ctx, bg.cancel = context.WithCancel(context.Background())

	go bg.task()
	return &bg

}

func (bg *BackgroundTask) task() {
	ticker := time.NewTicker(10 * time.Second)

	// Initialize pending faces auto-process ticker
	var pendingFacesTicker *time.Ticker
	var pendingFacesTickerDuration time.Duration

	// Check if pending faces auto-process is enabled
	if bg.conf.FacialRecognition.PendingFacesAutoProcess && bg.conf.FacialRecognition.Enabled {
		intervalHours := bg.conf.FacialRecognition.PendingFacesInterval
		if intervalHours < 1 {
			intervalHours = 6 // Default to 6 hours if invalid
			LogWarn("BACKGROUND", "Invalid pending faces interval, using default", fmt.Sprintf("Configured: %d hours, Using: %d hours", bg.conf.FacialRecognition.PendingFacesInterval, intervalHours))
		}
		pendingFacesTickerDuration = time.Duration(intervalHours) * time.Hour
		pendingFacesTicker = time.NewTicker(pendingFacesTickerDuration)
		LogInfo("BACKGROUND", "Pending faces auto-process enabled", fmt.Sprintf("Interval: %d hours (%v)", intervalHours, pendingFacesTickerDuration))
	} else {
		LogInfo("BACKGROUND", "Pending faces auto-process disabled", fmt.Sprintf("AutoProcess: %t, FacialRecognition: %t", bg.conf.FacialRecognition.PendingFacesAutoProcess, bg.conf.FacialRecognition.Enabled))
	}

	defer ticker.Stop()
	if pendingFacesTicker != nil {
		defer pendingFacesTicker.Stop()
	}

	LogInfo("BACKGROUND", "Background task started", fmt.Sprintf("Camera check: 10s, Pending faces: %v", pendingFacesTickerDuration))

	// Create a channel for pending faces ticker that can be nil
	var pendingFacesChan <-chan time.Time
	if pendingFacesTicker != nil {
		pendingFacesChan = pendingFacesTicker.C
	}

	for {
		select {
		case <-bg.ctx.Done():
			LogInfo("BACKGROUND", "Background task stopped", "")
			return
		case <-ticker.C:
			cams, err := bg.api.getCameras()
			if err != nil {
				LogError("BACKGROUND", "Failed to get cameras from API", err.Error())
				continue
			}
			discoveredCount := 0
			for k := range cams.Cameras {
				if _, exists := bg.conf.Frigate.Cameras[k]; !exists {
					discoveredCount++
				}
				_ = bg.conf.Frigate.checkOrAddCamera(k)
			}
			if discoveredCount > 0 {
				LogInfo("BACKGROUND", "Camera discovery completed", fmt.Sprintf("New cameras: %d", discoveredCount))
			}
		// Removed periodic configuration save - configuration is now saved immediately when changed
		case <-pendingFacesChan:
			bg.processPendingFacesAutomatically()
		}
	}
}

// processPendingFacesAutomatically processes all pending face events automatically
func (bg *BackgroundTask) processPendingFacesAutomatically() {
	LogInfo("BACKGROUND", "Starting automatic pending faces processing", "")

	// Check if facial recognition is enabled and pending faces manager is available
	if !bg.conf.FacialRecognition.Enabled {
		LogWarn("BACKGROUND", "Facial recognition not enabled, skipping pending faces processing", "")
		return
	}

	if bg.notify.pendingFacesManager == nil {
		LogWarn("BACKGROUND", "Pending faces manager not available, skipping pending faces processing", "")
		return
	}

	if bg.notify.facialRecognitionService == nil {
		LogWarn("BACKGROUND", "Facial recognition service not available, skipping pending faces processing", "")
		return
	}

	LogInfo("BACKGROUND", "Processing pending faces automatically", fmt.Sprintf("Interval: %d hours", bg.conf.FacialRecognition.PendingFacesInterval))

	// Get current stats before processing
	stats := bg.notify.pendingFacesManager.GetPendingEventsStats()
	pendingCount := stats["pending"].(int)

	if pendingCount == 0 {
		LogInfo("BACKGROUND", "No pending events to process", "")
		return
	}

	LogInfo("BACKGROUND", "Found pending events for automatic processing", fmt.Sprintf("Count: %d", pendingCount))

	// Process all pending events
	successCount, errorCount, err := bg.notify.pendingFacesManager.ProcessAllPendingEventsWithAI(bg.notify.facialRecognitionService)

	if err != nil {
		LogError("BACKGROUND", "Automatic pending faces processing failed", err.Error())
	} else {
		LogInfo("BACKGROUND", "Automatic pending faces processing completed", fmt.Sprintf("Success: %d, Errors: %d, Total: %d", successCount, errorCount, pendingCount))

		// Get updated stats after processing
		updatedStats := bg.notify.pendingFacesManager.GetPendingEventsStats()
		remainingPending := updatedStats["pending"].(int)

		LogInfo("BACKGROUND", "Pending faces processing summary", fmt.Sprintf("Processed: %d, Remaining: %d, Errors: %d", successCount, remainingPending, errorCount))
	}
}
