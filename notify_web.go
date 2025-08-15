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
	// Check if web notifications are enabled
	if web.config.Map["enabled"] != "true" {
		return nil
	}

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
		_ = t.Execute(c.Writer, web.generatePayload(false))
	})

	web.webServer.r.POST("/htmx/web/toggle", func(c *gin.Context) {
		// Toggle the enabled status
		if web.config.Map["enabled"] == "true" {
			web.config.Map["enabled"] = "false"
		} else {
			web.config.Map["enabled"] = "true"
		}

		// Save configuration to disk immediately
		web.webServer.saveConfiguration()

		// Return updated page
		t := template.Must(template.ParseFS(templateFS, "templates/web.html"))
		_ = t.Execute(c.Writer, web.generatePayload(false))
	})

	web.webServer.r.POST("/htmx/web.html", func(c *gin.Context) {
		c.MultipartForm()

		// Process form fields
		for key, _ := range c.Request.PostForm {
			// Web notifications don't have an active checkbox in the form
			// The active state is managed by the separate toggle button
			_ = key // Keep variable to avoid unused variable error
		}

		LogInfo("WEB", "Configuration updated", fmt.Sprintf("Enabled: %s", web.config.Map["enabled"]))

		// Save configuration to disk immediately
		web.webServer.saveConfiguration()

		t := template.Must(template.ParseFS(templateFS, "templates/web.html"))
		_ = t.Execute(c.Writer, web.generatePayload(true))
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
