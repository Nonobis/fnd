package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type FNDFrigateEventManager struct {
	api                 *FNDFrigateApi
	activeEvents        map[string]eventMessage
	notificationChannel chan FNDNotification

	lastNotificationSent time.Time
	fConf                *FNDFrigateConfiguration
	pendingFacesManager  *PendingFacesManager
	taskScheduler        *TaskScheduler

	m sync.Mutex
}

func NewFNDFrigateEventManager(api *FNDFrigateApi, fConf *FNDFrigateConfiguration, facialRecognitionConfig *FNDFacialRecognitionConfiguration, taskScheduler *TaskScheduler) *FNDFrigateEventManager {
	manager := &FNDFrigateEventManager{
		api:                  api,
		activeEvents:         make(map[string]eventMessage),
		notificationChannel:  make(chan FNDNotification, 100),
		lastNotificationSent: time.Now(),
		fConf:                fConf,
		taskScheduler:        taskScheduler,
	}

	// Initialize pending faces manager if facial recognition config is available
	if facialRecognitionConfig != nil {
		manager.pendingFacesManager = NewPendingFacesManager(facialRecognitionConfig)
		LogInfo("EVENT", "Pending faces manager initialized", "")
	}

	return manager
}

func (e *FNDFrigateEventManager) addNewEventMessage(msg eventMessage) error {
	e.m.Lock()
	defer e.m.Unlock()

	LogDebug("EVENT", "Processing event message", fmt.Sprintf("Event ID: %s, Type: %s, Camera: %s, Label: %s",
		msg.Before.Id, msg.TypeInfo, msg.Before.Camera, msg.Before.Label))

	_, avail := e.activeEvents[msg.Before.Id]
	LogDebug("EVENT", "Event status check", fmt.Sprintf("Event ID: %s, Already exists: %t", msg.Before.Id, avail))

	switch msg.TypeInfo {
	case "new":
		if avail {
			LogWarn("EVENT", "Unexpected NEW event", fmt.Sprintf("Event ID: %s already exists", msg.Before.Id))
			return errors.New("Unexpected NEW Event")
		}
		LogInfo("EVENT", "Adding new event", fmt.Sprintf("Event ID: %s, Camera: %s, Label: %s, Score: %.2f",
			msg.Before.Id, msg.Before.Camera, msg.Before.Label, msg.Before.Score))
		e.activeEvents[msg.Before.Id] = msg

		// Store person events for later facial analysis if facial recognition is disabled
		if msg.Before.Label == "person" && e.pendingFacesManager != nil {
			if err := e.storePersonEventForLaterAnalysis(msg); err != nil {
				LogWarn("EVENT", "Failed to store person event for later analysis", err.Error())
			}
		}

		// Queue event for processing if task scheduler is available and enabled
		if e.taskScheduler != nil && e.taskScheduler.config.EnableEventQueue {
			queuedEvent := QueuedEvent{
				ID:        fmt.Sprintf("event_%s_%d", msg.Before.Id, time.Now().Unix()),
				EventID:   msg.Before.Id,
				Type:      msg.TypeInfo,
				Camera:    msg.Before.Camera,
				Label:     msg.Before.Label,
				Score:     msg.Before.Score,
				Timestamp: time.Now(),
				Data:      make(map[string]interface{}),
			}
			
			if err := e.taskScheduler.QueueEvent(queuedEvent); err != nil {
				LogError("EVENT", "Failed to queue event", fmt.Sprintf("Event ID: %s, Error: %s", msg.Before.Id, err.Error()))
			} else {
				LogDebug("EVENT", "Event queued for processing", fmt.Sprintf("Event ID: %s", msg.Before.Id))
			}
		} else {
			// Process immediately if task scheduler is not available or disabled
			var err error
			if e.shouldSendNotification(msg) {
				LogInfo("EVENT", "Notification will be sent", fmt.Sprintf("Event ID: %s", msg.Before.Id))
				err = e.prepareNotification(msg)
			} else {
				LogDebug("EVENT", "Notification skipped", fmt.Sprintf("Event ID: %s - Camera inactive or cooldown active", msg.Before.Id))
			}
			if err != nil {
				return err
			}
		}
	case "update":
		if !avail {
			LogWarn("EVENT", "Unexpected UPDATE event", fmt.Sprintf("Event ID: %s does not exist", msg.Before.Id))
			return errors.New("Unexpected UPDATE Event")
		}
		LogDebug("EVENT", "Updating existing event", fmt.Sprintf("Event ID: %s, New score: %.2f", msg.Before.Id, msg.Before.Score))
		e.activeEvents[msg.Before.Id] = msg
	case "end":
		if !avail {
			LogWarn("EVENT", "Unexpected END event", fmt.Sprintf("Event ID: %s does not exist", msg.Before.Id))
			return errors.New("Unexpected END Event")
		}
		LogInfo("EVENT", "Removing ended event", fmt.Sprintf("Event ID: %s", msg.Before.Id))
		delete(e.activeEvents, msg.Before.Id)
	default:
		LogWarn("EVENT", "Unknown event type", fmt.Sprintf("Event ID: %s, Type: %s", msg.Before.Id, msg.TypeInfo))
	}

	LogDebug("EVENT", "Active events count", fmt.Sprintf("Total active events: %d", len(e.activeEvents)))
	return nil
}

