package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const MAX_NOTIFICATIONS = 3

type FNDWebServer struct {
	srv             *http.Server
	r               *gin.Engine
	OverviewPayload OverviewPayload
	notifyIndex     int
	frigateConf     *FNDFrigateConfiguration
	globalConf      *FNDConfiguration
	translation     *Translation
	frigateEvent    *FNDFrigateEventManager
}

type FNDWebNotification struct {
	N             FNDNotification
	Jepg_encoded  string
	Video_encoded string
}

type FNDNotificationSinkStatus struct {
	Name    string
	Good    bool
	Message string
}

type OverviewPayload struct {
	WebNotifications        []FNDWebNotification
	NotificationStatus      map[string]FNDNotificationSinkStatus
	Version                 string
	TranslatedText          []string
	ActiveLanguage          int
	SupportedLanguages      []Language
	CurrentLanguage         Language
	WebNotificationsEnabled bool
	CooldownInfo            CooldownInfo
}

type CooldownInfo struct {
	CooldownPeriod    int  // Cooldown period in seconds
	IsActive          bool // Whether cooldown is currently active
	SecondsRemaining  int  // Seconds remaining until next notification allowed
	LastNotification  string // Time of last notification (formatted)
}

type LogStats struct {
	EntriesInMemory   int
	FileSizeHuman     string
	ActiveSubscribers int
	MemoryUsageHuman  string
}

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

func setupBasicRoutes(addr string, conf *FNDFrigateConfiguration, globalConf *FNDConfiguration) *FNDWebServer {
	r := gin.Default()

	var web FNDWebServer
	web.srv = &http.Server{
		Addr:    addr,
		Handler: r,
	}
	web.OverviewPayload.WebNotifications = make([]FNDWebNotification, MAX_NOTIFICATIONS)
	web.OverviewPayload.NotificationStatus = make(map[string]FNDNotificationSinkStatus)
	web.OverviewPayload.Version = version

	web.frigateConf = conf
	web.globalConf = globalConf
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
			web.translation.lookupToken("last_notify"),
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
	r.GET("/htmx/testnotification", func(c *gin.Context) {
		LogInfo("WEB", "Test notification requested", "")
		web.sendTestNotification()

		t := template.Must(template.ParseFS(templateFS, "templates/generic_ok.html"))
		t.Execute(c.Writer, nil)
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
			web.translation.lookupToken("last_notify"),
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

		LogInfo("WEB", "Frigate configuration updated successfully", "")
		t := template.Must(template.ParseFS(templateFS, "templates/frigate.html"))
		t.Execute(c.Writer, FrigateTemplatePayload{
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
		LogInfo("WEB", "Notification settings updated successfully", fmt.Sprintf("Cooldown: %ds", conf.Cooldown))

		t := template.Must(template.ParseFS(templateFS, "templates/notifications.html"))
		t.Execute(c.Writer, NotificationPayload{
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
	r.POST("/api/logs/test", func(c *gin.Context) {
		LogDebug("TEST", "Debug level test message", "This is a debug level test")
		LogInfo("TEST", "Info level test message", "This is an info level test")
		LogWarn("TEST", "Warning level test message", "This is a warning level test")
		LogError("TEST", "Error level test message", "This is an error level test")

		c.JSON(200, gin.H{"message": "Test log entries created"})
	})

	r.GET("/api/logs/stats", func(c *gin.Context) {
		logger := GetLogger()
		if logger == nil {
			c.JSON(500, gin.H{"error": "Logger not initialized"})
			return
		}

		stats := getLogStats(logger)
		c.JSON(200, gin.H{"stats": stats})
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
	stats := LogStats{}

	if logger == nil {
		return stats
	}

	logger.mutex.RLock()
	stats.EntriesInMemory = len(logger.entries)
	logger.mutex.RUnlock()

	logger.subscribeMutex.RLock()
	stats.ActiveSubscribers = len(logger.subscribers)
	logger.subscribeMutex.RUnlock()

	// Get file size
	if fileInfo, err := os.Stat(logger.filePath); err == nil {
		size := fileInfo.Size()
		if size < 1024 {
			stats.FileSizeHuman = fmt.Sprintf("%d B", size)
		} else if size < 1024*1024 {
			stats.FileSizeHuman = fmt.Sprintf("%.1f KB", float64(size)/1024)
		} else {
			stats.FileSizeHuman = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
		}
	} else {
		stats.FileSizeHuman = "N/A"
	}

	// Estimate memory usage (rough calculation)
	estimatedSize := stats.EntriesInMemory * 200 // rough estimate per log entry
	if estimatedSize < 1024 {
		stats.MemoryUsageHuman = fmt.Sprintf("%d B", estimatedSize)
	} else if estimatedSize < 1024*1024 {
		stats.MemoryUsageHuman = fmt.Sprintf("%.1f KB", float64(estimatedSize)/1024)
	} else {
		stats.MemoryUsageHuman = fmt.Sprintf("%.1f MB", float64(estimatedSize)/(1024*1024))
	}

	return stats
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

func (web *FNDWebServer) addNotification(n FNDNotification) {
	webNotif := FNDWebNotification{
		N:            n,
		Jepg_encoded: base64.StdEncoding.EncodeToString(n.JpegData),
	}

	// Encode video if available
	if n.HasVideo && len(n.VideoData) > 0 {
		webNotif.Video_encoded = base64.StdEncoding.EncodeToString(n.VideoData)
	}

	web.OverviewPayload.WebNotifications[web.notifyIndex] = webNotif
	web.notifyIndex = (web.notifyIndex + 1) % MAX_NOTIFICATIONS
}

func (web *FNDWebServer) addNotificationSinkStatus(n FNDNotificationSinkStatus) {
	web.OverviewPayload.NotificationStatus[n.Name] = n
}

func (web *FNDWebServer) setWebNotificationsEnabled(enabled bool) {
	web.OverviewPayload.WebNotificationsEnabled = enabled
}

func (web *FNDWebServer) sendTestNotification() {
	if web.frigateEvent == nil {
		fmt.Println("ASSERTION failed: FrigateEventManager is nil")
		return
	}

	data, err := staticFS.ReadFile("static/test_notification.jpg")
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	web.frigateEvent.sendNotification(FNDNotification{
		JpegData: data,
		Date:     time.Now().Format("15:04:05 02.01.2006"),
		Caption:  "Test",
	})
}
