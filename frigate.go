package main

import (
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	QOS = 1
)

type FNDFrigateConnection struct {
	mqttServerAddress string
	client            mqtt.Client
	config            *FNDFrigateConfiguration

	lastEventMessage eventMessage
	eventManager     FNDFrigateEventManager
	api              FNDFrigateApi
}

type eventMessage struct {
	TypeInfo string `json:"type"`
	Before   struct {
		Id             string  `json:"id"`
		Camera         string  `json:"camera"`
		Label          string  `json:"label"`
		Top_Score      float32 `json:"top_score"`
		False_Positive bool    `json:"false_positive"`
		Score          float32 `json:"score"`
	} `json:"before"`
	After struct {
		Id             string  `json:"id"`
		Camera         string  `json:"camera"`
		Label          string  `json:"label"`
		Top_Score      float32 `json:"top_score"`
		False_Positive bool    `json:"false_positive"`
		Score          float32 `json:"score"`
	} `json:"after"`
}

func newFrigateConnection(conf *FNDFrigateConfiguration, facialRecognitionConfig *FNDFacialRecognitionConfiguration, taskScheduler *TaskScheduler) *FNDFrigateConnection {
	con := &FNDFrigateConnection{
		mqttServerAddress: "tcp://" + conf.MqttServer + ":" + conf.MqttPort,
		config:            conf,
		api:               *NewFNDFrigateApi("http://"+conf.Host+":"+conf.Port, conf.ExternalURL),
	}
	con.eventManager = *NewFNDFrigateEventManager(&con.api, conf, facialRecognitionConfig, taskScheduler)
	return con

}

func (o *FNDFrigateConnection) handle(_ mqtt.Client, msg mqtt.Message) {
	LogDebug("MQTT", "Message received", fmt.Sprintf("Topic: %s, Payload length: %d bytes", msg.Topic(), len(msg.Payload())))

	// Get topic prefix from configuration or use default
	topicPrefix := o.config.MqttTopicPrefix
	if topicPrefix == "" {
		topicPrefix = "frigate"
	}
	eventsTopic := topicPrefix + "/events"

	switch msg.Topic() {
	case eventsTopic:
		LogDebug("FRIGATE", "Processing Frigate event", fmt.Sprintf("Raw payload: %s", string(msg.Payload())))
		LogInfo("FRIGATE", "Event received", fmt.Sprintf("Topic: %s, Payload length: %d bytes", msg.Topic(), len(msg.Payload())))

		if err := json.Unmarshal(msg.Payload(), &o.lastEventMessage); err != nil {
			LogError("FRIGATE", "Failed to parse event message", fmt.Sprintf("Payload: %s, Error: %s", string(msg.Payload()), err.Error()))
		} else {
			LogInfo("FRIGATE", "Event message parsed successfully", fmt.Sprintf("Event ID: %s, Type: %s, Camera: %s, Label: %s, Score: %.2f",
				o.lastEventMessage.Before.Id,
				o.lastEventMessage.TypeInfo,
				o.lastEventMessage.Before.Camera,
				o.lastEventMessage.Before.Label,
				o.lastEventMessage.Before.Score))

			err = o.eventManager.addNewEventMessage(o.lastEventMessage)
			if err != nil {
				LogError("FRIGATE", "Failed to process event message", err.Error())
			} else {
				LogDebug("FRIGATE", "Event message processed successfully", fmt.Sprintf("Event ID: %s", o.lastEventMessage.Before.Id))
			}
		}
	default:
		LogWarn("MQTT", "Unexpected topic received", fmt.Sprintf("Topic: %s", msg.Topic()))
		LogDebug("MQTT", "Unexpected message details", fmt.Sprintf("Topic: %s, Payload: %s", msg.Topic(), string(msg.Payload())))
	}
}

