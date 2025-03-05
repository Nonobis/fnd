package main

import (
	"errors"
	"sync"
	"time"
)

type FNDFrigateEventManager struct {
	api                 *FNDFrigateApi
	activeEvents        map[string]eventMessage
	notificationChannel chan FNDNotification

	lastNotificationSent time.Time
	fConf                *FNDFrigateConfiguration

	m sync.Mutex
}

func NewFNDFrigateEventManager(api *FNDFrigateApi, fConf *FNDFrigateConfiguration) *FNDFrigateEventManager {
	return &FNDFrigateEventManager{
		api:                  api,
		activeEvents:         make(map[string]eventMessage),
		notificationChannel:  make(chan FNDNotification, 100),
		lastNotificationSent: time.Now(),
		fConf:                fConf,
	}
}

func (e *FNDFrigateEventManager) addNewEventMessage(msg eventMessage) error {
	e.m.Lock()
	defer e.m.Unlock()
	_, avail := e.activeEvents[msg.Before.Id]
	switch msg.TypeInfo {
	case "new":
		if avail {
			return errors.New("Unerwartetes NEW Event")
		}
		e.activeEvents[msg.Before.Id] = msg
		var err error
		if e.shouldSendNotification(msg) {
			err = e.prepareNotification(msg)
		}
		if err != nil {
			return err
		}
	case "update":
		if !avail {
			return errors.New("Unerwartetes UPDATE Event")
		}
		e.activeEvents[msg.Before.Id] = msg
	case "end":
		if !avail {
			return errors.New("Unerwartetes END Event")
		}
		delete(e.activeEvents, msg.Before.Id)
	}

	return nil
}

func (e *FNDFrigateEventManager) shouldSendNotification(msg eventMessage) bool {
	if !e.fConf.checkOrAddCamera(msg.Before.Camera).Active {
		return false
	}

	diff := time.Now().Sub(e.lastNotificationSent)

	return diff.Seconds() > float64(e.fConf.Cooldown)

}

func (e *FNDFrigateEventManager) prepareNotification(msg eventMessage) error {
	//TODO: Translate this

	n := FNDNotification{
		Caption: "camera: " + msg.Before.Camera + " object: " + msg.Before.Label,
		Date:    time.Now().Format("15:04:05 02.01.2006"),
	}
	jpeg, err := e.api.getSnapshotByID(msg.Before.Id)
	if err != nil {
		return err
	}

	n.JpegData = jpeg

	e.sendNotification(n)
	e.lastNotificationSent = time.Now()

	return nil

}

// Reiht die Benachrichtigung ein. Wird die Schlange zu voll, wird die Benachrichtigung
// verworfen. Blockiert also nie.
func (e *FNDFrigateEventManager) sendNotification(n FNDNotification) {
	if len(e.notificationChannel) == cap(e.notificationChannel) {
		return
	}

	e.notificationChannel <- n
}

func (e *FNDFrigateEventManager) shutdown() {
	close(e.notificationChannel)
}
