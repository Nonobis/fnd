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

	// Initialize logger
	if err := initLogger(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		panic(err)
	}
	defer globalLogger.Close()

	LogInfo("Version: %s", version)
	LogInfo("Starting FND application...")

	//change the working Directory to the Executable Dir
	ex, err := os.Executable()
	if err != nil {
		LogError("Error getting executable path: %v", err)
		panic(err)
	}
	exPath := filepath.Dir(ex)
	LogInfo("Executable path: %s", exPath)
	os.Chdir(exPath)

	err = os.MkdirAll(CONFIGURATION_FOLDER, 0755)
	if err != nil {
		LogError("Error creating config directory: %v", err)
		panic(err)
	}

	LogInfo("Loading configuration from: %s", configuration_path)
	conf := LoadFNDConf(configuration_path)
	LogInfo("Configuration loaded successfully")

	// ###################################

	LogInfo("Setting up Frigate connection...")
	connection, err := setupFNDFrigateConnection(&conf.Frigate)
	if err != nil {
		LogError("Error setting up Frigate connection: %v", err)
		LogWarn("Continuing without Frigate connection...")
		// Create a dummy connection for now
		connection = &FNDFrigateConnection{}
	}
	LogInfo("Frigate connection setup completed")

	LogInfo("Setting up web routes...")
	web := setupBasicRoutes("0.0.0.0:7777", &conf.Frigate)
	LogInfo("Web routes setup completed")

	LogInfo("Setting up notification manager...")
	notify := NewFNDNotificationManager(conf.Notify)
	notify.setupNotificationSinks(connection.eventManager.notificationChannel, web, connection)
	LogInfo("Notification manager setup completed")

	LogInfo("Starting web server...")
	go web.run(&connection.eventManager)

	LogInfo("Starting background task...")
	bg := RunBackgroundTask(&connection.api, conf, notify, configuration_path)
	LogInfo("Background task started")

	LogInfo("FND application is running. Press Ctrl+C to stop.")

	// ###################################

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig

	LogInfo("Shutting down FND application...")
	bg.cancel()
	connection.Disconnect()
	web.stop()

	conf.Notify = notify.removeAll()
	err = conf.WriteToFile(configuration_path)
	if err != nil {
		LogError("Error writing configuration: %v", err)
	}
	LogInfo("FND application stopped.")
}
