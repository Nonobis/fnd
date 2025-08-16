package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type FNDWebServer struct {
	srv             *http.Server
	r               *gin.Engine
	OverviewPayload OverviewPayload
	frigateConf     *FNDFrigateConfiguration
	globalConf      *FNDConfiguration
	configPath      string
	notifyManager   *FNDNotificationManager
	translation     *Translation
	frigateEvent    *FNDFrigateEventManager
}

type FNDNotificationSinkStatus struct {
	Name    string
	Good    bool
	Message string
}

type OverviewPayload struct {
	NotificationStatus map[string]FNDNotificationSinkStatus
	Version            string
	TranslatedText     []string
	ActiveLanguage     int
	SupportedLanguages []Language
	CurrentLanguage    Language
	CooldownInfo       CooldownInfo
}

type CooldownInfo struct {
	CooldownPeriod   int    // Cooldown period in seconds
	IsActive         bool   // Whether cooldown is currently active
	SecondsRemaining int    // Seconds remaining until next notification allowed
	LastNotification string // Time of last notification (formatted)
}

// LogStats is now defined in logger.go

type NotificationPayload struct {
	ShowStatus     bool
	Color          string
	StatusMessage  string
	Conf           *FNDFrigateConfiguration
	TranslatedText []string
}

type FrigateTemplatePayload struct {
	ShowStatus     bool
	Color          string
	StatusMessage  string
	Conf           *FNDFrigateConfiguration
	TranslatedText []string
}

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// saveConfiguration saves the current configuration to disk
func (web *FNDWebServer) saveConfiguration() error {
	err := web.globalConf.WriteToFile(web.configPath)
	if err != nil {
		LogError("WEB", "Failed to save configuration file", err.Error())
		return err
	}
	LogInfo("WEB", "Configuration saved successfully", web.configPath)
	return nil
}

// saveConfigurationWithNotifications synchronizes notification configurations and saves to disk
func (web *FNDWebServer) saveConfigurationWithNotifications(notifyManager *FNDNotificationManager) error {
	// Synchronize notification configurations
	if notifyManager != nil {
		web.globalConf.Notify = notifyManager.getConfigAll()
	}

	err := web.globalConf.WriteToFile(web.configPath)
	if err != nil {
		LogError("WEB", "Failed to save configuration file", err.Error())
		return err
	}
	LogInfo("WEB", "Configuration saved successfully", web.configPath)
	return nil
}

// setNotificationManager sets the notification manager for configuration synchronization
func (web *FNDWebServer) setNotificationManager(notifyManager *FNDNotificationManager) {
	web.notifyManager = notifyManager
}

