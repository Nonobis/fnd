package main

import (
	"fmt"
	"strings"
	"time"
)

type FNDNotification struct {
	JpegData              []byte
	VideoData             []byte
	VideoURL              string
	Date                  string
	Caption               string
	Title                 string
	HasVideo              bool
	FaceRecognitionResult *FaceRecognitionResult
}

type FNDNotificationSink interface {
	//must be unique
	getName() string
	setup(FNDNotificationConfigurationMap, bool) error
	sendNotification(FNDNotification) error
	remove() (FNDNotificationConfigurationMap, error)
	registerWebServer(webServer *FNDWebServer)
	getConfiguration() FNDNotificationConfigurationMap
	getStatus() FNDNotificationSinkStatus
}

type FNDNotificationManager struct {
	conf  FNDNotificationConfiguration
	sinks map[string]FNDNotificationSink

	//for status
	web         *FNDWebServer
	frigateConn *FNDFrigateConnection

	//template processor
	templateProcessor *TemplateProcessor

	//facial recognition
	facialRecognitionService *FacialRecognitionService
	pendingFacesManager      *PendingFacesManager
}

func NewFNDNotificationManager(conf FNDNotificationConfiguration, facialRecognitionConfig *FNDFacialRecognitionConfiguration) *FNDNotificationManager {
	manager := &FNDNotificationManager{
		conf:              conf,
		sinks:             make(map[string]FNDNotificationSink),
		templateProcessor: NewTemplateProcessor(&conf.Templates),
	}

	// Initialize facial recognition service if enabled
	if facialRecognitionConfig != nil && facialRecognitionConfig.Enabled {
		manager.facialRecognitionService = NewFacialRecognitionService(facialRecognitionConfig)
		LogInfo("NOTIFY", "Facial recognition service initialized", "")
	}

	// Initialize pending faces manager
	if facialRecognitionConfig != nil {
		manager.pendingFacesManager = NewPendingFacesManager(facialRecognitionConfig)
		LogInfo("NOTIFY", "Pending faces manager initialized", "")
	}

	return manager
}

func (m *FNDNotificationManager) setupNotificationSinks(c chan FNDNotification, web *FNDWebServer, frigateConn *FNDFrigateConnection) {
	m.registerNotificationSinks(&FNDTelegramNotificationSink{})
	m.registerNotificationSinks(&FNDAppriseNotificationSink{})
	m.registerNotificationSinks(&FNDGotifyNotificationSink{})

	m.web = web
	m.frigateConn = frigateConn
	for _, s := range m.sinks {
		s.registerWebServer(web)
	}
	go m.notificationThread(c)
	m.getStatusAll()
}

func (m *FNDNotificationManager) registerNotificationSinks(sink FNDNotificationSink) {
	_, avail := m.sinks[sink.getName()]
	if avail {
		LogWarn("NOTIFY", "Sink already registered", fmt.Sprintf("Sink: %s", sink.getName()))
		fmt.Println("Sink: ", sink.getName(), " is already registered")
		return
	}

	data, avail := m.conf.Conf[sink.getName()]
	err := sink.setup(data, avail)
	if err != nil {
		LogError("NOTIFY", "Sink setup failed", fmt.Sprintf("Sink: %s, Error: %s", sink.getName(), err.Error()))
		CaptureError(err, map[string]interface{}{
			"component": "notification_manager",
			"action":    "setup_sink",
			"sink_name": sink.getName(),
		})
		fmt.Println("Sink:", sink.getName(), " setup failed: ", err.Error())
		return
	}

	m.sinks[sink.getName()] = sink
	LogInfo("NOTIFY", "Notification sink registered", fmt.Sprintf("Sink: %s", sink.getName()))
	fmt.Println("Registered: ", sink.getName())
}

