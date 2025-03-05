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

	for {
		select {
		case <-bg.ctx.Done():
			return
		case <-ticker.C:
			cams, err := bg.api.getCameras()
			if err != nil {
				continue
			}
			for k := range cams.Cameras {
				_ = bg.conf.Frigate.checkOrAddCamera(k)
			}
		case <-tickerLong.C:
			bg.conf.Notify = bg.notify.getConfigAll()
			err := bg.conf.WriteToFile(bg.configuration_path)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}
}
