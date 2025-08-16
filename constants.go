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
	DEFAULT_APPRISE_SERVER_URL = "http://apprise:8000"
	DEFAULT_APPRISE_TIMEOUT    = 30
	DEFAULT_APPRISE_FORMAT     = "text"

	// Test data
	TEST_CAMERA_NAME = "test_camera"
	TEST_OBJECT_TYPE = "person"

	// Date/Time formats
	DATE_FORMAT_DISPLAY = "2006-01-02 15:04:05"
	DATE_FORMAT_FILE    = "2006-01-02_15-04-05"

	// HTTP status codes
	HTTP_STATUS_OK             = 200
	HTTP_STATUS_BAD_REQUEST    = 400
	HTTP_STATUS_UNAUTHORIZED   = 401
	HTTP_STATUS_FORBIDDEN      = 403
	HTTP_STATUS_NOT_FOUND      = 404
	HTTP_STATUS_INTERNAL_ERROR = 500

	// Common strings
	EMPTY_STRING = ""
	TRUE_STRING  = "true"
	FALSE_STRING = "false"

	// Log levels
	LOG_LEVEL_DEBUG = "DEBUG"
	LOG_LEVEL_INFO  = "INFO"
	LOG_LEVEL_WARN  = "WARN"
	LOG_LEVEL_ERROR = "ERROR"

	// Log components - Consolidated categories
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
	COMPONENT_LOGGER         = "LOGGER"
	COMPONENT_API            = "API"
	COMPONENT_EVENT          = "EVENT"
	COMPONENT_NOTIFICATION   = "NOTIFICATION"
	COMPONENT_CAMERA         = "CAMERA"
	COMPONENT_FACIAL_RECOGNITION = "FACIAL_RECOGNITION"
	COMPONENT_PENDING_FACES  = "PENDING_FACES"

	// Notification service names
	SERVICE_TELEGRAM = "telegram"
	SERVICE_GOTIFY   = "gotify"
	SERVICE_APPRISE  = "apprise"

	// File extensions
	EXTENSION_JSON = ".json"
	EXTENSION_DB   = ".db"
	EXTENSION_LOG  = ".log"

	// HTTP methods
	HTTP_METHOD_GET  = "GET"
	HTTP_METHOD_POST = "POST"
	HTTP_METHOD_PUT  = "PUT"
	HTTP_METHOD_DELETE = "DELETE"

	// HTTP headers
	HEADER_CONTENT_TYPE = "Content-Type"
	HEADER_ACCEPT       = "Accept"
	HEADER_USER_AGENT   = "User-Agent"
	HEADER_AUTHORIZATION = "Authorization"

	// MIME types
	MIME_TYPE_JSON = "application/json"
	MIME_TYPE_FORM = "application/x-www-form-urlencoded"
	MIME_TYPE_MULTIPART = "multipart/form-data"
	MIME_TYPE_JPEG = "image/jpeg"
	MIME_TYPE_MP4 = "video/mp4"

	// Error messages
	ERROR_MSG_INVALID_CONFIG = "Invalid configuration"
	ERROR_MSG_FILE_NOT_FOUND = "File not found"
	ERROR_MSG_PERMISSION_DENIED = "Permission denied"
	ERROR_MSG_TIMEOUT = "Operation timed out"
	ERROR_MSG_CONNECTION_FAILED = "Connection failed"

	// Success messages
	SUCCESS_MSG_CONFIG_SAVED = "Configuration saved successfully"
	SUCCESS_MSG_OPERATION_COMPLETED = "Operation completed successfully"

	// Info messages
	INFO_MSG_INITIALIZING = "Initializing"
	INFO_MSG_SHUTTING_DOWN = "Shutting down"
	INFO_MSG_OPERATION_STARTED = "Operation started"

	// Warning messages
	WARN_MSG_DEPRECATED_FEATURE = "This feature is deprecated"
	WARN_MSG_FALLBACK_USED = "Using fallback method"

	// Migration file names
	MIGRATION_PENDING_FACES_DIR   = "pending_faces"
)
