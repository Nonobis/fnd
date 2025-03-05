package main

import (
	"fmt"
)

type FNDWebNotificationSink struct {
	config    FNDNotificationConfigurationMap
	lastPict  string
	webServer *FNDWebServer
}

func (web *FNDWebNotificationSink) createDefaultConfig() {
	web.config = NEWDefaultFNDNotificationConfigurationMap()
	web.config.Map["enabled"] = "true"
}

func (web *FNDWebNotificationSink) getName() string {
	return "Web"
}

func (web *FNDWebNotificationSink) setup(conf FNDNotificationConfigurationMap, avail bool) error {
	if avail {
		web.config = conf
	} else {
		web.createDefaultConfig()
	}
	return nil
}

func (web *FNDWebNotificationSink) sendNotification(n FNDNotification) error {
	if web.webServer == nil {
		return nil
	}

	web.webServer.addNotification(n)
	return nil
}

func (web *FNDWebNotificationSink) remove() (FNDNotificationConfigurationMap, error) {
	fmt.Println(web.config)
	return web.config, nil
}

func (web *FNDWebNotificationSink) registerWebServer(webServer *FNDWebServer) {
	web.webServer = webServer
}

func (web *FNDWebNotificationSink) getConfiguration() FNDNotificationConfigurationMap {
	return web.config
}

func (web *FNDWebNotificationSink) getStatus() FNDNotificationSinkStatus {
	return FNDNotificationSinkStatus{
		Name:    web.getName(),
		Good:    true,
		Message: "OK",
	}
}