func (e *FNDFrigateEventManager) shouldSendNotification(msg eventMessage) bool {
	LogDebug("NOTIFICATION", "Checking notification criteria", fmt.Sprintf("Event ID: %s, Camera: %s, Object: %s", msg.Before.Id, msg.Before.Camera, msg.Before.Label))

	cameraConfig := e.fConf.checkOrAddCamera(msg.Before.Camera)
	if !cameraConfig.Active {
		LogDebug("NOTIFICATION", "Camera inactive", fmt.Sprintf("Camera: %s, Event ID: %s", msg.Before.Camera, msg.Before.Id))
		return false
	}
	LogDebug("NOTIFICATION", "Camera active", fmt.Sprintf("Camera: %s", msg.Before.Camera))

	// Check object filter if enabled
	if cameraConfig.ObjectFilter.Enabled {
		objectAllowed := false
		for _, allowedObject := range cameraConfig.ObjectFilter.Objects {
			if allowedObject == msg.Before.Label {
				objectAllowed = true
				break
			}
		}
		if !objectAllowed {
			LogDebug("NOTIFICATION", "Object filtered out", fmt.Sprintf("Camera: %s, Object: %s, Allowed objects: %v", msg.Before.Camera, msg.Before.Label, cameraConfig.ObjectFilter.Objects))
			return false
		}
		LogDebug("NOTIFICATION", "Object allowed", fmt.Sprintf("Camera: %s, Object: %s", msg.Before.Camera, msg.Before.Label))
	} else {
		LogDebug("NOTIFICATION", "Object filter disabled", fmt.Sprintf("Camera: %s, Object: %s", msg.Before.Camera, msg.Before.Label))
	}

	diff := time.Since(e.lastNotificationSent)
	cooldownSeconds := diff.Seconds()
	cooldownThreshold := float64(e.fConf.Cooldown)

	LogDebug("NOTIFICATION", "Cooldown check", fmt.Sprintf("Time since last notification: %.2fs, Cooldown threshold: %.2fs", cooldownSeconds, cooldownThreshold))

	shouldSend := cooldownSeconds > cooldownThreshold
	if shouldSend {
		LogDebug("NOTIFICATION", "Cooldown passed", fmt.Sprintf("Event ID: %s will be sent", msg.Before.Id))
	} else {
		LogDebug("NOTIFICATION", "Cooldown active", fmt.Sprintf("Event ID: %s skipped, %.2fs remaining", msg.Before.Id, cooldownThreshold-cooldownSeconds))
	}

	return shouldSend
}