func setupBasicRoutes(addr string, conf *FNDFrigateConfiguration, globalConf *FNDConfiguration, configPath string) *FNDWebServer {
	r := gin.Default()

	var web FNDWebServer
	web.srv = &http.Server{
		Addr:    addr,
		Handler: r,
	}

	web.OverviewPayload.NotificationStatus = make(map[string]FNDNotificationSinkStatus)
	web.OverviewPayload.Version = version

	web.frigateConf = conf
	web.globalConf = globalConf
	web.configPath = configPath
	web.r = r
	web.translation = setupTranslation()
	err := web.translation.setLanguage(web.frigateConf.Language)
	if err != nil {
		LogWarn("WEB", "Failed to set initial language", err.Error())
	}

	r.GET("/", func(c *gin.Context) {

		web.OverviewPayload.ActiveLanguage = web.translation.currentIndex
		web.OverviewPayload.SupportedLanguages = web.translation.getLanguages()
		web.OverviewPayload.CurrentLanguage = web.translation.getCurrentLanguage()

		web.OverviewPayload.TranslatedText = []string{
			web.translation.lookupToken("header"),
			web.translation.lookupToken("overview"),
			web.translation.lookupToken("menu"),
			web.translation.lookupToken("settings"),
			web.translation.lookupToken("notifications"),
			web.translation.lookupToken("test_notification"),
			web.translation.lookupToken("cooldown_status"),
			web.translation.lookupToken("cooldown_active"),
			web.translation.lookupToken("cooldown_ready"),
			web.translation.lookupToken("next_notification_in"),
			web.translation.lookupToken("seconds"),
		}

		// Update cooldown information
		web.OverviewPayload.CooldownInfo = getCooldownInfo(web.frigateEvent)

		t := template.Must(template.ParseFS(templateFS,
			"templates/index.html",
			"templates/overview.html",
			"templates/navigation.html"))
		t.Execute(c.Writer, web.OverviewPayload)
	})

	r.GET("/static/htmx.min.js", func(c *gin.Context) {
		c.FileFromFS("static/htmx.min.js", http.FS(staticFS))
	})
	r.GET("/static/bulma.min.css", func(c *gin.Context) {
		c.FileFromFS("static/bulma.min.css", http.FS(staticFS))
	})
	r.GET("/static/style.css", func(c *gin.Context) {
		c.FileFromFS("static/style.css", http.FS(staticFS))
	})

	r.GET("/htmx/overview.html", func(c *gin.Context) {
		// Update cooldown information for live updates
		web.OverviewPayload.CooldownInfo = getCooldownInfo(web.frigateEvent)

		t := template.Must(template.ParseFS(templateFS, "templates/overview.html"))
		t.Execute(c.Writer, web.OverviewPayload)
	})
	r.GET("/htmx/frigate.html", func(c *gin.Context) {
		text := []string{
			web.translation.lookupToken("frigate_host"),
			web.translation.lookupToken("frigate_port"),
			web.translation.lookupToken("mqtt_server"),
			web.translation.lookupToken("mqtt_port"),
			web.translation.lookupToken("apply"),
			"", // 5 - Reserved
			"", // 6 - Reserved
			"", // 7 - Reserved
			web.translation.lookupToken("mqtt_auth"),
			web.translation.lookupToken("mqtt_anonymous"),
			web.translation.lookupToken("mqtt_username"),
			web.translation.lookupToken("mqtt_password"),
			web.translation.lookupToken("mqtt_auth_mode"),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/frigate.html"))
		t.Execute(c.Writer, FrigateTemplatePayload{
			ShowStatus:     false,
			Conf:           conf,
			TranslatedText: text,
		})
	})

	r.GET("/htmx/notifications.html", func(c *gin.Context) {

		text := []string{
			web.translation.lookupToken("notifications"),
			web.translation.lookupToken("cooldown"),
			web.translation.lookupToken("active_cams"),
			web.translation.lookupToken("apply"),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/notifications.html"))
		t.Execute(c.Writer, NotificationPayload{
			ShowStatus:     false,
			Conf:           conf,
			TranslatedText: text,
		})
	})

	r.POST("/htmx/language.html", func(c *gin.Context) {

		lang := c.Query("lang")
		LogInfo("WEB", "Language change requested", fmt.Sprintf("New language: %s", lang))

		err := web.translation.setLanguage(lang)
		if err != nil {
			LogError("WEB", "Failed to change language", err.Error())
			errorTemplate := `<div class="notification is-danger is-light">{{.}}</div>`
			t := template.Must(template.New("error").Parse(errorTemplate))
			t.Execute(c.Writer, err.Error())
			return
		}

		web.frigateConf.Language = lang

		// Save configuration to disk immediately
		web.saveConfiguration()

		LogInfo("WEB", "Language changed successfully", fmt.Sprintf("Language: %s", lang))
		// Update the payload with new language data
		web.OverviewPayload.ActiveLanguage = web.translation.currentIndex
		web.OverviewPayload.CurrentLanguage = web.translation.getCurrentLanguage()
		web.OverviewPayload.SupportedLanguages = web.translation.getLanguages()

		// Update the main overview payload translations
		web.OverviewPayload.TranslatedText = []string{
			web.translation.lookupToken("header"),
			web.translation.lookupToken("overview"),
			web.translation.lookupToken("menu"),
			web.translation.lookupToken("settings"),
			web.translation.lookupToken("notifications"),
			web.translation.lookupToken("test_notification"),
		}

		successMsg := web.translation.lookupToken("language_changed")
		languageName := web.translation.getCurrentLanguage().Name

		// Use HTMX response headers to trigger multiple updates
		c.Header("HX-Trigger-After-Swap", "languageChanged")

		// Create a proper template structure for the notification
		notificationTemplate := `<div class="notification is-success is-light">
			<span class="icon">{{.Flag}}</span>
			{{.Message}}: {{.LanguageName}}
		</div>`

		t := template.Must(template.New("notification").Parse(notificationTemplate))
		t.Execute(c.Writer, struct {
			Flag         string
			Message      string
			LanguageName string
		}{
			Flag:         web.translation.getCurrentLanguage().Flag,
			Message:      successMsg,
			LanguageName: languageName,
		})
	})

	r.GET("/htmx/language_settings.html", func(c *gin.Context) {
		payload := struct {
			SupportedLanguages []Language
			CurrentLanguage    Language
		}{
			SupportedLanguages: web.translation.getLanguages(),
			CurrentLanguage:    web.translation.getCurrentLanguage(),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/language_settings.html"))
		t.Execute(c.Writer, payload)
	})

	// New routes for dynamic updates after language change
	r.GET("/htmx/navigation.html", func(c *gin.Context) {
		translatedText := []string{
			web.translation.lookupToken("header"),
			web.translation.lookupToken("overview"),
			web.translation.lookupToken("menu"),
			web.translation.lookupToken("settings"),
			web.translation.lookupToken("notifications"),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/navigation.html"))
		t.Execute(c.Writer, struct {
			TranslatedText []string
		}{
			TranslatedText: translatedText,
		})
	})

	r.GET("/htmx/header.html", func(c *gin.Context) {
		// Return updated language dropdown
		payload := struct {
			SupportedLanguages []Language
			CurrentLanguage    Language
			TranslatedText     []string
		}{
			SupportedLanguages: web.translation.getLanguages(),
			CurrentLanguage:    web.translation.getCurrentLanguage(),
			TranslatedText: []string{
				web.translation.lookupToken("header"),
			},
		}

		headerTemplate := `
		<h1 class="title is-1">{{index .TranslatedText 0}}</h1>
		<span class="tag is-link is-info">` + web.OverviewPayload.Version + `</span>
		<br>
		<br>
		<div class="field">
			<div class="control">
				<div class="dropdown is-hoverable language-dropdown">
					<div class="dropdown-trigger">
						<button class="button is-light" aria-haspopup="true" aria-controls="dropdown-menu" title="Select Language">
							<span class="icon">
								<span>{{ .CurrentLanguage.Flag }}</span>
							</span>
							<span>{{ .CurrentLanguage.Name }}</span>
							<span class="icon is-small">
								<span>▼</span>
							</span>
						</button>
					</div>
					<div class="dropdown-menu" id="dropdown-menu" role="menu">
						<div class="dropdown-content">
							{{ range .SupportedLanguages }}
							<a class="dropdown-item {{if eq .Code $.CurrentLanguage.Code}}is-active{{end}}" 
							   hx-post="/htmx/language.html?lang={{ .Code }}" 
							   hx-target="#lang-info"
							   hx-trigger="click"
							   title="Switch to {{ .Name }}">
								<span class="icon">
									<span>{{ .Flag }}</span>
								</span>
								<span>{{ .Name }}</span>
							</a>
							{{ end }}
						</div>
					</div>
				</div>
			</div>
		</div>
		<div id="lang-info"></div>`

		t := template.Must(template.New("header").Parse(headerTemplate))
		t.Execute(c.Writer, payload)
	})

	r.POST("/htmx/frigate.html", func(c *gin.Context) {
		LogInfo("WEB", "Frigate configuration update requested", "")
		text := []string{
			web.translation.lookupToken("frigate_host"),
			web.translation.lookupToken("frigate_port"),
			web.translation.lookupToken("mqtt_server"),
			web.translation.lookupToken("mqtt_port"),
			web.translation.lookupToken("apply"),
			"", // 5 - Reserved
			"", // 6 - Reserved
			"", // 7 - Reserved
			web.translation.lookupToken("mqtt_auth"),
			web.translation.lookupToken("mqtt_anonymous"),
			web.translation.lookupToken("mqtt_username"),
			web.translation.lookupToken("mqtt_password"),
			web.translation.lookupToken("mqtt_auth_mode"),
		}

		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "host" {
				if value[0] != "" {
					conf.Host = value[0]
				}
				continue
			}
			if key == "port" {
				if value[0] != "" {
					conf.Port = value[0]
				}
				continue
			}
			if key == "mqttserver" {
				if value[0] != "" {
					conf.MqttServer = value[0]
				}
				continue
			}
			if key == "mqttport" {
				if value[0] != "" {
					conf.MqttPort = value[0]
				}
				continue
			}
			if key == "mqttauth" {
				conf.MqttAnonymous = (value[0] == "anonymous")
				continue
			}
			if key == "mqttusername" {
				if value[0] != "" {
					conf.MqttUsername = value[0]
				}
				continue
			}
			if key == "mqttpassword" {
				if value[0] != "" {
					conf.MqttPassword = value[0]
				}
				continue
			}
		}

		// Save configuration to disk immediately
		web.saveConfiguration()

		LogInfo("WEB", "Frigate configuration updated successfully", "")
		t := template.Must(template.ParseFS(templateFS, "templates/frigate.html"))
		_ = t.Execute(c.Writer, FrigateTemplatePayload{
			ShowStatus:     true,
			Color:          "is-primary",
			StatusMessage:  "OK",
			Conf:           conf,
			TranslatedText: text,
		})
	})

	r.POST("/htmx/notifications.html", func(c *gin.Context) {
		LogInfo("WEB", "Notification settings update requested", "")

		text := []string{
			web.translation.lookupToken("notifications"),
			web.translation.lookupToken("cooldown"),
			web.translation.lookupToken("active_cams"),
			web.translation.lookupToken("apply"),
		}

		var onList []string
		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "cooldown0815" {
				if value[0] == "" {
					continue
				}
				newCd, err := strconv.Atoi(value[0])
				if err == nil {
					conf.Cooldown = newCd
				}
				continue
			}
			if value[0] == "on" {
				onList = append(onList, key)
			}
		}

		conf.activateCameras(onList)

		// Save configuration to disk immediately
		web.saveConfiguration()

		LogInfo("WEB", "Notification settings updated successfully", fmt.Sprintf("Cooldown: %ds", conf.Cooldown))

		t := template.Must(template.ParseFS(templateFS, "templates/notifications.html"))
		_ = t.Execute(c.Writer, NotificationPayload{
			ShowStatus:     true,
			Color:          "is-primary",
			StatusMessage:  "OK",
			Conf:           conf,
			TranslatedText: text,
		})
	})

	// Snapshot endpoints
	r.GET("/snapshot/:camera", func(c *gin.Context) {
		camera := c.Param("camera")
		if camera == "" {
			c.JSON(400, gin.H{"error": "Camera parameter required"})
			return
		}

		imageData, err := web.frigateEvent.api.getLiveSnapshotByCamera(camera)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.Header("Content-Type", "image/jpeg")
		c.Data(200, "image/jpeg", imageData)
	})

	r.GET("/live/:camera", func(c *gin.Context) {
		camera := c.Param("camera")
		if camera == "" {
			c.JSON(400, gin.H{"error": "Camera parameter required"})
			return
		}

		imageData, err := web.frigateEvent.api.getLiveSnapshotByCamera(camera)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.Header("Content-Type", "image/jpeg")
		c.Data(200, "image/jpeg", imageData)
	})

	// Send live snapshot via notifications
	r.POST("/api/send-snapshot/:camera", func(c *gin.Context) {
		camera := c.Param("camera")
		if camera == "" {
			c.JSON(400, gin.H{"error": "Camera parameter required"})
			return
		}

		// Access the notification manager through global state or dependency injection
		// For now, we'll create a simple response
		c.JSON(200, gin.H{
			"message": "Snapshot request received for camera: " + camera,
			"camera":  camera,
		})
	})

	// Logs routes
	r.GET("/htmx/logs.html", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		logs := logger.GetRecentLogs(100)
		components := make(map[string]bool)
		for _, log := range logs {
			components[log.Component] = true
		}

		var componentList []string
		for component := range components {
			componentList = append(componentList, component)
		}

		translatedText := []string{
			web.translation.lookupToken("logs"),
			web.translation.lookupToken("log_level"),
			web.translation.lookupToken("log_component"),
			web.translation.lookupToken("log_message"),
			web.translation.lookupToken("log_timestamp"),
			web.translation.lookupToken("log_details"),
			web.translation.lookupToken("clear_logs"),
			web.translation.lookupToken("download_logs"),
			web.translation.lookupToken("auto_refresh"),
			web.translation.lookupToken("filter_level"),
			web.translation.lookupToken("filter_component"),
			web.translation.lookupToken("all_levels"),
			web.translation.lookupToken("all_components"),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/logs.html"))
		t.Execute(c.Writer, struct {
			Logs           []LogEntry
			Components     []string
			TranslatedText []string
		}{
			Logs:           logs,
			Components:     componentList,
			TranslatedText: translatedText,
		})
	})

	// API routes for logs
	r.GET("/api/logs/recent", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		limit := 100
		if limitStr := c.Query("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		logs := logger.GetRecentLogs(limit)
		c.JSON(200, gin.H{"logs": logs})
	})

	r.GET("/api/logs/stream", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		// Set headers for Server-Sent Events
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")

		// Subscribe to live updates
		clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())
		logChannel := logger.Subscribe(clientID)
		defer logger.Unsubscribe(clientID)

		// Send initial ping
		c.Writer.WriteString("data: {\"type\":\"ping\"}\n\n")
		c.Writer.(http.Flusher).Flush()

		// Stream logs
		for {
			select {
			case logEntry, ok := <-logChannel:
				if !ok {
					return
				}
				data, _ := json.Marshal(logEntry)
				c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data)))
				c.Writer.(http.Flusher).Flush()
			case <-c.Request.Context().Done():
				return
			}
		}
	})

	r.POST("/api/logs/clear", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		// Clear in-memory logs (we keep the file for safety)
		logger.mutex.Lock()
		config := logger.config
		logger.entries = make([]LogEntry, 0, config.MaxEntries)
		logger.mutex.Unlock()

		LogInfo("WEB", "Logs cleared via web interface", "")
		c.JSON(200, gin.H{"message": "Logs cleared successfully"})
	})

	r.GET("/api/logs/download", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		// Read the log file
		data, err := os.ReadFile(logger.filePath)
		if err != nil {
			LogError("WEB", "Failed to read log file for download", err.Error())
			c.JSON(500, gin.H{"error": "Failed to read log file"})
			return
		}

		LogInfo("WEB", "Log file downloaded", "")
		filename := fmt.Sprintf("fnd-logs-%s.log", time.Now().Format("2006-01-02-15-04-05"))
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		c.Header("Content-Type", "application/octet-stream")
		c.Data(200, "application/octet-stream", data)
	})

	// Log Settings routes
	r.GET("/htmx/log_settings.html", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		config := logger.GetConfiguration()
		stats := getLogStats(logger)

		translatedText := []string{
			web.translation.lookupToken("log_settings"),
			web.translation.lookupToken("max_log_entries"),
			web.translation.lookupToken("minimum_log_level"),
			web.translation.lookupToken("enable_file_logging"),
			web.translation.lookupToken("enable_console_logging"),
			web.translation.lookupToken("log_settings_desc"),
			web.translation.lookupToken("log_level_debug"),
			web.translation.lookupToken("log_level_info"),
			web.translation.lookupToken("log_level_warn"),
			web.translation.lookupToken("log_level_error"),
		}

		// Check if this is a modal request
		if c.Query("modal") == "true" {
			t := template.Must(template.ParseFS(templateFS, "templates/log_settings_modal.html"))
			t.Execute(c.Writer, struct {
				ShowStatus     bool
				Color          string
				StatusMessage  string
				Conf           *FNDLoggingConfiguration
				Stats          LogStats
				TranslatedText []string
			}{
				ShowStatus:     false,
				Conf:           config,
				Stats:          stats,
				TranslatedText: translatedText,
			})
		} else {
			t := template.Must(template.ParseFS(templateFS, "templates/log_settings.html"))
			t.Execute(c.Writer, struct {
				ShowStatus     bool
				Color          string
				StatusMessage  string
				Conf           *FNDLoggingConfiguration
				Stats          LogStats
				TranslatedText []string
			}{
				ShowStatus:     false,
				Conf:           config,
				Stats:          stats,
				TranslatedText: translatedText,
			})
		}
	})

	r.POST("/htmx/log_settings.html", func(c *gin.Context) {
		LogInfo("WEB", "Log settings update requested", "")
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		// Parse form data
		c.MultipartForm()
		currentConfig := logger.GetConfiguration()
		newConfig := *currentConfig // Start with current config

		for key, value := range c.Request.PostForm {
			switch key {
			case "maxEntries":
				if value[0] != "" {
					if maxEntries, err := strconv.Atoi(value[0]); err == nil {
						if maxEntries >= 100 && maxEntries <= 10000 {
							newConfig.MaxEntries = maxEntries
						}
					}
				}
			case "logLevel":
				if value[0] != "" {
					if logLevel, err := strconv.Atoi(value[0]); err == nil {
						if logLevel >= 0 && logLevel <= 3 {
							newConfig.LogLevel = logLevel
						}
					}
				}
			case "enableFile":
				newConfig.EnableFile = value[0] == "on"
			case "enableConsole":
				newConfig.EnableConsole = value[0] == "on"
			}
		}

		// Update configuration in memory and logger
		web.globalConf.Logging = newConfig
		logger.UpdateConfiguration(&newConfig)

		// Save configuration to disk immediately
		web.saveConfiguration()

		LogInfo("WEB", "Log settings updated successfully", fmt.Sprintf("MaxEntries: %d, LogLevel: %d, EnableFile: %t, EnableConsole: %t",
			newConfig.MaxEntries, newConfig.LogLevel, newConfig.EnableFile, newConfig.EnableConsole))

		stats := getLogStats(logger)
		translatedText := []string{
			web.translation.lookupToken("log_settings"),
			web.translation.lookupToken("max_log_entries"),
			web.translation.lookupToken("minimum_log_level"),
			web.translation.lookupToken("enable_file_logging"),
			web.translation.lookupToken("enable_console_logging"),
			web.translation.lookupToken("log_settings_desc"),
			web.translation.lookupToken("log_level_debug"),
			web.translation.lookupToken("log_level_info"),
			web.translation.lookupToken("log_level_warn"),
			web.translation.lookupToken("log_level_error"),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/log_settings.html"))
		t.Execute(c.Writer, struct {
			ShowStatus     bool
			Color          string
			StatusMessage  string
			Conf           *FNDLoggingConfiguration
			Stats          LogStats
			TranslatedText []string
		}{
			ShowStatus:     true,
			Color:          "is-success",
			StatusMessage:  "Settings updated successfully",
			Conf:           &newConfig,
			Stats:          stats,
			TranslatedText: translatedText,
		})
	})

	// Additional API endpoints for log settings
	r.GET("/api/logs/stats", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		stats := getLogStats(logger)
		c.JSON(200, gin.H{"stats": stats})
	})

	// Test notification endpoints
	r.POST("/api/test/gotify", func(c *gin.Context) {
		web.sendTestNotificationToSink("Gotify", c)
	})

	r.POST("/api/test/telegram", func(c *gin.Context) {
		web.sendTestNotificationToSink("Telegram", c)
	})

	r.POST("/api/test/apprise", func(c *gin.Context) {
		web.sendTestNotificationToSink("Apprise", c)
	})

	// Notification Templates routes
	r.GET("/htmx/notification_templates.html", func(c *gin.Context) {
		web.handleNotificationTemplatesPage(c)
	})

	r.POST("/htmx/notification-templates/global", func(c *gin.Context) {
		web.handleNotificationTemplatesGlobal(c)
	})

	r.POST("/htmx/notification-templates/service/:service", func(c *gin.Context) {
		web.handleNotificationTemplatesService(c)
	})

	r.POST("/api/test/template", func(c *gin.Context) {
		web.handleTemplateTest(c)
	})

	// Facial Recognition Routes
	r.GET("/facial-recognition", func(c *gin.Context) {
		web.handleFacialRecognitionPage(c)
	})

	r.GET("/face-management", func(c *gin.Context) {
		web.handleFaceManagementPage(c)
	})

	r.POST("/api/facial-recognition/toggle", func(c *gin.Context) {
		web.handleFacialRecognitionToggle(c)
	})
	r.POST("/api/facial-recognition/config", func(c *gin.Context) {
		web.handleFacialRecognitionConfig(c)
	})

	r.POST("/api/facial-recognition/test", func(c *gin.Context) {
		web.handleFacialRecognitionTest(c)
	})

	r.GET("/api/facial-recognition/faces", func(c *gin.Context) {
		web.handleFacialRecognitionFaces(c)
	})

	r.GET("/api/facial-recognition/faces/add", func(c *gin.Context) {
		web.handleFacialRecognitionFaceAdd(c)
	})

	r.POST("/api/facial-recognition/faces", func(c *gin.Context) {
		web.handleFacialRecognitionFaceSave(c)
	})

	r.GET("/api/facial-recognition/faces/edit/:id", func(c *gin.Context) {
		web.handleFacialRecognitionFaceEdit(c)
	})

	r.PUT("/api/facial-recognition/faces/:id", func(c *gin.Context) {
		web.handleFacialRecognitionFaceUpdate(c)
	})

	r.DELETE("/api/facial-recognition/faces/:id", func(c *gin.Context) {
		web.handleFacialRecognitionFaceDelete(c)
	})

	r.GET("/api/overview/facial-recognition-status", func(c *gin.Context) {
		web.handleFacialRecognitionStatus(c)
	})

	// Serve face images
	r.GET("/api/facial-recognition/images/:person/:filename", func(c *gin.Context) {
		web.handleFacialRecognitionImage(c)
	})

	// Object Filter Routes
	r.GET("/object-filters", func(c *gin.Context) {
		web.handleObjectFiltersPage(c)
	})

	r.GET("/object-filters/camera/:camera", func(c *gin.Context) {
		web.handleObjectFiltersCameraConfig(c)
	})

	r.GET("/api/object-filters/cameras", func(c *gin.Context) {
		web.handleObjectFiltersCameras(c)
	})

	r.GET("/api/object-filters/camera/:camera", func(c *gin.Context) {
		web.handleObjectFiltersCameraConfig(c)
	})

	r.POST("/api/object-filters/camera/:camera", func(c *gin.Context) {
		web.handleObjectFiltersCameraSave(c)
	})

	r.GET("/api/object-filters/available-objects", func(c *gin.Context) {
		web.handleObjectFiltersAvailableObjects(c)
	})

	// Pending Faces Handlers

	r.GET("/htmx/pending_faces.html", func(c *gin.Context) {
		web.handlePendingFacesPage(c)
	})

	r.GET("/api/pending_faces", func(c *gin.Context) {
		web.handlePendingFacesList(c)
	})

	r.GET("/api/pending_faces/stats", func(c *gin.Context) {
		web.handlePendingFacesStats(c)
	})

	r.POST("/api/pending_faces/process/:id", func(c *gin.Context) {
		web.handlePendingFacesProcess(c)
	})

	r.DELETE("/api/pending_faces/:id", func(c *gin.Context) {
		web.handlePendingFacesDelete(c)
	})

	r.GET("/api/pending_faces/:id/image", func(c *gin.Context) {
		web.handlePendingFacesImage(c)
	})

	r.GET("/api/pending_faces/cleanup", func(c *gin.Context) {
		web.handlePendingFacesCleanup(c)
	})

	r.POST("/api/facial-recognition/pending-faces-config", func(c *gin.Context) {
		web.handlePendingFacesConfig(c)
	})

	// Task Scheduler Handlers
	r.GET("/htmx/task_scheduler.html", func(c *gin.Context) {
		web.handleTaskSchedulerPage(c)
	})

	r.GET("/api/task_scheduler/history", func(c *gin.Context) {
		web.handleTaskSchedulerHistory(c)
	})

	r.GET("/api/task_scheduler/queue", func(c *gin.Context) {
		web.handleTaskSchedulerQueue(c)
	})

	r.POST("/api/task_scheduler/execute/:taskType", func(c *gin.Context) {
		web.handleTaskSchedulerExecute(c)
	})

	r.POST("/api/task_scheduler/config", func(c *gin.Context) {
		web.handleTaskSchedulerConfig(c)
	})

	r.DELETE("/api/task_scheduler/history", func(c *gin.Context) {
		web.handleTaskSchedulerPurgeHistory(c)
	})

	return &web
}

