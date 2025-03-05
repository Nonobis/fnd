package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
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
	tel.config = NEWDefaultFNDNotificationConfigurationMap()
	tel.config.Map["enabled"] = "false"
}

func (tel *FNDTelegramNotificationSink) getName() string {
	return "Telegram"
}

func (tel *FNDTelegramNotificationSink) setup(conf FNDNotificationConfigurationMap, avail bool) error {
	if avail {
		tel.config = conf
		data, err := strconv.ParseInt(tel.config.Map["chatid"], 10, 64)
		if err == nil {
			tel.chatid = data
		}

	} else {
		tel.createDefaultConfig()
	}
	err := tel.botStart()
	if err != nil {
		fmt.Println(err.Error())
	}
	tel.lastStatusMessage = "init"
	return nil
}

func (tel *FNDTelegramNotificationSink) registerWebServer(webServer *FNDWebServer) {
	tel.webServer = webServer

	tel.webServer.r.GET("/htmx/telegram.html", func(c *gin.Context) {

		t := template.Must(template.ParseFS(templateFS, "templates/telegram.html"))
		t.Execute(c.Writer, tel.generatePayload(false))
	})

	tel.webServer.r.POST("/htmx/telegram.html", func(c *gin.Context) {

		lastToken := tel.config.Map["token"]

		tel.config.Map["enabled"] = "false"
		c.MultipartForm()
		for key, value := range c.Request.PostForm {
			if key == "token0815" {
				if value[0] == "" {
					continue
				}
				tel.config.Map["token"] = value[0]
				continue
			}
			if key == "chatid" {
				if value[0] == "" {
					continue
				}
				data, err := strconv.ParseInt(value[0], 10, 64)
				if err != nil {
					continue
				}
				tel.config.Map["chatid"] = value[0]
				tel.chatid = data
				continue
			}
			if key == "aktiv" {
				if value[0] == "" {
					continue
				}
				tel.config.Map["enabled"] = "true"
				continue
			}
		}

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

		t := template.Must(template.ParseFS(templateFS, "templates/telegram.html"))
		t.Execute(c.Writer, tel.generatePayload(true))
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
		tel.lastStatusMessage = "disabeled"
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

	params := &bot.SendPhotoParams{
		ChatID:  tel.chatid,
		Photo:   &models.InputFileUpload{Filename: "snapshot.jpeg", Data: bytes.NewReader(n.JpegData)},
		Caption: n.Caption,
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
		return errors.New("Bot is disabeled!")
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

	if update.Message.Text == "/getid" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   strconv.FormatInt(update.Message.Chat.ID, 10),
		})
	}
}

func (tel *FNDTelegramNotificationSink) getConfiguration() FNDNotificationConfigurationMap {
	return tel.config
}

func (tel *FNDTelegramNotificationSink) getStatus() FNDNotificationSinkStatus {
	return FNDNotificationSinkStatus{
		Name:    tel.getName(),
		Good:    tel.botRunning,
		Message: tel.lastStatusMessage,
	}
}
