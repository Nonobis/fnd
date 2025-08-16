package main

// Application constants
const (
	// Default values
	DEFAULT_FRIGATE_HOST      = "frigate"
	DEFAULT_FRIGATE_PORT      = "5000"
	DEFAULT_MQTT_SERVER       = "mqtt-server"
	DEFAULT_MQTT_PORT         = "1883"
	DEFAULT_MQTT_CLIENT_ID    = "fnd"
	DEFAULT_MQTT_TOPIC_PREFIX = "frigate"
	DEFAULT_COOLDOWN_SECONDS  = 60
	DEFAULT_LANGUAGE          = "en"
	DEFAULT_MAX_VIDEO_SIZE_MB = 10
	DEFAULT_VIDEO_ENABLED     = true
	DEFAULT_VIDEO_URL_ONLY    = false

	// Facial Recognition defaults
	DEFAULT_CODE_PROJECT_AI_HOST    = "localhost"
	DEFAULT_CODE_PROJECT_AI_PORT    = 8000
	DEFAULT_CODE_PROJECT_AI_TIMEOUT = 30
	DEFAULT_FACE_DATABASE_PATH      = "fnd_conf/faces.db"
	DEFAULT_PENDING_FACES_INTERVAL  = 6 // hours

	// Task Scheduler defaults
	DEFAULT_EVENT_PROCESSING_INTERVAL = 1  // minutes
	DEFAULT_LOG_PURGE_INTERVAL        = 24 // hours
	DEFAULT_TASK_HISTORY_RETENTION    = 7  // days
	DEFAULT_MAX_EVENT_QUEUE_SIZE      = 1000
	DEFAULT_MAX_CONCURRENT_TASKS      = 5

	// Logging defaults
	DEFAULT_LOG_FILE_NAME     = "fnd.log"
	DEFAULT_LOG_BUFFER_SIZE   = 100
	DEFAULT_MEMORY_CACHE_SIZE = 1000
	DEFAULT_ROTATION_SIZE_MB  = 10
	DEFAULT_MAX_LOG_FILES     = 5

	// Web server
	DEFAULT_WEB_SERVER_ADDRESS = "0.0.0.0:7777"

	// Validation limits
	MIN_PENDING_FACES_INTERVAL = 1
	MAX_PENDING_FACES_INTERVAL = 168 // 1 week

	// File permissions
	DEFAULT_DIRECTORY_PERMISSIONS = 0755

	// Sentry defaults
	DEFAULT_SENTRY_ENVIRONMENT = "production"
	DEFAULT_SENTRY_DEBUG       = false

	// Notification template defaults
	DEFAULT_NOTIFICATION_TITLE   = "New Event"
	DEFAULT_NOTIFICATION_MESSAGE = "A new event has occurred: {{.Object}} at {{.Camera}} on {{.Date}} {{.Time}}{{if .HasVideo}}\n🎥 Video: {{.VideoURL}}{{end}}{{if .HasSnapshot}}\n📸 Snapshot attached{{end}}{{if .HasFaces}}\n👤 Faces detected: {{.FaceCount}}{{if .RecognizedFaces}}\n✅ Recognized: {{.RecognizedFaces}}{{end}}{{if .UnknownFaces}}\n❓ Unknown: {{.UnknownFaces}}{{end}}{{end}}"

	// Apprise defaults
	DEFAULT_APPRISE_TIMEOUT = 30
	DEFAULT_APPRISE_FORMAT  = "text"

	// Test data
	TEST_CAMERA_NAME      = "test_camera"
	TEST_OBJECT_TYPE      = "person"
	TEST_DATE_FORMAT      = "01.01.2024"
	TEST_TIME_FORMAT      = "12:00:00"
	TEST_VIDEO_URL        = "http://example.com/video.mp4"
	TEST_EVENT_ID         = "test_event_123"
	TEST_SNAPSHOT_URL     = "[Snapshot attached]"
	TEST_RECOGNIZED_FACES = "John Doe (95.2%), Jane Smith (87.1%)"
	TEST_UNKNOWN_FACES    = "2"
	TEST_FACE_COUNT       = 4

	// Date/Time formats
	DATE_FORMAT = "02.01.2006"
	TIME_FORMAT = "15:04:05"

	// HTTP status codes
	HTTP_STATUS_SEE_OTHER = 303

	// Common strings
	EMPTY_STRING = ""
	TRUE_STRING  = "true"
	FALSE_STRING = "false"
)

