package main

import (
	"context"
	"fmt"
	"time"
)

type BackgroundTask struct {
	ctx                context.Context
	cancel             context.CancelFunc
	api                *FNDFrigateApi
	conf               *FNDConfiguration
	notify             *FNDNotificationManager
	configuration_path string
}

func RunBackgroundTask(api *FNDFrigateApi,
	conf *FNDConfiguration,
	notify *FNDNotificationManager,
	configuration_path string) *BackgroundTask {
	bg := BackgroundTask{
		api:                api,
		conf:               conf,
		notify:             notify,
		configuration_path: configuration_path,
	}

	bg.ctx, bg.cancel = context.WithCancel(context.Background())

	go bg.task()
	return &bg

}

func (bg *BackgroundTask) task() {
	ticker := time.NewTicker(10 * time.Second)
	tickerLong := time.NewTicker(120 * time.Minute)
	defer ticker.Stop()
	defer tickerLong.Stop()

	LogInfo("BACKGROUND", "Background task started", "Camera check: 10s, Config save: 120min")

	for {
		select {
		case <-bg.ctx.Done():
			LogInfo("BACKGROUND", "Background task stopped", "")
			return
		case <-ticker.C:
			cams, err := bg.api.getCameras()
			if err != nil {
				LogError("BACKGROUND", "Failed to get cameras from API", err.Error())
				continue
			}
			discoveredCount := 0
			for k := range cams.Cameras {
				if _, exists := bg.conf.Frigate.Cameras[k]; !exists {
					discoveredCount++
				}
				_ = bg.conf.Frigate.checkOrAddCamera(k)
			}
			if discoveredCount > 0 {
				LogInfo("BACKGROUND", "Camera discovery completed", fmt.Sprintf("New cameras: %d", discoveredCount))
			}
		case <-tickerLong.C:
			LogInfo("BACKGROUND", "Periodic configuration save", "")
			bg.conf.Notify = bg.notify.getConfigAll()
			err := bg.conf.WriteToFile(bg.configuration_path)
			if err != nil {
				LogError("BACKGROUND", "Failed to save configuration", err.Error())
				fmt.Println(err.Error())
			}
		}
	}
}
