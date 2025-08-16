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
	err = os.Chdir(exPath)
	if err != nil {
		LogError("SYSTEM", "Failed to change working directory", err.Error())
	}

	err = os.MkdirAll(CONFIGURATION_FOLDER, 0755)
	if err != nil {
		panic(err)
	}

	conf := LoadFNDConf(configuration_path)

	// Initialize logger with configuration
	err = InitializeLogger(&conf.Logging)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		panic(err)
	}

	// Initialize Sentry (before other components)
	if conf.Sentry.Enabled {
		// Try to get DSN from environment if not set in config
		if conf.Sentry.DSN == "" {
			conf.Sentry.DSN = GetSentryDSNFromEnv()
		}

		// Try to get environment from environment variable
		if conf.Sentry.Environment == "" {
			conf.Sentry.Environment = GetSentryEnvironmentFromEnv()
		}

		err := InitializeSentry(conf.Sentry)
		if err != nil {
			LogWarn("MAIN", "Failed to initialize Sentry", err.Error())
		} else {
			defer CloseSentry()
		}
	}

	// Set up panic handler for Sentry
	SetupPanicHandler()

	LogInfo("SYSTEM", "Application starting", fmt.Sprintf("Version: %s", version))

	// ###################################

	connection, err := setupFNDFrigateConnection(&conf.Frigate, &conf.FacialRecognition)
	if err != nil {
		LogError("FRIGATE", "Failed to setup Frigate connection", err.Error())
		fmt.Println(err.Error())
		return
	}
	LogInfo("FRIGATE", "Frigate connection established successfully", "")

	web := setupBasicRoutes("0.0.0.0:7777", &conf.Frigate, conf, configuration_path)
	LogInfo("WEB", "Web server initialized", "Address: 0.0.0.0:7777")

	notify := NewFNDNotificationManager(conf.Notify, &conf.FacialRecognition)
	notify.setupNotificationSinks(connection.eventManager.notificationChannel, web, connection)
	web.setNotificationManager(notify)
	LogInfo("NOTIFY", "Notification manager initialized", "")

	go web.run(&connection.eventManager)
	LogInfo("WEB", "Web server started", "")

	bg := RunBackgroundTask(&connection.api, conf, notify, configuration_path)
	LogInfo("BACKGROUND", "Background tasks started", "")

	// ###################################

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)
	LogInfo("SYSTEM", "Application ready", "Waiting for shutdown signal")
	<-sig

	LogInfo("SYSTEM", "Shutdown signal received", "Starting graceful shutdown")
	bg.cancel()
	LogInfo("BACKGROUND", "Background tasks stopped", "")

	connection.Disconnect()
	LogInfo("FRIGATE", "Frigate connection closed", "")

	web.stop()
	LogInfo("WEB", "Web server stopped", "")

	conf.Notify = notify.removeAll()
	err = conf.WriteToFile(configuration_path)
	if err != nil {
		LogError("CONFIG", "Failed to save configuration", err.Error())
		fmt.Println(err.Error())
	} else {
		LogInfo("CONFIG", "Configuration saved successfully", "")
	}

	CloseLogger()
}
