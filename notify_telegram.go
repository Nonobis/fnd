package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type FNDTelegramNotificationSink struct {
	config            FNDNotificationConfigurationMap
	webServer         *FNDWebServer
	bot               *bot.Bot
	ctx               context.Context
	cancel            context.CancelFunc
	botRunning        bool
	chatid            int64
	lastStatusMessage string
}

type TelegramTemplatePayload struct {
	Active         bool
	Token          string
	ChatID         string
	ShowStatus     bool
	Color          string
	StatusMessage  string
	TranslatedText []string
}

func (tel *FNDTelegramNotificationSink) createDefaultConfig() {
	LogDebug("TELEGRAM", "Creating default configuration", "")

	tel.config = NEWDefaultFNDNotificationConfigurationMap()
	tel.config.Map["enabled"] = "false"
	tel.config.Map["token"] = ""
	tel.config.Map["chatid"] = ""

	LogDebug("TELEGRAM", "Default configuration created", fmt.Sprintf("Enabled: %s, Token: %s, ChatID: %s",
		tel.config.Map["enabled"], tel.config.Map["token"], tel.config.Map["chatid"]))
}

func (tel *FNDTelegramNotificationSink) getName() string {
	return "Telegram"
}

func (tel *FNDTelegramNotificationSink) setup(conf FNDNotificationConfigurationMap, avail bool) error {
	LogInfo("TELEGRAM", "Setting up Telegram sink", fmt.Sprintf("Configuration available: %t", avail))

	if avail {
		tel.config = conf
		LogDebug("TELEGRAM", "Using existing configuration", fmt.Sprintf("Enabled: %s, Token: %s, ChatID: %s",
			conf.Map["enabled"], conf.Map["token"], conf.Map["chatid"]))

		data, err := strconv.ParseInt(tel.config.Map["chatid"], 10, 64)
		if err == nil {
			tel.chatid = data
			LogDebug("TELEGRAM", "Chat ID parsed successfully", fmt.Sprintf("ChatID: %d", tel.chatid))
		} else {
			LogWarn("TELEGRAM", "Failed to parse Chat ID", fmt.Sprintf("ChatID string: %s, Error: %s", tel.config.Map["chatid"], err.Error()))
		}

	} else {
		tel.createDefaultConfig()
	}

	LogDebug("TELEGRAM", "Starting Telegram bot", "")
	err := tel.botStart()
	if err != nil {
		LogError("TELEGRAM", "Failed to start bot", err.Error())
		fmt.Println(err.Error())
	} else {
		LogInfo("TELEGRAM", "Telegram bot started successfully", "")
	}

	tel.lastStatusMessage = "init"
	LogDebug("TELEGRAM", "Telegram sink setup complete", fmt.Sprintf("Initial status: %s", tel.lastStatusMessage))
	return nil
}

func (tel *FNDTelegramNotificationSink) registerWebServer(webServer *FNDWebServer) {
	tel.webServer = webServer

	tel.webServer.r.GET("/htmx/telegram.html", func(c *gin.Context) {

		t := template.Must(template.ParseFS(templateFS, "templates/telegram.html"))
		_ = t.Execute(c.Writer, tel.generatePayload(false))
	})

	tel.webServer.r.POST("/htmx/telegram/toggle", func(c *gin.Context) {
		// Toggle the enabled status
		if tel.config.Map["enabled"] == "true" {
			tel.config.Map["enabled"] = "false"
			tel.lastStatusMessage = "disabled"
		} else {
			tel.config.Map["enabled"] = "true"
			tel.lastStatusMessage = "enabled"
		}

		// Save configuration to disk immediately
		tel.webServer.saveConfigurationWithNotifications(tel.webServer.notifyManager)

		// Return updated page
		t := template.Must(template.ParseFS(templateFS, "templates/telegram.html"))
		_ = t.Execute(c.Writer, tel.generatePayload(false))
	})

	tel.webServer.r.POST("/htmx/telegram.html", func(c *gin.Context) {

		lastToken := tel.config.Map["token"]

		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "token0815" {
				if value[0] != "" {
					tel.config.Map["token"] = value[0]
				}
				continue
			}
			if key == "chatid" {
				if value[0] != "" {
					data, err := strconv.ParseInt(value[0], 10, 64)
					if err == nil {
						tel.config.Map["chatid"] = value[0]
						tel.chatid = data
					}
				}
				continue
			}
			// Telegram doesn't have an active checkbox in the form
			// The active state is managed by the separate toggle button
		}

		LogInfo("TELEGRAM", "Configuration updated", fmt.Sprintf("Enabled: %s, Token: %s",
			tel.config.Map["enabled"],
			func() string {
				if len(tel.config.Map["token"]) > 10 {
					return tel.config.Map["token"][:10] + "..."
				}
				return tel.config.Map["token"]
			}()))

		if !tel.botRunning {
			if tel.config.Map["enabled"] == "true" {
				err := tel.botStart()
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		} else {
			if tel.config.Map["enabled"] == "false" {
				tel.botStop()
			} else {
				if lastToken != tel.config.Map["token"] {
					go tel.gracefulBotRestart()
				}
			}
		}

		// Save configuration to disk immediately
		tel.webServer.saveConfigurationWithNotifications(tel.webServer.notifyManager)

		t := template.Must(template.ParseFS(templateFS, "templates/telegram.html"))
		_ = t.Execute(c.Writer, tel.generatePayload(true))
	})

}