func (m *FNDNotificationManager) notifyAll(n FNDNotification) {
	LogInfo("NOTIFY", "Sending notification to all enabled sinks", fmt.Sprintf("Caption: %s", n.Caption))

	// Process facial recognition if enabled and image data is available
	if m.facialRecognitionService != nil && len(n.JpegData) > 0 {
		m.processFacialRecognition(&n)
	}

	enabledCount := 0
	for sinkName, v := range m.sinks {
		// Check if the sink is enabled before sending notification
		enabled := m.isSinkEnabled(v)
		LogDebug("NOTIFY", "Checking sink for notification", fmt.Sprintf("Sink: %s, Enabled: %t", sinkName, enabled))

		if enabled {
			enabledCount++

			// Process template if available
			processedN := n
			if m.templateProcessor != nil {
				// Extract camera and object from caption (fallback format)
				camera := "unknown"
				object := "unknown"
				if n.Caption != "" {
					// Try to parse "camera: X object: Y" format
					if len(n.Caption) > 8 && n.Caption[:8] == "camera: " {
						parts := strings.Split(n.Caption, " object: ")
						if len(parts) == 2 {
							camera = parts[0][8:] // Remove "camera: " prefix
							object = parts[1]
						}
					}
				}

				variables := CreateTemplateVariables(n, camera, object, "")
				title, message, err := m.templateProcessor.ProcessTemplate(v.getName(), variables)
				if err != nil {
					LogWarn("NOTIFY", "Template processing failed, using default", fmt.Sprintf("Sink: %s, Error: %s", v.getName(), err.Error()))
				} else {
					processedN.Caption = message
					// Store title for services that support it
					processedN.Title = title
				}
			}

			err := v.sendNotification(processedN)
			if err != nil {
				LogError("NOTIFY", "Failed to send notification", fmt.Sprintf("Sink: %s, Error: %s", v.getName(), err.Error()))
				fmt.Println(err.Error())
			} else {
				LogInfo("NOTIFY", "Notification sent successfully", fmt.Sprintf("Sink: %s, Caption: %s", v.getName(), processedN.Caption))
			}
		}
	}
	if enabledCount == 0 {
		LogWarn("NOTIFY", "No enabled notification sinks", "Notification not sent")
	}
}

// Check if a notification sink is enabled
func (m *FNDNotificationManager) isSinkEnabled(sink FNDNotificationSink) bool {
	sinkName := sink.getName()
	config := sink.getConfiguration()
	enabled, exists := config.Map["enabled"]
	enabledBool := exists && enabled == "true"
	LogDebug("NOTIFY", "Sink enabled check", fmt.Sprintf("Sink: %s, Config exists: %t, Enabled value: %s, Enabled: %t", sinkName, exists, enabled, enabledBool))
	return enabledBool
}

func (m *FNDNotificationManager) getStatusAll() {
	LogDebug("NOTIFY", "Updating status for all notification sinks", "")

	// Always add Frigate status - it will show "Configuration required" if not configured
	frigateStatus := m.frigateConn.getStatus()
	LogDebug("NOTIFY", "Frigate status", fmt.Sprintf("Name: %s, Good: %t, Message: %s", frigateStatus.Name, frigateStatus.Good, frigateStatus.Message))
	m.web.addNotificationSinkStatus(frigateStatus)

	// Add MQTT status as a separate block
	mqttStatus := m.frigateConn.getMqttStatus()
	LogDebug("NOTIFY", "MQTT status", fmt.Sprintf("Name: %s, Good: %t, Message: %s", mqttStatus.Name, mqttStatus.Good, mqttStatus.Message))
	m.web.addNotificationSinkStatus(mqttStatus)

	for sinkName, v := range m.sinks {
		LogDebug("NOTIFY", "Processing sink", fmt.Sprintf("Name: %s, Enabled: %t", sinkName, m.isSinkEnabled(v)))

		// Add status for all sinks to overview, regardless of enabled state
		status := v.getStatus()

		// If the sink is disabled, modify the status to show it's disabled
		if !m.isSinkEnabled(v) {
			LogDebug("NOTIFY", "Sink disabled, updating status", fmt.Sprintf("Name: %s", sinkName))
			status.Good = false
			status.Message = "Disabled"
		}

		LogDebug("NOTIFY", "Sink status", fmt.Sprintf("Name: %s, Good: %t, Message: %s", status.Name, status.Good, status.Message))
		m.web.addNotificationSinkStatus(status)
	}
}

func (m *FNDNotificationManager) removeAll() FNDNotificationConfiguration {
	for _, v := range m.sinks {
		conf, err := v.remove()
		if err != nil {
			fmt.Println(err.Error())
			continue
		}
		m.conf.Conf[v.getName()] = conf
	}

	return m.conf
}

func (m *FNDNotificationManager) getConfigAll() FNDNotificationConfiguration {
	for _, v := range m.sinks {
		m.conf.Conf[v.getName()] = v.getConfiguration()
	}

	return m.conf
}

// getSink returns a specific notification sink by name
func (m *FNDNotificationManager) getSink(name string) (FNDNotificationSink, bool) {
	sink, exists := m.sinks[name]
	return sink, exists
}