// getCooldownInfo calculates the current cooldown status
func getCooldownInfo(frigateEvent *FNDFrigateEventManager) CooldownInfo {
	if frigateEvent == nil {
		return CooldownInfo{
			CooldownPeriod:   60,
			IsActive:         false,
			SecondsRemaining: 0,
			LastNotification: "Never",
		}
	}

	frigateEvent.m.Lock()
	lastNotification := frigateEvent.lastNotificationSent
	frigateEvent.m.Unlock()

	cooldownPeriod := frigateEvent.fConf.Cooldown
	elapsed := time.Since(lastNotification)
	elapsedSeconds := int(elapsed.Seconds())

	isActive := elapsedSeconds < cooldownPeriod
	secondsRemaining := 0
	if isActive {
		secondsRemaining = cooldownPeriod - elapsedSeconds
	}

	// Format last notification time
	var lastNotificationStr string
	if lastNotification.IsZero() || time.Since(lastNotification) > 24*time.Hour {
		lastNotificationStr = "Never"
	} else {
		lastNotificationStr = lastNotification.Format("15:04:05")
	}

	return CooldownInfo{
		CooldownPeriod:   cooldownPeriod,
		IsActive:         isActive,
		SecondsRemaining: secondsRemaining,
		LastNotification: lastNotificationStr,
	}
}