func (tel *FNDTelegramNotificationSink) gracefulBotRestart() {
	tel.botStop()
	time.Sleep(3 * time.Second)
	tel.botStart()
}

func (tel *FNDTelegramNotificationSink) generatePayload(postReq bool) TelegramTemplatePayload {
	en, _ := tel.config.Map["enabled"]
	var en_bool bool
	if en == "" || en == "false" {
		en_bool = false
	} else {
		en_bool = true
	}

	pay := TelegramTemplatePayload{
		Active: en_bool,
		Token:  tel.config.Map["token"],
		ChatID: tel.config.Map["chatid"],
		TranslatedText: []string{
			tel.webServer.translation.lookupToken("active"),
			tel.webServer.translation.lookupToken("apply"),
			tel.webServer.translation.lookupToken("tel_doc"),
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

func (tel *FNDTelegramNotificationSink) sendNotification(n FNDNotification) error {
	if tel.config.Map["enabled"] != "true" {
		tel.lastStatusMessage = "disabled"
		return nil
	}
	if tel.config.Map["token"] == "" {
		tel.lastStatusMessage = "Bot token is empty!"
		return errors.New("Bot token is empty!")
	}
	if !tel.botRunning {
		tel.lastStatusMessage = "Bot is not running!"
		return errors.New("Bot is not running!")
	}
	if tel.chatid == 0 {
		tel.lastStatusMessage = "Chat ID empty!"
		return errors.New("Chat ID empty!")
	}

	// If we have video data, send video instead of photo
	if n.HasVideo && len(n.VideoData) > 0 {
		videoParams := &bot.SendVideoParams{
			ChatID:  tel.chatid,
			Video:   &models.InputFileUpload{Filename: "clip.mp4", Data: bytes.NewReader(n.VideoData)},
			Caption: n.Caption,
		}

		_, err := tel.bot.SendVideo(tel.ctx, videoParams)
		if err != nil {
			tel.lastStatusMessage = err.Error()
			return err
		}
		tel.lastStatusMessage = "Online (video sent)"
		return nil
	}

	// Send photo with optional video URL
	caption := n.Caption
	if n.HasVideo && n.VideoURL != "" {
		caption += "\n🎥 Video: " + n.VideoURL
	}

	params := &bot.SendPhotoParams{
		ChatID:  tel.chatid,
		Photo:   &models.InputFileUpload{Filename: "snapshot.jpeg", Data: bytes.NewReader(n.JpegData)},
		Caption: caption,
	}

	_, err := tel.bot.SendPhoto(tel.ctx, params)
	if err != nil {
		tel.lastStatusMessage = err.Error()
		return err
	}
	tel.lastStatusMessage = "Online"
	return nil
}

func (tel *FNDTelegramNotificationSink) remove() (FNDNotificationConfigurationMap, error) {
	tel.botStop()
	return tel.config, nil
}

func (tel *FNDTelegramNotificationSink) botStart() error {
	if tel.config.Map["enabled"] != "true" {
		return errors.New("Bot is disabled!")
	}
	if tel.config.Map["token"] == "" {
		return errors.New("Bot Token is empty!")
	}
	if tel.botRunning {
		return errors.New("Bot already running")
	}

	tel.ctx, tel.cancel = context.WithCancel(context.Background())

	opts := []bot.Option{
		bot.WithDefaultHandler(tel.botHandler),
	}

	var err error
	tel.bot, err = bot.New(tel.config.Map["token"], opts...)
	if err != nil {
		return err
	}

	go tel.bot.Start(tel.ctx)

	fmt.Println("Telegram bot started")
	tel.botRunning = true

	return nil

}

func (tel *FNDTelegramNotificationSink) botStop() {
	if !tel.botRunning {
		return
	}
	tel.cancel()
	fmt.Println("Telegram bot ended")
	tel.botRunning = false
}

func (tel *FNDTelegramNotificationSink) botHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	if update.Message.Text == "/getid" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   strconv.FormatInt(update.Message.Chat.ID, 10),
		})
		return
	}

	// Handle snapshot commands: /snapshot camera_name or /live camera_name
	if strings.HasPrefix(update.Message.Text, "/snapshot ") || strings.HasPrefix(update.Message.Text, "/live ") {
		parts := strings.Fields(update.Message.Text)
		if len(parts) >= 2 {
			camera := parts[1]
			tel.handleSnapshotCommand(ctx, b, update.Message.Chat.ID, camera)
		} else {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Please specify a camera name. Example: /snapshot camera1",
			})
		}
		return
	}

	// List available cameras command
	if update.Message.Text == "/cameras" {
		tel.handleCamerasCommand(ctx, b, update.Message.Chat.ID)
		return
	}
}