func setupFNDFrigateConnection(conf *FNDFrigateConfiguration, facialRecognitionConfig *FNDFacialRecognitionConfiguration, taskScheduler *TaskScheduler) (*FNDFrigateConnection, error) {

	connection := newFrigateConnection(conf, facialRecognitionConfig, taskScheduler)
	opts := mqtt.NewClientOptions()
	opts.AddBroker(connection.mqttServerAddress)
	// Use configured client ID or default
	clientID := connection.config.MqttClientID
	if clientID == "" {
		clientID = "fnd"
	}
	opts.SetClientID(clientID)

	// Set authentication if not anonymous
	if !connection.config.MqttAnonymous && connection.config.MqttUsername != "" {
		opts.SetUsername(connection.config.MqttUsername)
		if connection.config.MqttPassword != "" {
			opts.SetPassword(connection.config.MqttPassword)
		}
		LogInfo("MQTT", "Using authentication", fmt.Sprintf("Username: %s", connection.config.MqttUsername))
		LogDebug("MQTT", "Authentication details", fmt.Sprintf("Username: %s, Password set: %t", connection.config.MqttUsername, connection.config.MqttPassword != ""))
	} else {
		LogInfo("MQTT", "Using anonymous connection", "")
		LogDebug("MQTT", "Anonymous connection", "No authentication required")
	}

	opts.SetOrderMatters(false)       // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = time.Second // Minimal delays on connect
	opts.WriteTimeout = time.Second   // Minimal delays on writes
	opts.KeepAlive = 10               // Keepalive every 10 seconds so we quickly detect network outages
	opts.PingTimeout = time.Second    // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = true
	opts.AutoReconnect = true

	opts.DefaultPublishHandler = func(_ mqtt.Client, msg mqtt.Message) {
		LogWarn("MQTT", "Unexpected message received", fmt.Sprintf("Topic: %s, Payload: %s", msg.Topic(), string(msg.Payload())))
	}

	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		LogError("MQTT", "Connection lost", err.Error())
		LogDebug("MQTT", "Connection lost details", fmt.Sprintf("Error: %s, Client connected: %t", err.Error(), cl.IsConnected()))
	}

	opts.OnConnect = func(c mqtt.Client) {
		LogInfo("MQTT", "Connection established", fmt.Sprintf("Broker: %s", connection.mqttServerAddress))
		LogDebug("MQTT", "Connection details", fmt.Sprintf("Client ID: %s, QoS: %d", clientID, QOS))

		// Get topic prefix for subscription
		topicPrefix := connection.config.MqttTopicPrefix
		if topicPrefix == "" {
			topicPrefix = "frigate"
		}
		eventsTopic := topicPrefix + "/events"

		t := c.Subscribe(eventsTopic, QOS, connection.handle)

		go func() {
			_ = t.Wait()
			if t.Error() != nil {
				LogError("MQTT", "Failed to subscribe", t.Error().Error())
			} else {
				LogInfo("MQTT", "Successfully subscribed", fmt.Sprintf("Topic: %s", eventsTopic))
				LogDebug("MQTT", "Subscription details", fmt.Sprintf("Topic: %s, QoS: %d", eventsTopic, QOS))
			}
		}()
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		LogInfo("MQTT", "Attempting to reconnect", fmt.Sprintf("Broker: %s", connection.mqttServerAddress))
		LogDebug("MQTT", "Reconnection attempt", fmt.Sprintf("Broker: %s, Time: %s", connection.mqttServerAddress, time.Now().Format("15:04:05")))
	}

	connection.client = mqtt.NewClient(opts)

	token := connection.client.Connect()

	go func() {
		if token.Wait() && token.Error() != nil {
			LogError("MQTT", "Connection failed", token.Error().Error())
			LogDebug("MQTT", "Connection failure details", fmt.Sprintf("Error: %s, Broker: %s", token.Error().Error(), connection.mqttServerAddress))
			return
		}
		LogInfo("MQTT", "Connection successful", "")
		LogDebug("MQTT", "Connection success details", fmt.Sprintf("Broker: %s, Time: %s", connection.mqttServerAddress, time.Now().Format("15:04:05")))
	}()

	return connection, nil
}

func (connection *FNDFrigateConnection) Disconnect() {
	connection.client.Disconnect(1000)
	connection.eventManager.shutdown()
}