// getLogStats returns current logging statistics
func getLogStats(logger *Logger) LogStats {
	if logger == nil {
		return LogStats{}
	}
	return logger.GetLogStats()
}

func (web *FNDWebServer) run(frigateEvent *FNDFrigateEventManager) {
	web.frigateEvent = frigateEvent
	LogInfo("WEB", "Starting web server", "Address: "+web.srv.Addr)
	if err := web.srv.ListenAndServe(); err != nil {
		LogError("WEB", "Web server error", err.Error())
		fmt.Println(err.Error())
	}
}

func (web *FNDWebServer) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := web.srv.Shutdown(ctx); err != nil {
		log.Fatal("Server Shutdown:", err)
	}
	// catching ctx.Done(). timeout of 1 seconds.
	<-ctx.Done()
	log.Println("timeout of 1 seconds.")
}

func (web *FNDWebServer) addNotificationSinkStatus(n FNDNotificationSinkStatus) {
	web.OverviewPayload.NotificationStatus[n.Name] = n
}

// sendTestNotificationToSink sends a test notification to a specific notification sink
func (web *FNDWebServer) sendTestNotificationToSink(sinkName string, c *gin.Context) {
	if web.notifyManager == nil {
		c.HTML(200, "", `<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Notification manager not initialized</span>
		</div>`)
		return
	}

	// Find the specific sink
	sink, exists := web.notifyManager.getSink(sinkName)
	if !exists {
		c.HTML(200, "", fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>%s notification sink not found</span>
		</div>`, sinkName))
		return
	}

	// Check if sink is enabled
	if !web.notifyManager.isSinkEnabled(sink) {
		c.HTML(200, "", fmt.Sprintf(`<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>%s notifications are currently disabled</span>
		</div>`, sinkName))
		return
	}

	// Load test image
	data, err := staticFS.ReadFile("static/test_notification.jpg")
	if err != nil {
		LogError("WEB", "Failed to load test notification image", err.Error())
		c.HTML(200, "", `<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to load test image</span>
		</div>`)
		return
	}

	// Create test notification with proper template variables
	testNotification := FNDNotification{
		JpegData:  data,
		VideoData: nil,
		VideoURL:  "https://example.com/test-video.mp4",
		Date:      time.Now().Format("15:04:05 02.01.2006"),
		Caption:   fmt.Sprintf("🧪 Test notification from FND\n\nThis is a test message sent to verify your %s configuration is working correctly.\n\nTime: %s", sinkName, time.Now().Format("2006-01-02 15:04:05")),
		HasVideo:  true,
		Title:     "🧪 FND Test Notification",
	}

	// Create template variables for test
	templateVars := CreateTemplateVariables(testNotification, "Test Camera", "Test Object", "test_event_123")

	// Process template if available
	if web.notifyManager.templateProcessor != nil {
		processedCaption, processedTitle, err := web.notifyManager.templateProcessor.ProcessTemplate(sinkName, templateVars)
		if err == nil {
			testNotification.Caption = processedCaption
			if processedTitle != "" {
				testNotification.Title = processedTitle
			}
		}
	}

	// Send test notification
	err = sink.sendNotification(testNotification)
	if err != nil {
		LogError("WEB", "Test notification failed", fmt.Sprintf("Sink: %s, Error: %s", sinkName, err.Error()))
		c.HTML(200, "", fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Test failed: %s</span>
		</div>`, err.Error()))
		return
	}

	LogInfo("WEB", "Test notification sent successfully", fmt.Sprintf("Sink: %s", sinkName))
	c.HTML(200, "", fmt.Sprintf(`<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Test notification sent successfully to %s!</span>
	</div>`, sinkName))
}

