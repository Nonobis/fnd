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
}

type FNDWebNotification struct {
	N            FNDNotification
	Jepg_encoded string
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
}

type BenachrichtigungPayload struct {
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

		web.OverviewPayload.TranslatedText = []string{
			web.translation.lookupToken("header"),
			web.translation.lookupToken("overview"),
			web.translation.lookupToken("menu"),
			web.translation.lookupToken("settings"),
			web.translation.lookupToken("notifications"),
			web.translation.lookupToken("last_notify"),
		}

		t := template.Must(template.ParseFS(templateFS,
			"templates/index.html",
			"templates/uebersicht.html",
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

	r.GET("/htmx/uebersicht.html", func(c *gin.Context) {
		t := template.Must(template.ParseFS(templateFS, "templates/uebersicht.html"))
		t.Execute(c.Writer, web.OverviewPayload)
	})
	r.GET("/htmx/frigate.html", func(c *gin.Context) {
		t := template.Must(template.ParseFS(templateFS, "templates/frigate.html"))
		t.Execute(c.Writer, nil)
	})
	r.GET("/htmx/benachrichtigungen.html", func(c *gin.Context) {

		text := []string{
			web.translation.lookupToken("notifications"),
			web.translation.lookupToken("cooldown"),
			web.translation.lookupToken("active_cams"),
			web.translation.lookupToken("apply"),
		}

		t := template.Must(template.ParseFS(templateFS, "templates/benachrichtigungen.html"))
		t.Execute(c.Writer, BenachrichtigungPayload{
			ShowStatus:     false,
			Conf:           conf,
			TranslatedText: text,
		})
	})

	r.POST("/htmx/language.html", func(c *gin.Context) {

		lang := c.Query("lang")
		var payload string

		err := web.translation.setLanguage(lang)
		if err != nil {
			payload = err.Error()
		} else {
			payload = web.translation.lookupToken("reload")
			web.frigateConf.Language = lang
		}

		t := template.Must(template.ParseFS(templateFS, "templates/language.html"))
		t.Execute(c.Writer, payload)
	})
	r.POST("/htmx/benachrichtigungen.html", func(c *gin.Context) {

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

		t := template.Must(template.ParseFS(templateFS, "templates/benachrichtigungen.html"))
		t.Execute(c.Writer, BenachrichtigungPayload{
			ShowStatus:     true,
			Color:          "is-primary",
			StatusMessage:  "OK",
			Conf:           conf,
			TranslatedText: text,
		})
	})

	return &web
}

func (web *FNDWebServer) run() {
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
	// catching ctx.Done(). timeout of 5 seconds.
	select {
	case <-ctx.Done():
		log.Println("timeout of 1 seconds.")
	}
}

func (web *FNDWebServer) addNotification(n FNDNotification) {
	web.OverviewPayload.WebNotifications[web.notifyIndex] = FNDWebNotification{
		N:            n,
		Jepg_encoded: base64.StdEncoding.EncodeToString(n.JpegData),
	}
	web.notifyIndex = (web.notifyIndex + 1) % MAX_NOTIFICATIONS
}

func (web *FNDWebServer) addNotificationSinkStatus(n FNDNotificationSinkStatus) {
	web.OverviewPayload.NotificationStatus[n.Name] = n
}
