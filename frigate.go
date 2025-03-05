package main

import (
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	QOS      = 1
	CLIENTID = "fnd_sub_v1"
)

type FNDFrigateConnection struct {
	mqttServerAddress string
	client            mqtt.Client

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

func newFrigateConnection(conf *FNDFrigateConfiguration) *FNDFrigateConnection {
	con := &FNDFrigateConnection{
		mqttServerAddress: "tcp://" + conf.MqttServer + ":" + conf.MqttPort,
		api:               *NewFNDFrigateApi("http://" + conf.Host + ":" + conf.Port),
	}
	con.eventManager = *NewFNDFrigateEventManager(&con.api, conf)
	return con

}

func (o *FNDFrigateConnection) handle(_ mqtt.Client, msg mqtt.Message) {

	switch msg.Topic() {
	case "frigate/events":
		if err := json.Unmarshal(msg.Payload(), &o.lastEventMessage); err != nil {
			fmt.Printf("Message could not be parsed (%s): %s", msg.Payload(), err)
		} else {
			err = o.eventManager.addNewEventMessage(o.lastEventMessage)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}

}

func setupFNDFrigateConnection(conf *FNDFrigateConfiguration) (*FNDFrigateConnection, error) {

	connection := newFrigateConnection(conf)
	opts := mqtt.NewClientOptions()
	opts.AddBroker(connection.mqttServerAddress)
	opts.SetClientID(CLIENTID)

	opts.SetOrderMatters(false)       // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = time.Second // Minimal delays on connect
	opts.WriteTimeout = time.Second   // Minimal delays on writes
	opts.KeepAlive = 10               // Keepalive every 10 seconds so we quickly detect network outages
	opts.PingTimeout = time.Second    // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = true
	opts.AutoReconnect = true

	opts.DefaultPublishHandler = func(_ mqtt.Client, msg mqtt.Message) {
		fmt.Printf("UNEXPECTED MESSAGE: %s\n", msg)
	}

	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		fmt.Println("MQTT connection lost")
	}

	opts.OnConnect = func(c mqtt.Client) {
		fmt.Println("MQTT connection established")

		t := c.Subscribe("frigate/events", QOS, connection.handle)

		go func() {
			_ = t.Wait()
			if t.Error() != nil {
				fmt.Printf("ERROR SUBSCRIBING: %s\n", t.Error())
			} else {
				fmt.Println("subscribed to: ", "frigate/events")
			}
		}()
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		fmt.Println("MQTT: attempting to reconnect")
	}

	connection.client = mqtt.NewClient(opts)

	token := connection.client.Connect()

	go func() {
		if token.Wait() && token.Error() != nil {
			fmt.Println("MQTT Error: ", token.Error())
			return
		}
		fmt.Println("Connection is up")
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
	if connection.client.IsConnected() {
		s.Message = "OK"
	} else {
		s.Message = "Init"
	}
	s.Good = connection.client.IsConnected()
	s.Name = "Frigate"
	return s
}
