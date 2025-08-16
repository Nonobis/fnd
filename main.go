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

	// Migrate data to standardized paths
	migrateDataToStandardPaths()

	// Initialize logger with configuration
	err = InitializeLogger(&conf.Logging)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		panic(err)
	}

	// Initialize Sentry (before other components)
	// Check for environment variables first
	sentryDSN := GetSentryDSNFromEnv()
	sentryEnvironment := GetSentryEnvironmentFromEnv()

	// Enable Sentry if DSN is provided via environment variable, regardless of config
	if sentryDSN != "" {
		conf.Sentry.Enabled = true
		conf.Sentry.DSN = sentryDSN
		conf.Sentry.Environment = sentryEnvironment
		LogInfo("MAIN", "Sentry enabled via environment variables", fmt.Sprintf("Environment: %s", sentryEnvironment))
	}

	if conf.Sentry.Enabled {
		// Try to get DSN from environment if not set in config
		if conf.Sentry.DSN == "" {
			conf.Sentry.DSN = sentryDSN
		}

		// Try to get environment from environment variable
		if conf.Sentry.Environment == "" {
			conf.Sentry.Environment = sentryEnvironment
		}

		err := InitializeSentry(conf.Sentry, version)
		if err != nil {
			LogWarn("MAIN", "Failed to initialize Sentry", err.Error())
		} else {
			defer CloseSentry()
		}
	} else {
		LogInfo("MAIN", "Sentry disabled", "No DSN provided in config or environment variables")
	}

	// Set up panic handler for Sentry
	SetupPanicHandler()

	LogInfo("SYSTEM", "Application starting", fmt.Sprintf("Version: %s", version))

	// ###################################

	// Initialize task scheduler
	taskScheduler := NewTaskScheduler(&conf.TaskScheduler, nil, nil, nil)
	taskScheduler.Start()
	LogInfo("TASK_SCHEDULER", "Task scheduler initialized", "")

	connection, err := setupFNDFrigateConnection(&conf.Frigate, &conf.FacialRecognition, taskScheduler)
	if err != nil {
		LogError("FRIGATE", "Failed to setup Frigate connection", err.Error())
		fmt.Println(err.Error())
		return
	}
	LogInfo("FRIGATE", "Frigate connection established successfully", "")

	web := setupBasicRoutes("0.0.0.0:7777", &conf.Frigate, conf, configuration_path, taskScheduler, connection)
	LogInfo("WEB", "Web server initialized", "Address: 0.0.0.0:7777")

	notify := NewFNDNotificationManager(conf.Notify, &conf.FacialRecognition)
	notify.setupNotificationSinks(connection.eventManager.notificationChannel, web, connection)
	web.setNotificationManager(notify)
	LogInfo("NOTIFY", "Notification manager initialized", "")

	// Update task scheduler with notification manager
	taskScheduler.notifyManager = notify
	taskScheduler.eventManager = &connection.eventManager
	taskScheduler.api = &connection.api

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

	// Stop task scheduler
	taskScheduler.Stop()
	LogInfo("TASK_SCHEDULER", "Task scheduler stopped", "")

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

// migrateDataToStandardPaths migrates existing data to standardized paths in fnd_conf directory
func migrateDataToStandardPaths() {
	// Migrate task_history.json if it exists in root
	if _, err := os.Stat("task_history.json"); err == nil {
		if err := os.Rename("task_history.json", "fnd_conf/task_history.json"); err == nil {
			LogInfo("MIGRATION", "Migrated task_history.json to fnd_conf/", "")
		} else {
			LogWarn("MIGRATION", "Failed to migrate task_history.json", err.Error())
		}
	}

	// Migrate event_queue.json if it exists in root
	if _, err := os.Stat("event_queue.json"); err == nil {
		if err := os.Rename("event_queue.json", "fnd_conf/event_queue.json"); err == nil {
			LogInfo("MIGRATION", "Migrated event_queue.json to fnd_conf/", "")
		} else {
			LogWarn("MIGRATION", "Failed to migrate event_queue.json", err.Error())
		}
	}

	// Migrate pending_events.json from face_db if it exists
	if _, err := os.Stat("face_db/pending_events.json"); err == nil {
		if err := os.Rename("face_db/pending_events.json", "fnd_conf/pending_events.json"); err == nil {
			LogInfo("MIGRATION", "Migrated pending_events.json to fnd_conf/", "")
		} else {
			LogWarn("MIGRATION", "Failed to migrate pending_events.json", err.Error())
		}
	}

	// Migrate faces.json from face_db if it exists
	if _, err := os.Stat("face_db/faces.json"); err == nil {
		if err := os.Rename("face_db/faces.json", "fnd_conf/faces.json"); err == nil {
			LogInfo("MIGRATION", "Migrated faces.json to fnd_conf/", "")
		} else {
			LogWarn("MIGRATION", "Failed to migrate faces.json", err.Error())
		}
	}

	// Create pending_faces directory in fnd_conf if it doesn't exist
	if err := os.MkdirAll("fnd_conf/pending_faces", 0755); err != nil {
		LogWarn("MIGRATION", "Failed to create pending_faces directory", err.Error())
	}
}
