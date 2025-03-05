package main

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"text/template"

	"github.com/gin-gonic/gin"
)

type FNDAppriseNotificationSink struct {
	config            FNDNotificationConfigurationMap
	webServer         *FNDWebServer
	lastStatusMessage string
}

type AppriseTemplatePayload struct {
	Active          bool
	AppriseConfigID string
	ShowStatus      bool
	Color           string
	StatusMessage   string
	TranslatedText  []string
}

func (apprise *FNDAppriseNotificationSink) createDefaultConfig() {
	apprise.config = NEWDefaultFNDNotificationConfigurationMap()
	apprise.config.Map["enabled"] = "false"
}

func (apprise *FNDAppriseNotificationSink) getName() string {
	return "Apprise"
}

func (apprise *FNDAppriseNotificationSink) setup(conf FNDNotificationConfigurationMap, avail bool) error {
	if avail {
		apprise.config = conf
	} else {
		apprise.createDefaultConfig()
	}
	apprise.lastStatusMessage = "init"
	return nil
}

func (apprise *FNDAppriseNotificationSink) registerWebServer(webServer *FNDWebServer) {
	apprise.webServer = webServer

	apprise.webServer.r.GET("/htmx/apprise.html", func(c *gin.Context) {
		t := template.Must(template.ParseFS(templateFS, "templates/apprise.html"))
		t.Execute(c.Writer, apprise.generatePayload(false))
	})

	apprise.webServer.r.POST("/htmx/apprise.html", func(c *gin.Context) {
		apprise.config.Map["enabled"] = "false"
		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "appriseConfigID" {
				if value[0] == "" {
					continue
				}
				apprise.config.Map["configID"] = value[0]
				continue
			}
			if key == "aktiv" {
				if value[0] == "" {
					continue
				}
				apprise.config.Map["enabled"] = "true"
				continue
			}
		}

		t := template.Must(template.ParseFS(templateFS, "templates/apprise.html"))
		t.Execute(c.Writer, apprise.generatePayload(true))
	})

}

func (apprise *FNDAppriseNotificationSink) generatePayload(postReq bool) AppriseTemplatePayload {
	en, _ := apprise.config.Map["enabled"]
	var en_bool bool
	if en == "" || en == "false" {
		en_bool = false
	} else {
		en_bool = true
	}

	pay := AppriseTemplatePayload{
		Active:          en_bool,
		AppriseConfigID: apprise.config.Map["configID"],
		TranslatedText: []string{
			apprise.webServer.translation.lookupToken("active"),
			apprise.webServer.translation.lookupToken("confID"),
			apprise.webServer.translation.lookupToken("apprise_doc"),
			apprise.webServer.translation.lookupToken("apply"),
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

func (apprise *FNDAppriseNotificationSink) sendNotification(n FNDNotification) error {
	if apprise.config.Map["enabled"] != "true" {
		apprise.lastStatusMessage = "disabled"
		return nil
	}
	if apprise.config.Map["configID"] == "" {
		apprise.lastStatusMessage = "Configuration ID is empty!"
		return errors.New("Configuration ID is empty!")
	}

	//TODO: make this configurable
	url := "http://apprise:8000/notify/" + apprise.config.Map["configID"]
	var requestBody bytes.Buffer
	var err error
	writer := multipart.NewWriter(&requestBody)

	err = writer.WriteField("body", n.Caption+"   "+n.Date)
	if err != nil {
		return err
	}

	jpedDataReader := bytes.NewReader(n.JpegData)

	fileWriter, err := writer.CreateFormFile("attach", "Screenshot.jpeg")
	if err != nil {
		return err
	}

	_, err = io.Copy(fileWriter, jpedDataReader)
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		apprise.lastStatusMessage = "Rückgabewert falsch"
		return nil
	}
	apprise.lastStatusMessage = "Online"
	return nil
}

func (apprise *FNDAppriseNotificationSink) remove() (FNDNotificationConfigurationMap, error) {
	return apprise.config, nil
}

func (apprise *FNDAppriseNotificationSink) getConfiguration() FNDNotificationConfigurationMap {
	return apprise.config
}

func (apprise *FNDAppriseNotificationSink) getStatus() FNDNotificationSinkStatus {
	return FNDNotificationSinkStatus{
		Name:    apprise.getName(),
		Good:    apprise.lastStatusMessage == "Online",
		Message: apprise.lastStatusMessage,
	}
}