func (m *FNDNotificationManager) notificationThread(c chan FNDNotification) {
	for {
		n, ok := <-c
		if !ok {
			return
		}
		m.notifyAll(n)
		m.getStatusAll()
	}
}

func (m *FNDNotificationManager) sendLiveSnapshot(camera string, api *FNDFrigateApi) error {
	// Get live snapshot from camera
	imageData, err := api.getLiveSnapshotByCamera(camera)
	if err != nil {
		return err
	}

	// Create notification with live snapshot
	n := FNDNotification{
		JpegData:  imageData,
		Date:      time.Now().Format("15:04:05 02.01.2006"),
		Caption:   "Live snapshot from camera: " + camera,
		HasVideo:  false,
		VideoData: nil,
		VideoURL:  "",
	}

	// Send to all notification sinks
	m.notifyAll(n)
	m.getStatusAll()

	return nil
}

// updateTemplates updates the template processor with new templates
func (m *FNDNotificationManager) updateTemplates(templates *NotificationTemplates) {
	m.templateProcessor = NewTemplateProcessor(templates)
	LogInfo("NOTIFY", "Templates updated", "Template processor refreshed")
}

// processFacialRecognition processes facial recognition on the notification image
func (m *FNDNotificationManager) processFacialRecognition(n *FNDNotification) {
	LogDebug("NOTIFY", "Processing facial recognition", fmt.Sprintf("Image size: %d bytes", len(n.JpegData)))

	// Perform face recognition
	recognitionResult, err := m.facialRecognitionService.RecognizeFaces(n.JpegData)
	if err != nil {
		LogError("NOTIFY", "Facial recognition failed", err.Error())
		return
	}

	// Store the recognition result
	n.FaceRecognitionResult = recognitionResult

	// Log recognition results
	if len(recognitionResult.RecognizedFaces) > 0 {
		recognizedNames := make([]string, 0, len(recognitionResult.RecognizedFaces))
		for _, face := range recognitionResult.RecognizedFaces {
			if face.Person != nil {
				recognizedNames = append(recognizedNames, fmt.Sprintf("%s %s (%.1f%%)",
					face.Person.FirstName, face.Person.LastName, face.Confidence*100))
			}
		}
		LogInfo("NOTIFY", "Faces recognized", fmt.Sprintf("Recognized: %s", strings.Join(recognizedNames, ", ")))
	}

	if len(recognitionResult.UnknownFaces) > 0 {
		LogInfo("NOTIFY", "Unknown faces detected", fmt.Sprintf("Count: %d", len(recognitionResult.UnknownFaces)))
	}

	// Update notification caption with facial recognition results
	m.updateNotificationWithFacialRecognition(n, recognitionResult)
}

// updateNotificationWithFacialRecognition updates the notification caption with facial recognition results
func (m *FNDNotificationManager) updateNotificationWithFacialRecognition(n *FNDNotification, result *FaceRecognitionResult) {
	if result == nil {
		return
	}

	var recognitionInfo strings.Builder

	// Add recognized faces information
	if len(result.RecognizedFaces) > 0 {
		recognitionInfo.WriteString("\n👤 Recognized: ")
		recognizedNames := make([]string, 0, len(result.RecognizedFaces))
		for _, face := range result.RecognizedFaces {
			if face.Person != nil {
				recognizedNames = append(recognizedNames, fmt.Sprintf("%s %s (%.1f%%)",
					face.Person.FirstName, face.Person.LastName, face.Confidence*100))
			}
		}
		recognitionInfo.WriteString(strings.Join(recognizedNames, ", "))
	}

	// Add unknown faces information
	if len(result.UnknownFaces) > 0 {
		if recognitionInfo.Len() > 0 {
			recognitionInfo.WriteString("\n")
		}
		recognitionInfo.WriteString(fmt.Sprintf("❓ Unknown faces: %d", len(result.UnknownFaces)))
	}

	// Append recognition info to caption
	if recognitionInfo.Len() > 0 {
		n.Caption += recognitionInfo.String()
	}
}

// GetPendingFacesManager returns the pending faces manager
func (m *FNDNotificationManager) GetPendingFacesManager() *PendingFacesManager {
	return m.pendingFacesManager
}

// GetFacialRecognitionService returns the facial recognition service
func (m *FNDNotificationManager) GetFacialRecognitionService() *FacialRecognitionService {
	return m.facialRecognitionService
}
