package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const (
	CONFIGURATION_FILE   = "conf.json"
	CONFIGURATION_FOLDER = "fnd_conf"
)

var version string

func main() {
	configuration_path := CONFIGURATION_FOLDER + "/" + CONFIGURATION_FILE

	fmt.Println("Version: ", version)

	//change the working Directory to the Executable Dir
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	os.Chdir(exPath)

	err = os.MkdirAll(CONFIGURATION_FOLDER, 0755)
	if err != nil {
		panic(err)
	}

	conf := LoadFNDConf(configuration_path)

	// ###################################

	connection, err := setupFNDFrigateConnection(&conf.Frigate)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	web := setupBasicRoutes("0.0.0.0:7777", &conf.Frigate)

	notify := NewFNDNotificationManager(conf.Notify)
	notify.setupNotificationSinks(connection.eventManager.notificationChannel, web, connection)

	go web.run()
	bg := RunBackgroundTask(&connection.api, conf, notify, configuration_path)

	// ###################################

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig

	bg.cancel()
	connection.Disconnect()
	web.stop()

	conf.Notify = notify.removeAll()
	err = conf.WriteToFile(configuration_path)
	if err != nil {
		fmt.Println(err.Error())
	}
}
