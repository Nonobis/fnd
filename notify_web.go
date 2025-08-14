package main

import (
	"fmt"
	"text/template"

	"github.com/gin-gonic/gin"
)

type FNDWebNotificationSink struct {
	config    FNDNotificationConfigurationMap
	lastPict  string
	webServer *FNDWebServer
}

type WebTemplatePayload struct {
	Active         bool
	ShowStatus     bool
	Color          string
	StatusMessage  string
	TranslatedText []string
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

	web.webServer.r.GET("/htmx/web.html", func(c *gin.Context) {
		t := template.Must(template.ParseFS(templateFS, "templates/web.html"))
		t.Execute(c.Writer, web.generatePayload(false))
	})

	web.webServer.r.POST("/htmx/web.html", func(c *gin.Context) {
		web.config.Map["enabled"] = "false"
		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "aktiv" {
				if value[0] == "" {
					continue
				}
				web.config.Map["enabled"] = "true"
				continue
			}
		}

		t := template.Must(template.ParseFS(templateFS, "templates/web.html"))
		t.Execute(c.Writer, web.generatePayload(true))
	})
}

func (web *FNDWebNotificationSink) generatePayload(postReq bool) WebTemplatePayload {
	en, _ := web.config.Map["enabled"]
	var en_bool bool
	if en == "" || en == "false" {
		en_bool = false
	} else {
		en_bool = true
	}

	pay := WebTemplatePayload{
		Active: en_bool,
		TranslatedText: []string{
			web.webServer.translation.lookupToken("active"),
			web.webServer.translation.lookupToken("apply"),
		},
	}

	if !postReq {
		return pay
	}

	pay.ShowStatus = true
	pay.Color = "is-primary"
	pay.StatusMessage = "OK"

	return pay
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