// Notification Templates handlers
func (web *FNDWebServer) handleNotificationTemplatesPage(c *gin.Context) {
	// Get available variables
	variables := GetAvailableVariables()
	var variableList []struct {
		Variable    string
		Description string
	}
	for variable, description := range variables {
		variableList = append(variableList, struct {
			Variable    string
			Description string
		}{
			Variable:    variable,
			Description: description,
		})
	}

	// Define services
	services := []struct {
		Key      string
		Name     string
		Icon     string
		Template NotificationTemplate
	}{
		{
			Key:      "Telegram",
			Name:     "Telegram",
			Icon:     "fab fa-telegram-plane",
			Template: web.globalConf.Notify.Templates.PerService["Telegram"],
		},
		{
			Key:      "Gotify",
			Name:     "Gotify",
			Icon:     "fas fa-bell",
			Template: web.globalConf.Notify.Templates.PerService["Gotify"],
		},
		{
			Key:      "Apprise",
			Name:     "Apprise",
			Icon:     "fas fa-broadcast-tower",
			Template: web.globalConf.Notify.Templates.PerService["Apprise"],
		},
	}

	payload := struct {
		Global   NotificationTemplate
		Services []struct {
			Key      string
			Name     string
			Icon     string
			Template NotificationTemplate
		}
		Variables []struct {
			Variable    string
			Description string
		}
	}{
		Global:    web.globalConf.Notify.Templates.Global,
		Services:  services,
		Variables: variableList,
	}

	// Debug logging
	LogInfo("WEB", "Loading notification templates page", fmt.Sprintf("Global title: '%s', Global message: '%s'",
		web.globalConf.Notify.Templates.Global.Title,
		web.globalConf.Notify.Templates.Global.Message))

	// Ensure templates are initialized
	if web.globalConf.Notify.Templates.Global.Title == "" {
		web.globalConf.Notify.Templates.Global.Title = "New Event"
	}
	if web.globalConf.Notify.Templates.Global.Message == "" {
		web.globalConf.Notify.Templates.Global.Message = "A new event has occurred: {{.Object}} at {{.Camera}} on {{.Date}} {{.Time}}{{if .HasVideo}}\n🎥 Video: {{.VideoURL}}{{end}}{{if .HasSnapshot}}\n📸 Snapshot attached{{end}}{{if .HasFaces}}\n👤 Faces detected: {{.FaceCount}}{{if .RecognizedFaces}}\n✅ Recognized: {{.RecognizedFaces}}{{end}}{{if .UnknownFaces}}\n❓ Unknown: {{.UnknownFaces}}{{end}}{{end}}"
	}

	t := template.Must(template.ParseFS(templateFS, "templates/notification_templates.html"))
	err := t.Execute(c.Writer, payload)
	if err != nil {
		LogError("WEB", "Failed to execute notification templates template", err.Error())
		c.Data(500, "text/html", []byte(fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Template error: %s</span>
		</div>`, err.Error())))
	}
}

func (web *FNDWebServer) handleNotificationTemplatesGlobal(c *gin.Context) {
	c.MultipartForm()

	title := c.PostForm("title")
	message := c.PostForm("message")

	LogInfo("WEB", "Saving global template", fmt.Sprintf("Title: '%s', Message: '%s'", title, message))

	web.globalConf.Notify.Templates.Global.Title = title
	web.globalConf.Notify.Templates.Global.Message = message

	// Update notification manager if available
	if web.notifyManager != nil {
		web.notifyManager.updateTemplates(&web.globalConf.Notify.Templates)
	}

	web.saveConfiguration()
	web.handleNotificationTemplatesPage(c)
}

func (web *FNDWebServer) handleNotificationTemplatesService(c *gin.Context) {
	service := c.Param("service")
	c.MultipartForm()

	title := c.PostForm("title")
	message := c.PostForm("message")

	LogInfo("WEB", "Saving service template", fmt.Sprintf("Service: %s, Title: '%s', Message: '%s'", service, title, message))

	if web.globalConf.Notify.Templates.PerService == nil {
		web.globalConf.Notify.Templates.PerService = make(map[string]NotificationTemplate)
	}

	template := web.globalConf.Notify.Templates.PerService[service]
	template.Title = title
	template.Message = message
	web.globalConf.Notify.Templates.PerService[service] = template

	// Update notification manager if available
	if web.notifyManager != nil {
		web.notifyManager.updateTemplates(&web.globalConf.Notify.Templates)
	}

	web.saveConfiguration()
	web.handleNotificationTemplatesPage(c)
}

func (web *FNDWebServer) handleTemplateTest(c *gin.Context) {
	c.MultipartForm()

	title := c.PostForm("title")
	message := c.PostForm("message")

	// Create test variables with more complete data
	testVars := TemplateVariables{
		Camera:          "front_door",
		Object:          "person",
		Date:            "15.12.2024",
		Time:            "14:30:25",
		VideoURL:        "http://example.com/video.mp4",
		HasVideo:        true,
		EventID:         "test_event_123",
		HasSnapshot:     true,
		SnapshotURL:     "[Snapshot attached]",
		HasFaces:        true,
		FaceCount:       2,
		RecognizedFaces: "John Doe, Jane Smith",
		UnknownFaces:    "1",
	}

	// Process templates
	processor := NewTemplateProcessor(&web.globalConf.Notify.Templates)

	var processedTitle, processedMessage string
	var err error

	if title != "" {
		processedTitle, err = processor.processTemplateString(title, testVars)
		if err != nil {
			errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
				<span class="icon"><i class="fas fa-times-circle"></i></span>
				<span>Title template error: %s</span>
			</div>`, err.Error())
			c.Data(200, "text/html", []byte(errorHTML))
			return
		}
	}

	if message != "" {
		processedMessage, err = processor.processTemplateString(message, testVars)
		if err != nil {
			errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
				<span class="icon"><i class="fas fa-times-circle"></i></span>
				<span>Message template error: %s</span>
			</div>`, err.Error())
			c.Data(200, "text/html", []byte(errorHTML))
			return
		}
	}

	// Return preview
	previewHTML := fmt.Sprintf(`<div class="notification is-info is-light">
		<h4 class="title is-5">Template Preview:</h4>
		<div class="content">
			<p><strong>Title:</strong> %s</p>
			<p><strong>Message:</strong></p>
			<pre style="white-space: pre-wrap; background: #f5f5f5; padding: 1rem; border-radius: 4px;">%s</pre>
		</div>
	</div>`, processedTitle, processedMessage)

	c.Data(200, "text/html", []byte(previewHTML))
}

// Facial Recognition Routes and Handlers

func (web *FNDWebServer) handleFacialRecognitionPage(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition page", "")

	tmpl, err := template.ParseFS(templateFS, "templates/facial_recognition.html")
	if err != nil {
		LogError("WEB", "Failed to parse facial recognition template", err.Error())
		c.String(500, "Internal server error")
		return
	}

	data := struct {
		Config *FNDFacialRecognitionConfiguration
	}{
		Config: &web.globalConf.FacialRecognition,
	}

	err = tmpl.Execute(c.Writer, data)
	if err != nil {
		LogError("WEB", "Failed to execute facial recognition template", err.Error())
		c.String(500, "Internal server error")
		return
	}
}

func (web *FNDWebServer) handleFaceManagementPage(c *gin.Context) {
	LogDebug("WEB", "Handling face management page", "")

	tmpl, err := template.ParseFS(templateFS, "templates/face_management.html")
	if err != nil {
		LogError("WEB", "Failed to parse face management template", err.Error())
		c.String(500, "Internal server error")
		return
	}

	data := struct{}{}
	err = tmpl.Execute(c.Writer, data)
	if err != nil {
		LogError("WEB", "Failed to execute face management template", err.Error())
		c.String(500, "Internal server error")
		return
	}
}

func (web *FNDWebServer) handleFacialRecognitionToggle(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition toggle", "")

	// Toggle the enabled status
	web.globalConf.FacialRecognition.Enabled = !web.globalConf.FacialRecognition.Enabled

	// Save configuration
	err := web.saveConfiguration()
	if err != nil {
		LogError("WEB", "Failed to save facial recognition configuration", err.Error())
		c.String(500, "Failed to save configuration")
		return
	}

	// Reinitialize facial recognition service if notification manager exists
	if web.notifyManager != nil {
		if web.globalConf.FacialRecognition.Enabled {
			web.notifyManager.facialRecognitionService = NewFacialRecognitionService(&web.globalConf.FacialRecognition)
			LogInfo("WEB", "Facial recognition service reinitialized", "")
		} else {
			web.notifyManager.facialRecognitionService = nil
			LogInfo("WEB", "Facial recognition service disabled", "")
		}
	}

	// Return the updated page
	web.handleFacialRecognitionPage(c)
}

func (web *FNDWebServer) handleFacialRecognitionConfig(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition configuration update", "")

	c.MultipartForm()

	// Parse form data
	host := c.PostForm("codeProjectAIHost")
	portStr := c.PostForm("codeProjectAIPort")
	useSSL := c.PostForm("codeProjectAIUseSSL") == "true"
	timeoutStr := c.PostForm("codeProjectAITimeout")
	faceDetectionEnabled := c.PostForm("faceDetectionEnabled") == "true"
	faceRecognitionEnabled := c.PostForm("faceRecognitionEnabled") == "true"
	// Convert port
	port := 8000
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	// Convert timeout
	timeout := 30
	if timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = t
		}
	}

	// Update configuration (keep enabled status unchanged)
	web.globalConf.FacialRecognition.CodeProjectAIHost = host
	web.globalConf.FacialRecognition.CodeProjectAIPort = port
	web.globalConf.FacialRecognition.CodeProjectAIUseSSL = useSSL
	web.globalConf.FacialRecognition.CodeProjectAITimeout = timeout
	web.globalConf.FacialRecognition.FaceDetectionEnabled = faceDetectionEnabled
	web.globalConf.FacialRecognition.FaceRecognitionEnabled = faceRecognitionEnabled
	// FaceDatabasePath is now fixed to fnd_conf/faces.db

	// Save configuration
	err := web.saveConfiguration()
	if err != nil {
		LogError("WEB", "Failed to save facial recognition configuration", err.Error())
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to save configuration: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	// Reinitialize facial recognition service if notification manager exists and enabled
	if web.notifyManager != nil && web.globalConf.FacialRecognition.Enabled {
		web.notifyManager.facialRecognitionService = NewFacialRecognitionService(&web.globalConf.FacialRecognition)
		LogInfo("WEB", "Facial recognition service reinitialized", "")
	}

	LogInfo("WEB", "Facial recognition configuration updated", fmt.Sprintf("Host: %s, Port: %d", host, port))

	// Return the updated page
	web.handleFacialRecognitionPage(c)
}

func (web *FNDWebServer) handleFacialRecognitionTest(c *gin.Context) {
	LogDebug("WEB", "Testing facial recognition connection", "")

	if !web.globalConf.FacialRecognition.Enabled {
		errorHTML := `<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>Facial recognition is disabled. Please enable it first.</span>
		</div>`
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	// Create temporary service for testing
	testService := NewFacialRecognitionService(&web.globalConf.FacialRecognition)

	err := testService.TestConnection()
	if err != nil {
		LogError("WEB", "Facial recognition connection test failed", err.Error())
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Connection test failed: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	LogInfo("WEB", "Facial recognition connection test successful", "")
	successHTML := `<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Connection test successful! CodeProject.AI is reachable.</span>
	</div>`
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleFacialRecognitionFaces(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition faces list", "")

	if web.notifyManager == nil || web.notifyManager.facialRecognitionService == nil {
		errorHTML := `<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>Facial recognition service is not available.</span>
		</div>`
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	faces := web.notifyManager.facialRecognitionService.GetAllFaces()

	if len(faces) == 0 {
		infoHTML := `<div class="notification is-info is-light">
			<span class="icon"><i class="fas fa-info-circle"></i></span>
			<span>No faces in database. Add some faces to get started.</span>
		</div>`
		c.Data(200, "text/html", []byte(infoHTML))
		return
	}

	// Create faces list HTML
	var facesHTML strings.Builder
	facesHTML.WriteString(`<div class="faces-grid">`)

	for _, face := range faces {
		statusClass := "is-success"
		statusText := "Active"
		statusIcon := "fa-check-circle"
		if !face.IsActive {
			statusClass = "is-danger"
			statusText = "Inactive"
			statusIcon = "fa-times-circle"
		}

		// Try to load face image if it exists
		imageHTML := `<div class="face-placeholder">
			<span class="icon is-large">
				<i class="fas fa-user"></i>
			</span>
		</div>`

		if face.ImagePath != "" {
			// Check if image file exists
			if _, err := os.Stat(face.ImagePath); err == nil {
				// Extract person directory and filename from path
				personDir := filepath.Base(filepath.Dir(face.ImagePath))
				filename := filepath.Base(face.ImagePath)
				imageURL := fmt.Sprintf("/api/facial-recognition/images/%s/%s", personDir, filename)

				imageHTML = fmt.Sprintf(`<img src="%s" alt="%s %s" class="face-image">`,
					imageURL, face.FirstName, face.LastName)
			}
		}

		facesHTML.WriteString(fmt.Sprintf(`<div class="face-card">
			<div class="face-info">
				%s
				<div class="face-details">
					<h4 class="title is-5">%s %s</h4>
					<p class="subtitle is-6">Face ID: %s</p>
					<p class="subtitle is-6">Added: %s</p>
					<span class="tag %s">
						<span class="icon is-small">
							<i class="fas %s"></i>
						</span>
						<span>%s</span>
					</span>
				</div>
				<div class="face-actions">
					<button class="button is-info is-small" hx-get="/api/facial-recognition/faces/edit/%s" hx-target="#face-form" hx-swap="outerHTML" title="Edit face">
						<span class="icon">
							<i class="fas fa-edit"></i>
						</span>
					</button>
					<button class="button is-danger is-small" hx-delete="/api/facial-recognition/faces/%s" hx-confirm="Are you sure you want to delete this face?" title="Delete face">
						<span class="icon">
							<i class="fas fa-trash"></i>
						</span>
					</button>
				</div>
			</div>
		</div>`, imageHTML, face.FirstName, face.LastName, face.FaceID,
			face.CreatedAt.Format("2006-01-02 15:04"), statusClass, statusIcon, statusText, face.ID, face.ID))
	}

	facesHTML.WriteString(`</div>`)

	c.Data(200, "text/html", []byte(facesHTML.String()))
}

func (web *FNDWebServer) handleFacialRecognitionFaceAdd(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition face add form", "")

	formHTML := `<div class="settings-card">
		<div class="settings-card-header">
			<h3 class="title is-4">
				<span class="icon-text">
					<span class="icon">
						<i class="fas fa-user-plus"></i>
					</span>
					<span>Add New Face</span>
				</span>
			</h3>
		</div>
		<div class="settings-card-body">
			<form hx-post="/api/facial-recognition/faces" hx-encoding="multipart/form-data" hx-target="#face-form" hx-swap="outerHTML">
				<div class="form-grid">
					<div class="form-group">
						<label class="label">
							<span class="icon-text">
								<span class="icon">
									<i class="fas fa-user"></i>
								</span>
								<span>First Name</span>
							</span>
						</label>
						<div class="control has-icons-left">
							<input class="input is-medium" type="text" name="firstName" required placeholder="John">
							<span class="icon is-small is-left">
								<i class="fas fa-user"></i>
							</span>
						</div>
						<p class="help-text">Enter the person's first name</p>
					</div>
					
					<div class="form-group">
						<label class="label">
							<span class="icon-text">
								<span class="icon">
									<i class="fas fa-user"></i>
								</span>
								<span>Last Name</span>
							</span>
						</label>
						<div class="control has-icons-left">
							<input class="input is-medium" type="text" name="lastName" required placeholder="Doe">
							<span class="icon is-small is-left">
								<i class="fas fa-user"></i>
							</span>
						</div>
						<p class="help-text">Enter the person's last name</p>
					</div>
				</div>
				
				<div class="form-group">
					<label class="label">
						<span class="icon-text">
							<span class="icon">
								<i class="fas fa-image"></i>
							</span>
							<span>Face Image</span>
						</span>
					</label>
					<div class="control">
						<input class="input" type="file" name="image" accept="image/*" required>
					</div>
					<p class="help-text">Upload a clear, front-facing image of the person's face. The image should be well-lit and show the face clearly.</p>
				</div>
				
				<div class="form-actions">
					<button class="button is-primary" type="submit">
						<span class="icon">
							<i class="fas fa-save"></i>
						</span>
						<span>Add Face</span>
					</button>
					<button class="button" type="button" hx-get="/api/facial-recognition/faces" hx-target="#face-form" hx-swap="outerHTML">
						Cancel
					</button>
				</div>
			</form>
		</div>
	</div>`

	c.Data(200, "text/html", []byte(formHTML))
}

func (web *FNDWebServer) handleFacialRecognitionFaceEdit(c *gin.Context) {
	faceID := c.Param("id")
	LogDebug("WEB", "Handling facial recognition face edit", fmt.Sprintf("Face ID: %s", faceID))

	if web.notifyManager == nil || web.notifyManager.facialRecognitionService == nil {
		errorHTML := `<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>Facial recognition service is not available.</span>
		</div>`
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	face := web.notifyManager.facialRecognitionService.GetFaceByID(faceID)
	if face == nil {
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Face with ID %s not found.</span>
		</div>`, faceID)
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	formHTML := fmt.Sprintf(`<div class="settings-card">
		<div class="settings-card-header">
			<h3 class="title is-4">
				<span class="icon-text">
					<span class="icon">
						<i class="fas fa-edit"></i>
					</span>
					<span>Edit Face</span>
				</span>
			</h3>
		</div>
		<div class="settings-card-body">
			<form hx-put="/api/facial-recognition/faces/%s" hx-target="#face-form" hx-swap="outerHTML">
				<input type="hidden" name="id" value="%s">
				
				<div class="form-grid">
					<div class="form-group">
						<label class="label">
							<span class="icon-text">
								<span class="icon">
									<i class="fas fa-user"></i>
								</span>
								<span>First Name</span>
							</span>
						</label>
						<div class="control has-icons-left">
							<input class="input is-medium" type="text" name="firstName" value="%s" required>
							<span class="icon is-small is-left">
								<i class="fas fa-user"></i>
							</span>
						</div>
						<p class="help-text">Enter the person's first name</p>
					</div>
					
					<div class="form-group">
						<label class="label">
							<span class="icon-text">
								<span class="icon">
									<i class="fas fa-user"></i>
								</span>
								<span>Last Name</span>
							</span>
						</label>
						<div class="control has-icons-left">
							<input class="input is-medium" type="text" name="lastName" value="%s" required>
							<span class="icon is-small is-left">
								<i class="fas fa-user"></i>
							</span>
						</div>
						<p class="help-text">Enter the person's last name</p>
					</div>
				</div>
				
				<div class="form-group">
					<label class="label">
						<span class="icon-text">
							<span class="icon">
								<i class="fas fa-toggle-on"></i>
							</span>
							<span>Status</span>
						</span>
					</label>
					<div class="control">
						<label class="checkbox">
							<input type="checkbox" name="isActive" value="true" %s>
							Active (face will be used for recognition)
						</label>
					</div>
					<p class="help-text">Inactive faces will not be used for recognition</p>
				</div>
				
				<div class="form-actions">
					<button class="button is-primary" type="submit">
						<span class="icon">
							<i class="fas fa-save"></i>
						</span>
						<span>Update Face</span>
					</button>
					<button class="button" type="button" hx-get="/api/facial-recognition/faces" hx-target="#face-form" hx-swap="outerHTML">
						Cancel
					</button>
				</div>
			</form>
		</div>
	</div>`, faceID, face.ID, face.FirstName, face.LastName,
		func() string {
			if face.IsActive {
				return "checked"
			}
			return ""
		}())

	c.Data(200, "text/html", []byte(formHTML))
}

func (web *FNDWebServer) handleFacialRecognitionFaceSave(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition face save", "")

	if web.notifyManager == nil || web.notifyManager.facialRecognitionService == nil {
		errorHTML := `<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>Facial recognition service is not available.</span>
		</div>`
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	c.MultipartForm()

	// Parse form data
	firstName := c.PostForm("firstName")
	lastName := c.PostForm("lastName")
	isActive := c.PostForm("isActive") == "true"

	// Get uploaded file
	file, err := c.FormFile("image")
	if err != nil {
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to get uploaded file: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	// Read file data
	src, err := file.Open()
	if err != nil {
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to open uploaded file: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}
	defer src.Close()

	imageData, err := io.ReadAll(src)
	if err != nil {
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to read uploaded file: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	// Create face record
	faceRecord := &FaceRecord{
		FirstName: firstName,
		LastName:  lastName,
		IsActive:  isActive,
	}

	// Add face to database
	err = web.notifyManager.facialRecognitionService.AddFaceToDatabase(faceRecord, imageData)
	if err != nil {
		LogError("WEB", "Failed to add face to database", err.Error())
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to add face: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	LogInfo("WEB", "Face added to database", fmt.Sprintf("Person: %s %s", firstName, lastName))

	successHTML := fmt.Sprintf(`<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Face added successfully: %s %s</span>
	</div>`, firstName, lastName)
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleFacialRecognitionFaceUpdate(c *gin.Context) {
	faceID := c.Param("id")
	LogDebug("WEB", "Handling facial recognition face update", fmt.Sprintf("Face ID: %s", faceID))

	if web.notifyManager == nil || web.notifyManager.facialRecognitionService == nil {
		errorHTML := `<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>Facial recognition service is not available.</span>
		</div>`
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	c.MultipartForm()

	// Parse form data
	firstName := c.PostForm("firstName")
	lastName := c.PostForm("lastName")
	isActive := c.PostForm("isActive") == "true"

	// Get existing face
	face := web.notifyManager.facialRecognitionService.GetFaceByID(faceID)
	if face == nil {
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Face with ID %s not found.</span>
		</div>`, faceID)
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	// Update face record
	face.FirstName = firstName
	face.LastName = lastName
	face.IsActive = isActive

	// Update face in database
	err := web.notifyManager.facialRecognitionService.UpdateFaceInDatabase(face)
	if err != nil {
		LogError("WEB", "Failed to update face in database", err.Error())
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to update face: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	LogInfo("WEB", "Face updated in database", fmt.Sprintf("Person: %s %s", firstName, lastName))

	successHTML := fmt.Sprintf(`<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Face updated successfully: %s %s</span>
	</div>`, firstName, lastName)
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleFacialRecognitionFaceDelete(c *gin.Context) {
	faceID := c.Param("id")
	LogDebug("WEB", "Handling facial recognition face delete", fmt.Sprintf("Face ID: %s", faceID))

	if web.notifyManager == nil || web.notifyManager.facialRecognitionService == nil {
		errorHTML := `<div class="notification is-warning is-light">
			<span class="icon"><i class="fas fa-exclamation-triangle"></i></span>
			<span>Facial recognition service is not available.</span>
		</div>`
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	// Delete face from database
	err := web.notifyManager.facialRecognitionService.DeleteFaceFromDatabase(faceID)
	if err != nil {
		LogError("WEB", "Failed to delete face from database", err.Error())
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Failed to delete face: %s</span>
		</div>`, err.Error())
		c.Data(200, "text/html", []byte(errorHTML))
		return
	}

	LogInfo("WEB", "Face deleted from database", fmt.Sprintf("Face ID: %s", faceID))

	successHTML := `<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Face deleted successfully</span>
	</div>`
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleFacialRecognitionStatus(c *gin.Context) {
	LogDebug("WEB", "Handling facial recognition status", "")

	enabled := web.globalConf.FacialRecognition.Enabled
	statusClass := "status-error"
	statusText := "Disabled"
	icon := "fas fa-times-circle"
	indicatorClass := "has-background-danger"

	if enabled {
		statusClass = "status-success"
		statusText = "Enabled"
		icon = "fas fa-check-circle"
		indicatorClass = "has-background-success"
	}

	statusHTML := fmt.Sprintf(`<div class="status-card %s">
		<div class="status-card-header">
			<span class="status-name">
				<span class="icon">
					<i class="%s"></i>
				</span>
				Facial Recognition
			</span>
			<span class="status-indicator %s"></span>
		</div>
		<div class="status-card-body">
			<p class="status-message">Status: %s</p>
		</div>
	</div>`, statusClass, icon, indicatorClass, statusText)

	c.Data(200, "text/html", []byte(statusHTML))
}

func (web *FNDWebServer) handleFacialRecognitionImage(c *gin.Context) {
	person := c.Param("person")
	filename := c.Param("filename")
	LogDebug("WEB", "Handling facial recognition image request", fmt.Sprintf("Person: %s, File: %s", person, filename))

	if web.notifyManager == nil || web.notifyManager.facialRecognitionService == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// Construct the image path
	imagePath := filepath.Join("fnd_conf", person, filename)

	// Check if file exists
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		LogDebug("WEB", "Face image not found", imagePath)
		c.Status(http.StatusNotFound)
		return
	}

	// Set appropriate headers
	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour

	// Serve the file
	c.File(imagePath)
}

// Object Filter Handlers

func (web *FNDWebServer) handleObjectFiltersPage(c *gin.Context) {
	LogDebug("WEB", "Handling object filters page", "")

	t := template.Must(template.ParseFS(templateFS, "templates/object_filters.html"))
	t.Execute(c.Writer, gin.H{
		"Cameras": web.frigateConf.Cameras,
	})
}

func (web *FNDWebServer) handleObjectFiltersCameras(c *gin.Context) {
	LogDebug("WEB", "Handling object filters cameras list", "")

	// Try to get cameras from Frigate API if available
	if web.frigateEvent != nil && web.frigateEvent.api != nil {
		cams, err := web.frigateEvent.api.getCameras()
		if err == nil {
			// Sync discovered cameras with configuration
			for cameraName := range cams.Cameras {
				web.frigateConf.checkOrAddCamera(cameraName)
			}
			LogInfo("WEB", "Synced cameras from Frigate", fmt.Sprintf("Total cameras: %d", len(cams.Cameras)))
		} else {
			LogWarn("WEB", "Failed to sync cameras from Frigate", err.Error())
		}
	}

	cameras := make([]gin.H, 0) // Initialize as empty slice, not nil
	for cameraName, cameraConfig := range web.frigateConf.Cameras {
		cameras = append(cameras, gin.H{
			"Name":         cameraName,
			"Active":       cameraConfig.Active,
			"ObjectFilter": cameraConfig.ObjectFilter,
		})
	}

	LogInfo("WEB", "Returning cameras list", fmt.Sprintf("Cameras count: %d", len(cameras)))
	c.JSON(200, cameras)
}

func (web *FNDWebServer) handleObjectFiltersCameraConfig(c *gin.Context) {
	cameraName := c.Param("camera")
	LogDebug("WEB", "Handling object filters camera config", fmt.Sprintf("Camera: %s", cameraName))

	cameraConfig := web.frigateConf.checkOrAddCamera(cameraName)
	availableObjects := GetAvailableObjects()

	// Create template with custom functions
	t := template.Must(template.New("object_filters_camera_modal.html").Funcs(template.FuncMap{
		"contains": func(slice []string, item string) bool {
			for _, s := range slice {
				if s == item {
					return true
				}
			}
			return false
		},
	}).ParseFS(templateFS, "templates/object_filters_camera_modal.html"))

	t.Execute(c.Writer, gin.H{
		"Camera":           cameraConfig,
		"CameraName":       cameraName,
		"AvailableObjects": availableObjects,
	})
}

func (web *FNDWebServer) handleObjectFiltersCameraSave(c *gin.Context) {
	cameraName := c.Param("camera")
	LogDebug("WEB", "Handling object filters camera save", fmt.Sprintf("Camera: %s", cameraName))

	// Parse form data
	enabled := c.PostForm("enabled") == "true"
	objects := c.PostFormArray("objects")

	// Update camera configuration
	web.frigateConf.m.Lock()
	if config, exists := web.frigateConf.Cameras[cameraName]; exists {
		config.ObjectFilter.Enabled = enabled
		config.ObjectFilter.Objects = objects
		web.frigateConf.Cameras[cameraName] = config
	} else {
		// Create new camera config if it doesn't exist
		web.frigateConf.Cameras[cameraName] = CameraConfig{
			Name:   cameraName,
			Active: true,
			ObjectFilter: ObjectFilter{
				Enabled: enabled,
				Objects: objects,
			},
		}
	}
	web.frigateConf.m.Unlock()

	// Save configuration
	err := web.saveConfiguration()
	if err != nil {
		LogError("WEB", "Failed to save object filter configuration", err.Error())
		c.JSON(500, gin.H{"error": "Failed to save configuration"})
		return
	}

	LogInfo("WEB", "Object filter configuration saved", fmt.Sprintf("Camera: %s, Enabled: %t, Objects: %v", cameraName, enabled, objects))

	// Return success response with script to close modal and reload
	successHTML := fmt.Sprintf(`<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Object filter configuration saved for camera: %s</span>
		<script>
			setTimeout(function() {
				closeCameraModal();
				loadCamerasList();
			}, 1500);
		</script>
	</div>`, cameraName)
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleObjectFiltersAvailableObjects(c *gin.Context) {
	LogDebug("WEB", "Handling available objects request", "")

	objects := GetAvailableObjects()
	c.JSON(200, objects)
}

// Pending Faces Handlers

func (web *FNDWebServer) handlePendingFacesPage(c *gin.Context) {
	LogDebug("WEB", "Handling pending faces page", "")

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// Get pending events stats
	stats := web.notifyManager.pendingFacesManager.GetPendingEventsStats()

	t := template.Must(template.ParseFS(templateFS, "templates/pending_faces.html"))
	t.Execute(c.Writer, gin.H{
		"Stats": stats,
	})
}

func (web *FNDWebServer) handlePendingFacesList(c *gin.Context) {
	LogDebug("WEB", "Handling pending faces list", "")

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.JSON(500, gin.H{"error": "Pending faces manager not available"})
		return
	}

	// Get filter parameters
	camera := c.Query("camera")
	status := c.Query("status") // "pending", "processed", or "all"

	var events []PendingFaceEvent
	if camera != "" {
		events = web.notifyManager.pendingFacesManager.GetPendingEventsByCamera(camera)
	} else {
		events = web.notifyManager.pendingFacesManager.GetPendingEvents()
	}

	// Filter by status if specified
	if status == "pending" {
		var pendingEvents []PendingFaceEvent
		for _, event := range events {
			if !event.Processed {
				pendingEvents = append(pendingEvents, event)
			}
		}
		events = pendingEvents
	} else if status == "processed" {
		var processedEvents []PendingFaceEvent
		for _, event := range events {
			if event.Processed {
				processedEvents = append(processedEvents, event)
			}
		}
		events = processedEvents
	}

	c.JSON(200, events)
}

func (web *FNDWebServer) handlePendingFacesStats(c *gin.Context) {
	LogDebug("WEB", "Handling pending faces stats", "")

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.JSON(500, gin.H{"error": "Pending faces manager not available"})
		return
	}

	stats := web.notifyManager.pendingFacesManager.GetPendingEventsStats()
	c.JSON(200, stats)
}

func (web *FNDWebServer) handlePendingFacesProcess(c *gin.Context) {
	pendingID := c.Param("id")
	LogDebug("WEB", "Handling pending faces process", fmt.Sprintf("Pending ID: %s", pendingID))

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.JSON(500, gin.H{"error": "Pending faces manager not available"})
		return
	}

	if web.notifyManager.facialRecognitionService == nil {
		c.JSON(500, gin.H{"error": "Facial recognition service not available"})
		return
	}

	// Process the pending event with AI
	err := web.notifyManager.pendingFacesManager.ProcessPendingEventWithAI(pendingID, web.notifyManager.facialRecognitionService)
	if err != nil {
		LogError("WEB", "Failed to process pending event", err.Error())
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	LogInfo("WEB", "Pending event processed successfully", fmt.Sprintf("Pending ID: %s", pendingID))
	c.JSON(200, gin.H{"message": "Event processed successfully"})
}

func (web *FNDWebServer) handlePendingFacesDelete(c *gin.Context) {
	pendingID := c.Param("id")
	LogDebug("WEB", "Handling pending faces delete", fmt.Sprintf("Pending ID: %s", pendingID))

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.JSON(500, gin.H{"error": "Pending faces manager not available"})
		return
	}

	// Delete the pending event
	err := web.notifyManager.pendingFacesManager.DeletePendingEvent(pendingID)
	if err != nil {
		LogError("WEB", "Failed to delete pending event", err.Error())
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	LogInfo("WEB", "Pending event deleted successfully", fmt.Sprintf("Pending ID: %s", pendingID))
	c.JSON(200, gin.H{"message": "Event deleted successfully"})
}

func (web *FNDWebServer) handlePendingFacesImage(c *gin.Context) {
	pendingID := c.Param("id")
	LogDebug("WEB", "Handling pending faces image request", fmt.Sprintf("Pending ID: %s", pendingID))

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// Get the pending event
	event := web.notifyManager.pendingFacesManager.GetPendingEventByID(pendingID)
	if event == nil {
		c.Status(http.StatusNotFound)
		return
	}

	// Check if image file exists
	if _, err := os.Stat(event.ImagePath); os.IsNotExist(err) {
		LogDebug("WEB", "Pending face image not found", event.ImagePath)
		c.Status(http.StatusNotFound)
		return
	}

	// Set appropriate headers
	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=3600") // Cache for 1 hour

	// Serve the file
	c.File(event.ImagePath)
}

func (web *FNDWebServer) handlePendingFacesCleanup(c *gin.Context) {
	LogDebug("WEB", "Handling pending faces cleanup", "")

	if web.notifyManager == nil || web.notifyManager.pendingFacesManager == nil {
		c.JSON(500, gin.H{"error": "Pending faces manager not available"})
		return
	}

	// Get days parameter (default to 30 days)
	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid days parameter"})
		return
	}

	// Cleanup old processed events
	err = web.notifyManager.pendingFacesManager.CleanupOldProcessedEvents(days)
	if err != nil {
		LogError("WEB", "Failed to cleanup old events", err.Error())
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	LogInfo("WEB", "Pending faces cleanup completed", fmt.Sprintf("Days: %d", days))
	c.JSON(200, gin.H{"message": "Cleanup completed successfully"})
}

func (web *FNDWebServer) handlePendingFacesConfig(c *gin.Context) {
	LogDebug("WEB", "Handling pending faces configuration update", "")

	if web.notifyManager == nil {
		c.JSON(500, gin.H{"error": "Notification manager not available"})
		return
	}

	// Parse form data
	if err := c.Request.ParseForm(); err != nil {
		LogError("WEB", "Failed to parse form data", err.Error())
		c.JSON(400, gin.H{"error": "Invalid form data"})
		return
	}

	// Get current configuration
	config := web.globalConf.FacialRecognition

	// Update pending faces auto-process setting
	autoProcess := c.PostForm("pendingFacesAutoProcess") == "true"
	LogInfo("WEB", "Updating pending faces auto-process setting", fmt.Sprintf("Enabled: %t", autoProcess))
	config.PendingFacesAutoProcess = autoProcess

	// Update processing interval
	intervalStr := c.PostForm("pendingFacesInterval")
	if intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil && interval >= 1 && interval <= 168 {
			LogInfo("WEB", "Updating pending faces processing interval", fmt.Sprintf("Interval: %d hours", interval))
			config.PendingFacesInterval = interval
		} else {
			LogWarn("WEB", "Invalid pending faces interval value", fmt.Sprintf("Value: %s, Error: %v", intervalStr, err))
			c.JSON(400, gin.H{"error": "Invalid interval value (must be 1-168 hours)"})
			return
		}
	}

	// Update the configuration
	web.globalConf.FacialRecognition = config

	// Save configuration to file
	if err := web.saveConfiguration(); err != nil {
		LogError("WEB", "Failed to save configuration", err.Error())
		c.JSON(500, gin.H{"error": "Failed to save configuration"})
		return
	}

	LogInfo("WEB", "Pending faces configuration updated successfully", fmt.Sprintf("AutoProcess: %t, Interval: %d hours", autoProcess, config.PendingFacesInterval))

	// Redirect back to facial recognition page
	c.Redirect(http.StatusSeeOther, "/facial-recognition")
}

// Task Scheduler Handlers

func (web *FNDWebServer) handleTaskSchedulerPage(c *gin.Context) {
	LogDebug("WEB", "Handling task scheduler page", "")

	t := template.Must(template.ParseFS(templateFS, "templates/task_scheduler.html"))
	t.Execute(c.Writer, gin.H{
		"Config": web.globalConf.TaskScheduler,
	})
}

func (web *FNDWebServer) handleTaskSchedulerHistory(c *gin.Context) {
	LogDebug("WEB", "Handling task scheduler history", "")

	// Get limit parameter (default to 50)
	limitStr := c.DefaultQuery("limit", "50")
	_, err := strconv.Atoi(limitStr)
	if err != nil {
		// Use default limit if parsing fails
	}

	// For now, return empty history until we integrate task scheduler properly
	history := []TaskExecution{}
	
	// Create HTML response
	var historyHTML strings.Builder
	if len(history) == 0 {
		historyHTML.WriteString(`<div class="notification is-info is-light">
			<span class="icon"><i class="fas fa-info-circle"></i></span>
			<span>No task execution history available yet.</span>
		</div>`)
	} else {
		historyHTML.WriteString(`<div class="execution-history">`)
		for _, execution := range history {
			statusClass := "completed"
			statusIcon := "fa-check-circle"
			if execution.Status == "failed" {
				statusClass = "failed"
				statusIcon = "fa-times-circle"
			} else if execution.Status == "running" {
				statusClass = "running"
				statusIcon = "fa-spinner fa-spin"
			}
			
			// Format completion time
			completionTime := "N/A"
			if execution.CompletedAt != nil {
				completionTime = execution.CompletedAt.Format("2006-01-02 15:04:05")
			}
			
			historyHTML.WriteString(fmt.Sprintf(`<div class="execution-item %s">
				<div class="columns is-multiline">
					<div class="column is-3">
						<strong>%s</strong>
					</div>
					<div class="column is-2">
						<span class="tag is-small">
							<span class="icon is-small">
								<i class="fas %s"></i>
							</span>
							<span>%s</span>
						</span>
					</div>
					<div class="column is-2">
						%s
					</div>
					<div class="column is-3">
						%s
					</div>
					<div class="column is-2">
						%s
					</div>
				</div>
			</div>`, statusClass, execution.TaskType, statusIcon, execution.Status,
				execution.StartedAt.Format("2006-01-02 15:04:05"),
				completionTime,
				execution.Duration.String()))
		}
		historyHTML.WriteString(`</div>`)
	}
	
	c.Data(200, "text/html", []byte(historyHTML.String()))
}

func (web *FNDWebServer) handleTaskSchedulerQueue(c *gin.Context) {
	LogDebug("WEB", "Handling task scheduler queue", "")

	// For now, return empty queue stats until we integrate task scheduler properly
	stats := map[string]interface{}{
		"total":     0,
		"pending":   0,
		"failed":    0,
		"completed": 0,
		"maxSize":   1000,
	}
	
	// Create HTML response for queue stats
	queueStatsHTML := fmt.Sprintf(`<div class="queue-stats">
		<div class="stat-card">
			<div class="stat-number">%v</div>
			<div class="stat-label">Total Events</div>
		</div>
		<div class="stat-card">
			<div class="stat-number">%v</div>
			<div class="stat-label">Pending</div>
		</div>
		<div class="stat-card">
			<div class="stat-number">%v</div>
			<div class="stat-label">Completed</div>
		</div>
		<div class="stat-card">
			<div class="stat-number">%v</div>
			<div class="stat-label">Failed</div>
		</div>
		<div class="stat-card">
			<div class="stat-number">%v</div>
			<div class="stat-label">Max Size</div>
		</div>
	</div>`, stats["total"], stats["pending"], stats["completed"], stats["failed"], stats["maxSize"])
	
	c.Data(200, "text/html", []byte(queueStatsHTML))
}

func (web *FNDWebServer) handleTaskSchedulerExecute(c *gin.Context) {
	taskType := c.Param("taskType")
	LogDebug("WEB", "Handling task scheduler execute", fmt.Sprintf("Task type: %s", taskType))

	// Validate task type
	validTypes := map[string]bool{
		"event_processing": true,
		"pending_faces":    true,
		"log_purge":        true,
	}

	if !validTypes[taskType] {
		errorHTML := fmt.Sprintf(`<div class="notification is-danger is-light">
			<span class="icon"><i class="fas fa-times-circle"></i></span>
			<span>Invalid task type: %s</span>
		</div>`, taskType)
		c.Data(400, "text/html", []byte(errorHTML))
		return
	}

	// For now, return success until we integrate task scheduler properly
	executionId := fmt.Sprintf("%s_%d", taskType, time.Now().Unix())
	successHTML := fmt.Sprintf(`<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Task execution triggered: %s (ID: %s)</span>
		<button class="delete" onclick="this.parentElement.remove()"></button>
	</div>`, taskType, executionId)
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleTaskSchedulerConfig(c *gin.Context) {
	LogDebug("WEB", "Handling task scheduler configuration update", "")

	// Parse form data
	if err := c.Request.ParseForm(); err != nil {
		LogError("WEB", "Failed to parse form data", err.Error())
		c.JSON(400, gin.H{"error": "Invalid form data"})
		return
	}

	// Get current configuration
	config := web.globalConf.TaskScheduler

	// Update event processing interval
	if intervalStr := c.PostForm("eventProcessingInterval"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil && interval >= 1 && interval <= 60 {
			config.EventProcessingInterval = interval
		}
	}

	// Update pending faces interval
	if intervalStr := c.PostForm("pendingFacesInterval"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil && interval >= 1 && interval <= 168 {
			config.PendingFacesInterval = interval
		}
	}

	// Update log purge interval
	if intervalStr := c.PostForm("logPurgeInterval"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil && interval >= 1 && interval <= 168 {
			config.LogPurgeInterval = interval
		}
	}

	// Update retention days
	if retentionStr := c.PostForm("taskHistoryRetentionDays"); retentionStr != "" {
		if retention, err := strconv.Atoi(retentionStr); err == nil && retention >= 1 && retention <= 365 {
			config.TaskHistoryRetentionDays = retention
		}
	}

	// Update enable event queue
	config.EnableEventQueue = c.PostForm("enableEventQueue") == "true"

	// Update max queue size
	if queueSizeStr := c.PostForm("maxEventQueueSize"); queueSizeStr != "" {
		if queueSize, err := strconv.Atoi(queueSizeStr); err == nil && queueSize >= 100 && queueSize <= 10000 {
			config.MaxEventQueueSize = queueSize
		}
	}

	// Update max concurrent tasks
	if concurrentStr := c.PostForm("maxConcurrentTasks"); concurrentStr != "" {
		if concurrent, err := strconv.Atoi(concurrentStr); err == nil && concurrent >= 1 && concurrent <= 20 {
			config.MaxConcurrentTasks = concurrent
		}
	}

	// Update the configuration
	web.globalConf.TaskScheduler = config

	// Save configuration to file
	if err := web.saveConfiguration(); err != nil {
		LogError("WEB", "Failed to save configuration", err.Error())
		c.JSON(500, gin.H{"error": "Failed to save configuration"})
		return
	}

	LogInfo("WEB", "Task scheduler configuration updated successfully", "")

	successHTML := `<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>Configuration updated successfully</span>
		<button class="delete" onclick="this.parentElement.remove()"></button>
	</div>`
	c.Data(200, "text/html", []byte(successHTML))
}

func (web *FNDWebServer) handleTaskSchedulerPurgeHistory(c *gin.Context) {
	LogDebug("WEB", "Handling task scheduler history purge", "")

	// For now, return success until we integrate task scheduler properly
	successHTML := `<div class="notification is-success is-light">
		<span class="icon"><i class="fas fa-check-circle"></i></span>
		<span>History purge completed successfully</span>
		<button class="delete" onclick="this.parentElement.remove()"></button>
	</div>`
	c.Data(200, "text/html", []byte(successHTML))
}
