package main

import (
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
	WebNotifications   []FNDWebNotification
	NotificationStatus map[string]FNDNotificationSinkStatus
	Version            string
	TranslatedText     []string
	ActiveLanguage     int
	SupportedLanguages []Language
	CurrentLanguage    Language
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

func setupBasicRoutes(addr string, conf *FNDFrigateConfiguration) *FNDWebServer {
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
	web.r = r
	web.translation = setupTranslation()
	web.translation.setLanguage(web.frigateConf.Language)

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
		}

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
		}

		t := template.Must(template.ParseFS(templateFS, "templates/frigate.html"))
		t.Execute(c.Writer, FrigateTemplatePayload{
			ShowStatus:     false,
			Conf:           conf,
			TranslatedText: text,
		})
	})
	r.GET("/htmx/testnotification", func(c *gin.Context) {

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

		err := web.translation.setLanguage(lang)
		if err != nil {
			errorTemplate := `<div class="notification is-danger is-light">{{.}}</div>`
			t := template.Must(template.New("error").Parse(errorTemplate))
			t.Execute(c.Writer, err.Error())
			return
		}

		web.frigateConf.Language = lang
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
		text := []string{
			web.translation.lookupToken("frigate_host"),
			web.translation.lookupToken("frigate_port"),
			web.translation.lookupToken("mqtt_server"),
			web.translation.lookupToken("mqtt_port"),
			web.translation.lookupToken("apply"),
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
		}

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

	return &web
}

func (web *FNDWebServer) run(frigateEvent *FNDFrigateEventManager) {
	web.frigateEvent = frigateEvent
	if err := web.srv.ListenAndServe(); err != nil {
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