// Component names for logging
const (
	COMPONENT_SYSTEM         = "SYSTEM"
	COMPONENT_MAIN           = "MAIN"
	COMPONENT_CONFIG         = "CONFIG"
	COMPONENT_WEB            = "WEB"
	COMPONENT_FRIGATE        = "FRIGATE"
	COMPONENT_NOTIFY         = "NOTIFY"
	COMPONENT_BACKGROUND     = "BACKGROUND"
	COMPONENT_TASK_SCHEDULER = "TASK_SCHEDULER"
	COMPONENT_TEMPLATE       = "TEMPLATE"
	COMPONENT_TRANSLATE      = "TRANSLATE"
	COMPONENT_MIGRATION      = "MIGRATION"
)

// Notification service names
const (
	SERVICE_TELEGRAM = "telegram"
	SERVICE_GOTIFY   = "gotify"
	SERVICE_APPRISE  = "apprise"
)

// File extensions
const (
	EXTENSION_JSON = ".json"
	EXTENSION_DB   = ".db"
	EXTENSION_LOG  = ".log"
)

// HTTP methods
const (
	HTTP_METHOD_GET  = "GET"
	HTTP_METHOD_POST = "POST"
)

// Common HTTP headers
const (
	HEADER_CONTENT_TYPE = "Content-Type"
	HEADER_ACCEPT       = "Accept"
)

// MIME types
const (
	MIME_TYPE_JSON = "application/json"
	MIME_TYPE_HTML = "text/html"
	MIME_TYPE_TEXT = "text/plain"
)

// Error messages
const (
	ERROR_INVALID_FORM_DATA  = "Invalid form data"
	ERROR_SAVE_CONFIGURATION = "Failed to save configuration"
	ERROR_INVALID_INTERVAL   = "Invalid interval value (must be 1-168 hours)"
	ERROR_INVALID_PORT       = "Port must be between 1 and 65535"
	ERROR_INVALID_EMAIL      = "Please enter a valid email address"
	ERROR_INVALID_URL        = "Please enter a valid URL starting with http:// or https://"
	ERROR_INVALID_TOKEN      = "Token must be at least 10 characters long"
	ERROR_INVALID_CHAT_ID    = "Chat ID must be a number"
	ERROR_INVALID_HOST       = "Host must contain only letters, numbers, dots, and hyphens"
	ERROR_REQUIRED_FIELD     = "This field is required"
	ERROR_POSITIVE_INTEGER   = "Please enter a positive integer"
)

// Success messages
const (
	SUCCESS_CONFIGURATION_SAVED   = "Configuration saved successfully"
	SUCCESS_PENDING_FACES_UPDATED = "Pending faces configuration updated successfully"
)

// Info messages
const (
	INFO_APPLICATION_STARTING     = "Application starting"
	INFO_APPLICATION_READY        = "Application ready"
	INFO_SHUTDOWN_SIGNAL_RECEIVED = "Shutdown signal received"
	INFO_GRACEFUL_SHUTDOWN        = "Starting graceful shutdown"
	INFO_SENTRY_ENABLED           = "Sentry enabled via environment variables"
	INFO_SENTRY_DISABLED          = "Sentry disabled"
)

// Warning messages
const (
	WARN_NO_CONFIGURATION_FOUND     = "No configuration found, using default"
	WARN_SENTRY_INIT_FAILED         = "Failed to initialize Sentry"
	WARN_MIGRATION_FAILED           = "Failed to migrate"
	WARN_TASK_SCHEDULER_UNAVAILABLE = "Task scheduler not available for history"
)

// Debug messages
const (
	DEBUG_LOADING_CONFIGURATION   = "Loading configuration"
	DEBUG_CREATING_DEFAULT_CONFIG = "Creating default configuration"
	DEBUG_CONFIGURATION_LOADED    = "Configuration loaded successfully"
	DEBUG_SETTING_UP_TRANSLATION  = "Setting up translation system"
	DEBUG_LOADING_LANGUAGE_CONFIG = "Loading language configuration"
)

// Migration file names
const (
	MIGRATION_TASK_HISTORY_FILE   = "task_history.json"
	MIGRATION_EVENT_QUEUE_FILE    = "event_queue.json"
	MIGRATION_PENDING_EVENTS_FILE = "pending_events.json"
	MIGRATION_FACES_FILE          = "faces.json"
	MIGRATION_PENDING_FACES_DIR   = "pending_faces"
)
