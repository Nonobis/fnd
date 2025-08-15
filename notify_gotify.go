package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
)

type FNDGotifyNotificationSink struct {
	config            FNDNotificationConfigurationMap
	webServer         *FNDWebServer
	lastStatusMessage string
}

type GotifyTemplatePayload struct {
	Active         bool
	ServerURL      string
	AppToken       string
	ShowStatus     bool
	Color          string
	StatusMessage  string
	TranslatedText []string
}

type GotifyMessage struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

func (gotify *FNDGotifyNotificationSink) createDefaultConfig() {
	gotify.config = NEWDefaultFNDNotificationConfigurationMap()
	gotify.config.Map["enabled"] = "false"
	gotify.config.Map["serverurl"] = "http://gotify:80"
	gotify.config.Map["apptoken"] = ""
	gotify.config.Map["priority"] = "5"
}

func (gotify *FNDGotifyNotificationSink) getName() string {
	return "Gotify"
}

func (gotify *FNDGotifyNotificationSink) setup(conf FNDNotificationConfigurationMap, avail bool) error {
	if avail {
		gotify.config = conf
	} else {
		gotify.createDefaultConfig()
	}
	gotify.lastStatusMessage = "init"
	return nil
}

func (gotify *FNDGotifyNotificationSink) registerWebServer(webServer *FNDWebServer) {
	gotify.webServer = webServer

	gotify.webServer.r.GET("/htmx/gotify.html", func(c *gin.Context) {
		t := template.Must(template.ParseFS(templateFS, "templates/gotify.html"))
		t.Execute(c.Writer, gotify.generatePayload(false))
	})

	gotify.webServer.r.POST("/htmx/gotify/toggle", func(c *gin.Context) {
		// Toggle the enabled status
		if gotify.config.Map["enabled"] == "true" {
			gotify.config.Map["enabled"] = "false"
			gotify.lastStatusMessage = "disabled"
		} else {
			gotify.config.Map["enabled"] = "true"
			gotify.lastStatusMessage = "enabled"
		}

		// Return updated page
		t := template.Must(template.ParseFS(templateFS, "templates/gotify.html"))
		t.Execute(c.Writer, gotify.generatePayload(false))
	})

	gotify.webServer.r.POST("/htmx/gotify.html", func(c *gin.Context) {
		gotify.config.Map["enabled"] = "false"
		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "serverurl" {
				if value[0] == "" {
					continue
				}
				gotify.config.Map["serverurl"] = value[0]
				continue
			}
			if key == "apptoken" {
				if value[0] == "" {
					continue
				}
				gotify.config.Map["apptoken"] = value[0]
				continue
			}
			if key == "priority" {
				if value[0] == "" {
					continue
				}
				// Validate priority is between 0-10
				if priority, err := strconv.Atoi(value[0]); err == nil && priority >= 0 && priority <= 10 {
					gotify.config.Map["priority"] = value[0]
				}
				continue
			}
			if key == "active" {
				if value[0] == "" {
					continue
				}
				gotify.config.Map["enabled"] = "true"
				continue
			}
		}

		t := template.Must(template.ParseFS(templateFS, "templates/gotify.html"))
		t.Execute(c.Writer, gotify.generatePayload(true))
	})
}

func (gotify *FNDGotifyNotificationSink) generatePayload(postReq bool) GotifyTemplatePayload {
	en, _ := gotify.config.Map["enabled"]
	var en_bool bool
	if en == "" || en == "false" {
		en_bool = false
	} else {
		en_bool = true
	}

	serverURL, _ := gotify.config.Map["serverurl"]
	if serverURL == "" {
		serverURL = "http://gotify:80"
	}

	appToken, _ := gotify.config.Map["apptoken"]

	pay := GotifyTemplatePayload{
		Active:    en_bool,
		ServerURL: serverURL,
		AppToken:  appToken,
		TranslatedText: []string{
			gotify.webServer.translation.lookupToken("active"),
			gotify.webServer.translation.lookupToken("apply"),
			gotify.webServer.translation.lookupToken("gotify_doc"),
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

func (gotify *FNDGotifyNotificationSink) sendNotification(n FNDNotification) error {
	if gotify.config.Map["enabled"] != "true" {
		gotify.lastStatusMessage = "disabled"
		return nil
	}
	if gotify.config.Map["apptoken"] == "" {
		gotify.lastStatusMessage = "App token is empty!"
		return errors.New("App token is empty!")
	}
	if gotify.config.Map["serverurl"] == "" {
		gotify.lastStatusMessage = "Server URL is empty!"
		return errors.New("Server URL is empty!")
	}

	// Parse priority, default to 5 if invalid
	priority := 5
	if priorityStr, exists := gotify.config.Map["priority"]; exists {
		if p, err := strconv.Atoi(priorityStr); err == nil && p >= 0 && p <= 10 {
			priority = p
		}
	}

	// Create the message with optional video link
	messageText := fmt.Sprintf("%s\n\n%s", n.Caption, n.Date)
	if n.HasVideo && n.VideoURL != "" {
		messageText += "\n\n🎥 Video: " + n.VideoURL
	}

	message := GotifyMessage{
		Title:    "FND Notification",
		Message:  messageText,
		Priority: priority,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		gotify.lastStatusMessage = "Failed to marshal message: " + err.Error()
		return err
	}

	// Prepare the request
	url := gotify.config.Map["serverurl"] + "/message"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		gotify.lastStatusMessage = "Failed to create request: " + err.Error()
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", gotify.config.Map["apptoken"])

	// Send the request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		gotify.lastStatusMessage = "Failed to send notification: " + err.Error()
		return err
	}
	defer resp.Body.Close()

	// Read response body for error details
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		gotify.lastStatusMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	gotify.lastStatusMessage = "Online"
	return nil
}

func (gotify *FNDGotifyNotificationSink) remove() (FNDNotificationConfigurationMap, error) {
	return gotify.config, nil
}

func (gotify *FNDGotifyNotificationSink) getConfiguration() FNDNotificationConfigurationMap {
	return gotify.config
}

func (gotify *FNDGotifyNotificationSink) getStatus() FNDNotificationSinkStatus {
	return FNDNotificationSinkStatus{
		Name:    gotify.getName(),
		Good:    gotify.lastStatusMessage == "Online",
		Message: gotify.lastStatusMessage,
	}
}