func (e *FNDFrigateEventManager) prepareNotification(msg eventMessage) error {
	LogInfo("NOTIFICATION", "Preparing notification", fmt.Sprintf("Event ID: %s, Camera: %s, Label: %s", msg.Before.Id, msg.Before.Camera, msg.Before.Label))

	n := FNDNotification{
		Caption:  "camera: " + msg.Before.Camera + " object: " + msg.Before.Label,
		Date:     time.Now().Format("15:04:05 02.01.2006"),
		HasVideo: false,
	}

	// Always get snapshot
	LogDebug("NOTIFICATION", "Fetching snapshot", fmt.Sprintf("Event ID: %s", msg.Before.Id))
	jpeg, err := e.api.getSnapshotByID(msg.Before.Id)
	if err != nil {
		LogError("NOTIFICATION", "Failed to get snapshot", fmt.Sprintf("Event ID: %s, Error: %s", msg.Before.Id, err.Error()))
		return err
	}
	n.JpegData = jpeg
	LogDebug("NOTIFICATION", "Snapshot retrieved", fmt.Sprintf("Event ID: %s, Size: %d bytes", msg.Before.Id, len(jpeg)))

	// Try to get video if enabled
	if e.fConf.VideoEnabled {
		LogDebug("NOTIFICATION", "Video enabled, processing video", fmt.Sprintf("Event ID: %s, VideoUrlOnly: %t", msg.Before.Id, e.fConf.VideoUrlOnly))

		if e.fConf.VideoUrlOnly {
			// Just provide URL without downloading the video
			n.VideoURL = e.api.getClipURL(msg.Before.Id)
			n.HasVideo = true
			LogDebug("NOTIFICATION", "Video URL only mode", fmt.Sprintf("Event ID: %s, URL: %s", msg.Before.Id, n.VideoURL))
		} else {
			// Try to download video clip
			LogDebug("NOTIFICATION", "Downloading video clip", fmt.Sprintf("Event ID: %s", msg.Before.Id))
			video, err := e.api.getClipByID(msg.Before.Id)
			if err != nil {
				// Video not available, fallback to URL
				LogWarn("NOTIFICATION", "Video download failed, using URL fallback", fmt.Sprintf("Event ID: %s, Error: %s", msg.Before.Id, err.Error()))
				n.VideoURL = e.api.getClipURL(msg.Before.Id)
			} else {
				// Check video size limit
				videoSizeMB := len(video) / (1024 * 1024)
				LogDebug("NOTIFICATION", "Video size check", fmt.Sprintf("Event ID: %s, Size: %d MB, Max allowed: %d MB", msg.Before.Id, videoSizeMB, e.fConf.MaxVideoSizeMB))

				if videoSizeMB <= e.fConf.MaxVideoSizeMB {
					n.VideoData = video
					n.HasVideo = true
					LogDebug("NOTIFICATION", "Video attached", fmt.Sprintf("Event ID: %s, Size: %d MB", msg.Before.Id, videoSizeMB))
				} else {
					// Video too large, use URL instead
					LogWarn("NOTIFICATION", "Video too large, using URL", fmt.Sprintf("Event ID: %s, Size: %d MB exceeds limit of %d MB", msg.Before.Id, videoSizeMB, e.fConf.MaxVideoSizeMB))
					n.VideoURL = e.api.getClipURL(msg.Before.Id)
					n.HasVideo = true
				}
			}
		}
	} else {
		LogDebug("NOTIFICATION", "Video disabled", fmt.Sprintf("Event ID: %s", msg.Before.Id))
	}

	LogInfo("NOTIFICATION", "Sending notification", fmt.Sprintf("Event ID: %s, HasVideo: %t, HasSnapshot: %t", msg.Before.Id, n.HasVideo, len(n.JpegData) > 0))
	e.sendNotification(n)
	e.lastNotificationSent = time.Now()
	LogDebug("NOTIFICATION", "Notification sent, cooldown reset", fmt.Sprintf("Event ID: %s", msg.Before.Id))

	return nil
}

// Queues the notification. If the queue is too full, the notification
// is discarded. So it never blocks.
func (e *FNDFrigateEventManager) sendNotification(n FNDNotification) {
	queueLength := len(e.notificationChannel)
	queueCapacity := cap(e.notificationChannel)

	LogDebug("NOTIFICATION", "Queue status", fmt.Sprintf("Current length: %d/%d", queueLength, queueCapacity))

	if queueLength == queueCapacity {
		LogWarn("NOTIFICATION", "Queue full, notification discarded", fmt.Sprintf("Caption: %s", n.Caption))
		return
	}

	e.notificationChannel <- n
	LogDebug("NOTIFICATION", "Notification queued", fmt.Sprintf("Caption: %s, Queue length: %d/%d", n.Caption, queueLength+1, queueCapacity))
}

// storePersonEventForLaterAnalysis stores a person event image for later facial analysis
func (e *FNDFrigateEventManager) storePersonEventForLaterAnalysis(msg eventMessage) error {
	LogDebug("EVENT", "Storing person event for later analysis", fmt.Sprintf("Event ID: %s, Camera: %s", msg.Before.Id, msg.Before.Camera))

	// Get snapshot for the event
	jpeg, err := e.api.getSnapshotByID(msg.Before.Id)
	if err != nil {
		LogError("EVENT", "Failed to get snapshot for pending storage", err.Error())
		return err
	}

	// Store in pending faces manager
	return e.pendingFacesManager.StorePersonEvent(msg.Before.Id, msg.Before.Camera, jpeg)
}

func (e *FNDFrigateEventManager) shutdown() {
	close(e.notificationChannel)
}
