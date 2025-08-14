package main

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
)

type FNDAppriseNotificationSink struct {
	config            FNDNotificationConfigurationMap
	appriseConfig     *AppriseConfig
	webServer         *FNDWebServer
	lastStatusMessage string
}

type AppriseTemplatePayload struct {
	Active          bool
	AppriseConfigID string
	ServerURL       string
	Timeout         int
	Format          string
	ShowStatus      bool
	Color           string
	StatusMessage   string
	TranslatedText  []string
}

func (apprise *FNDAppriseNotificationSink) createDefaultConfig() {
	apprise.config = NEWDefaultFNDNotificationConfigurationMap()
	defaultApprise := NewDefaultAppriseConfig()
	apprise.config.Map = defaultApprise.ToMap()
}

// updateConfig synchronizes changes back to the legacy map format
func (apprise *FNDAppriseNotificationSink) updateConfig() {
	if apprise.appriseConfig != nil {
		apprise.config.Map = apprise.appriseConfig.ToMap()
	}
}

func (apprise *FNDAppriseNotificationSink) getName() string {
	return "Apprise"
}

func (apprise *FNDAppriseNotificationSink) setup(conf FNDNotificationConfigurationMap, avail bool) error {
	if avail {
		apprise.config = conf
		// Convert from legacy map format to structured config if needed
		apprise.appriseConfig = &AppriseConfig{}
		apprise.appriseConfig.FromMap(conf.Map)
	} else {
		apprise.createDefaultConfig()
		apprise.appriseConfig = &AppriseConfig{}
		*apprise.appriseConfig = NewDefaultAppriseConfig()
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
		apprise.appriseConfig.Enabled = false
		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "appriseConfigID" {
				if value[0] == "" {
					continue
				}
				apprise.appriseConfig.ConfigID = value[0]
				continue
			}
			if key == "serverURL" {
				if value[0] == "" {
					continue
				}
				apprise.appriseConfig.ServerURL = value[0]
				continue
			}
			if key == "timeout" {
				if value[0] == "" {
					continue
				}
				if timeout, err := strconv.Atoi(value[0]); err == nil && timeout > 0 && timeout <= 300 {
					apprise.appriseConfig.Timeout = timeout
				}
				continue
			}
			if key == "format" {
				if value[0] == "" {
					continue
				}
				if value[0] == "text" || value[0] == "markdown" || value[0] == "html" {
					apprise.appriseConfig.Format = value[0]
				}
				continue
			}
			if key == "active" {
				if value[0] == "" {
					continue
				}
				apprise.appriseConfig.Enabled = true
				continue
			}
		}

		// Synchronize back to legacy format for compatibility
		apprise.updateConfig()

		t := template.Must(template.ParseFS(templateFS, "templates/apprise.html"))
		t.Execute(c.Writer, apprise.generatePayload(true))
	})

}

func (apprise *FNDAppriseNotificationSink) generatePayload(postReq bool) AppriseTemplatePayload {
	pay := AppriseTemplatePayload{
		Active:          apprise.appriseConfig.Enabled,
		AppriseConfigID: apprise.appriseConfig.ConfigID,
		ServerURL:       apprise.appriseConfig.ServerURL,
		Timeout:         apprise.appriseConfig.Timeout,
		Format:          apprise.appriseConfig.Format,
		TranslatedText: []string{
			apprise.webServer.translation.lookupToken("active"),
			apprise.webServer.translation.lookupToken("confID"),
			apprise.webServer.translation.lookupToken("apprise_doc"),
			apprise.webServer.translation.lookupToken("apply"),
			apprise.webServer.translation.lookupToken("server_url"),
			apprise.webServer.translation.lookupToken("timeout"),
			apprise.webServer.translation.lookupToken("format"),
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
	if !apprise.appriseConfig.Enabled {
		apprise.lastStatusMessage = "disabled"
		return nil
	}
	if apprise.appriseConfig.ConfigID == "" {
		apprise.lastStatusMessage = "Configuration ID is empty!"
		return errors.New("Configuration ID is empty!")
	}

	url := apprise.appriseConfig.ServerURL + "/notify/" + apprise.appriseConfig.ConfigID
	var requestBody bytes.Buffer
	var err error
	writer := multipart.NewWriter(&requestBody)

	// Add video URL to caption if available
	body := n.Caption + "   " + n.Date
	if n.HasVideo && n.VideoURL != "" {
		body += "\n🎥 Video: " + n.VideoURL
	}

	err = writer.WriteField("body", body)
	if err != nil {
		return err
	}

	// Attach image
	jpedDataReader := bytes.NewReader(n.JpegData)
	fileWriter, err := writer.CreateFormFile("attach", "Screenshot.jpeg")
	if err != nil {
		return err
	}

	_, err = io.Copy(fileWriter, jpedDataReader)
	if err != nil {
		return err
	}

	// Attach video if we have video data
	if n.HasVideo && len(n.VideoData) > 0 {
		videoDataReader := bytes.NewReader(n.VideoData)
		videoWriter, err := writer.CreateFormFile("attach", "clip.mp4")
		if err != nil {
			return err
		}

		_, err = io.Copy(videoWriter, videoDataReader)
		if err != nil {
			return err
		}
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

	client := &http.Client{
		Timeout: time.Duration(apprise.appriseConfig.Timeout) * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		apprise.lastStatusMessage = "Invalid return value"
		return nil
	}
	if n.HasVideo && len(n.VideoData) > 0 {
		apprise.lastStatusMessage = "Online (with video)"
	} else {
		apprise.lastStatusMessage = "Online"
	}
	return nil
}

func (apprise *FNDAppriseNotificationSink) remove() (FNDNotificationConfigurationMap, error) {
	return apprise.config, nil
}

func (apprise *FNDAppriseNotificationSink) getConfiguration() FNDNotificationConfigurationMap {
	// Ensure legacy map is up to date
	apprise.updateConfig()
	return apprise.config
}

// GetAppriseConfig returns the structured Apprise configuration
func (apprise *FNDAppriseNotificationSink) GetAppriseConfig() *AppriseConfig {
	return apprise.appriseConfig
}

// SetAppriseConfig updates the structured Apprise configuration
func (apprise *FNDAppriseNotificationSink) SetAppriseConfig(config *AppriseConfig) {
	apprise.appriseConfig = config
	apprise.updateConfig()
}

func (apprise *FNDAppriseNotificationSink) getStatus() FNDNotificationSinkStatus {
	return FNDNotificationSinkStatus{
		Name:    apprise.getName(),
		Good:    apprise.lastStatusMessage == "Online",
		Message: apprise.lastStatusMessage,
	}
}