// FNDNotificationSinkStatus hier bissl missbraucht
func (connection *FNDFrigateConnection) getStatus() FNDNotificationSinkStatus {
	var s FNDNotificationSinkStatus
	s.Name = "Frigate"

	// Check if configuration is valid (not default values)
	if !connection.isConfigurationValid() {
		s.Good = false
		s.Message = "Configuration required"
		LogDebug("FRIGATE", "Status check", "Configuration invalid - using default values")
		return s
	}

	// Check connection status
	if connection.client.IsConnected() {
		s.Message = "Connected"
		s.Good = true
		LogDebug("FRIGATE", "Status check", "Connection is active")
	} else {
		s.Message = "Disconnected"
		s.Good = false
		LogDebug("FRIGATE", "Status check", "Connection is inactive")
	}

	return s
}

// Check if Frigate configuration contains valid (non-default) values
func (connection *FNDFrigateConnection) isConfigurationValid() bool {
	conf := connection.eventManager.fConf

	// Check if configuration values are not the default placeholder values or empty
	if conf.Host == "" || conf.Host == "frigate" ||
		conf.MqttServer == "" || conf.MqttServer == "mqtt-server" ||
		conf.Port == "" || conf.MqttPort == "" {
		return false
	}

	// Additional check: ensure ports are reasonable
	if conf.Port == "5000" && conf.MqttPort == "1883" &&
		(conf.Host == "frigate" || conf.MqttServer == "mqtt-server") {
		return false
	}

	return true
}

// Get MQTT-specific status for overview display
func (connection *FNDFrigateConnection) getMqttStatus() FNDNotificationSinkStatus {
	var s FNDNotificationSinkStatus
	s.Name = "MQTT"

	// Check if MQTT configuration is valid
	if !connection.isMqttConfigurationValid() {
		s.Good = false
		s.Message = "Configuration required"
		return s
	}

	// Check connection status
	if connection.client.IsConnected() {
		s.Message = "Connected"
		s.Good = true
	} else {
		s.Message = "Disconnected"
		s.Good = false
	}

	return s
}

// Check if MQTT configuration specifically is valid
func (connection *FNDFrigateConnection) isMqttConfigurationValid() bool {
	conf := connection.eventManager.fConf

	// Check basic MQTT configuration
	if conf.MqttServer == "" || conf.MqttServer == "mqtt-server" ||
		conf.MqttPort == "" {
		return false
	}

	// Check authentication configuration
	if !conf.MqttAnonymous {
		if conf.MqttUsername == "" {
			return false
		}
	}

	return true
}

// PublishTestEvent publishes a test event via the real MQTT connection
func (connection *FNDFrigateConnection) PublishTestEvent() error {
	if !connection.client.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	// Generate a test event
	testEvent := eventMessage{
		TypeInfo: "new",
		Before: struct {
			Id             string  `json:"id"`
			Camera         string  `json:"camera"`
			Label          string  `json:"label"`
			Top_Score      float32 `json:"top_score"`
			False_Positive bool    `json:"false_positive"`
			Score          float32 `json:"score"`
		}{
			Id:             fmt.Sprintf("test_event_%d", time.Now().Unix()),
			Camera:         "test_camera",
			Label:          "person",
			Top_Score:      0.95,
			False_Positive: false,
			Score:          0.95,
		},
		After: struct {
			Id             string  `json:"id"`
			Camera         string  `json:"camera"`
			Label          string  `json:"label"`
			Top_Score      float32 `json:"top_score"`
			False_Positive bool    `json:"false_positive"`
			Score          float32 `json:"score"`
		}{
			Id:             fmt.Sprintf("test_event_%d", time.Now().Unix()),
			Camera:         "test_camera",
			Label:          "person",
			Top_Score:      0.95,
			False_Positive: false,
			Score:          0.95,
		},
	}

	// Convert to JSON
	payload, err := json.Marshal(testEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal test event: %v", err)
	}

	// Get topic prefix for publishing
	topicPrefix := connection.config.MqttTopicPrefix
	if topicPrefix == "" {
		topicPrefix = "frigate"
	}
	eventsTopic := topicPrefix + "/events"

	// Publish the test event
	token := connection.client.Publish(eventsTopic, QOS, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish test event: %v", token.Error())
	}

	LogInfo("MQTT", "Test event published successfully", fmt.Sprintf("Topic: %s, Event ID: %s", eventsTopic, testEvent.Before.Id))
	return nil
}
