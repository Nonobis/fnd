package main

import (
	"fmt"
)

type FNDNotification struct {
	JpegData []byte
	Date     string
	Caption  string
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
	m.registerNotificationSinks(&FNDWebNotificationSink{})
	m.registerNotificationSinks(&FNDTelegramNotificationSink{})
	m.registerNotificationSinks(&FNDAppriseNotificationSink{})

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
		fmt.Println("Sink: ", sink.getName(), " is already registered")
		return
	}

	data, avail := m.conf.Conf[sink.getName()]
	err := sink.setup(data, avail)
	if err != nil {
		fmt.Println("Sink:", sink.getName(), " setup failed: ", err.Error())
		return
	}

	m.sinks[sink.getName()] = sink

	fmt.Println("Registered: ", sink.getName())
}

func (m *FNDNotificationManager) notifyAll(n FNDNotification) {
	for _, v := range m.sinks {
		err := v.sendNotification(n)
		if err != nil {
			fmt.Println(err.Error())
		}
	}
}

func (m *FNDNotificationManager) getStatusAll() {

	m.web.addNotificationSinkStatus(m.frigateConn.getStatus())
	for _, v := range m.sinks {
		m.web.addNotificationSinkStatus(v.getStatus())
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