func (tel *FNDTelegramNotificationSink) getConfiguration() FNDNotificationConfigurationMap {
	return tel.config
}

func (tel *FNDTelegramNotificationSink) handleSnapshotCommand(ctx context.Context, b *bot.Bot, chatID int64, camera string) {
	if tel.webServer == nil || tel.webServer.frigateEvent == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Service not available",
		})
		return
	}

	// Get live snapshot
	imageData, err := tel.webServer.frigateEvent.api.getLiveSnapshotByCamera(camera)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Failed to get snapshot from camera '%s': %s", camera, err.Error()),
		})
		return
	}

	// Send photo
	params := &bot.SendPhotoParams{
		ChatID:  chatID,
		Photo:   &models.InputFileUpload{Filename: fmt.Sprintf("snapshot_%s.jpeg", camera), Data: bytes.NewReader(imageData)},
		Caption: fmt.Sprintf("Live snapshot from camera: %s\nTime: %s", camera, time.Now().Format("15:04:05 02.01.2006")),
	}

	_, err = b.SendPhoto(ctx, params)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Failed to send snapshot: %s", err.Error()),
		})
	}
}

func (tel *FNDTelegramNotificationSink) handleCamerasCommand(ctx context.Context, b *bot.Bot, chatID int64) {
	if tel.webServer == nil || tel.webServer.frigateEvent == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Service not available",
		})
		return
	}

	// Get available cameras
	stats, err := tel.webServer.frigateEvent.api.getCameras()
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Failed to get cameras: %s", err.Error()),
		})
		return
	}

	if len(stats.Cameras) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "No cameras available",
		})
		return
	}

	// Build camera list message
	message := "Available cameras:\n"
	for cameraName := range stats.Cameras {
		message += fmt.Sprintf("📷 %s\n", cameraName)
	}
	message += "\nUse /snapshot camera_name or /live camera_name to get a live picture"

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   message,
	})
}

func (tel *FNDTelegramNotificationSink) getStatus() FNDNotificationSinkStatus {
	// Check if Telegram is enabled
	if tel.config.Map["enabled"] != "true" {
		return FNDNotificationSinkStatus{
			Name:    tel.getName(),
			Good:    false,
			Message: "Disabled",
		}
	}

	// Check if required configuration is present
	if tel.config.Map["token"] == "" {
		return FNDNotificationSinkStatus{
			Name:    tel.getName(),
			Good:    false,
			Message: "Bot token not configured",
		}
	}

	if tel.config.Map["chatid"] == "" {
		return FNDNotificationSinkStatus{
			Name:    tel.getName(),
			Good:    false,
			Message: "Chat ID not configured",
		}
	}

	// If bot is running and we have a successful status, use it
	if tel.botRunning && tel.lastStatusMessage == "Online" {
		return FNDNotificationSinkStatus{
			Name:    tel.getName(),
			Good:    true,
			Message: tel.lastStatusMessage,
		}
	}

	// If we have an error message, use it
	if tel.lastStatusMessage != "" && tel.lastStatusMessage != "init" {
		return FNDNotificationSinkStatus{
			Name:    tel.getName(),
			Good:    false,
			Message: tel.lastStatusMessage,
		}
	}

	// If bot is running but no specific status, show as ready
	if tel.botRunning {
		return FNDNotificationSinkStatus{
			Name:    tel.getName(),
			Good:    true,
			Message: "Ready",
		}
	}

	// Default status for enabled and configured but not yet started
	return FNDNotificationSinkStatus{
		Name:    tel.getName(),
		Good:    false,
		Message: "Not started",
	}
}
