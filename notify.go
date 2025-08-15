package main

import (
	"fmt"
	"time"
)

type FNDNotification struct {
	JpegData  []byte
	VideoData []byte
	VideoURL  string
	Date      string
	Caption   string
	HasVideo  bool
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
}

func NewFNDNotificationManager(conf FNDNotificationConfiguration) *FNDNotificationManager {
	return &FNDNotificationManager{
		conf:  conf,
		sinks: make(map[string]FNDNotificationSink),
	}

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
		fmt.Println("Sink:", sink.getName(), " setup failed: ", err.Error())
		return
	}

	m.sinks[sink.getName()] = sink
	LogInfo("NOTIFY", "Notification sink registered", fmt.Sprintf("Sink: %s", sink.getName()))
	fmt.Println("Registered: ", sink.getName())
}

func (m *FNDNotificationManager) notifyAll(n FNDNotification) {
	enabledCount := 0
	for _, v := range m.sinks {
		// Check if the sink is enabled before sending notification
		if m.isSinkEnabled(v) {
			enabledCount++
			err := v.sendNotification(n)
			if err != nil {
				LogError("NOTIFY", "Failed to send notification", fmt.Sprintf("Sink: %s, Error: %s", v.getName(), err.Error()))
				fmt.Println(err.Error())
			} else {
				LogInfo("NOTIFY", "Notification sent successfully", fmt.Sprintf("Sink: %s, Caption: %s", v.getName(), n.Caption))
			}
		}
	}
	if enabledCount == 0 {
		LogWarn("NOTIFY", "No enabled notification sinks", "Notification not sent")
	}
}

// Check if a notification sink is enabled
func (m *FNDNotificationManager) isSinkEnabled(sink FNDNotificationSink) bool {
	config := sink.getConfiguration()
	enabled, exists := config.Map["enabled"]
	return exists && enabled == "true"
}

func (m *FNDNotificationManager) getStatusAll() {

	// Always add Frigate status - it will show "Configuration required" if not configured
	m.web.addNotificationSinkStatus(m.frigateConn.getStatus())

	// Add MQTT status as a separate block
	m.web.addNotificationSinkStatus(m.frigateConn.getMqttStatus())



	for _, v := range m.sinks {
		// Only add status for enabled sinks to overview
		if m.isSinkEnabled(v) {
			m.web.addNotificationSinkStatus(v.getStatus())
		}
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
