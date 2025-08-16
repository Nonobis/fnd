package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type FNDConfiguration struct {
	Frigate FNDFrigateConfiguration
	Notify  FNDNotificationConfiguration
	Logging FNDLoggingConfiguration
	FacialRecognition FNDFacialRecognitionConfiguration
}

type FNDFrigateConfiguration struct {
	Host           string
	Port           string
	MqttServer     string
	MqttPort       string
	MqttUsername   string
	MqttPassword   string
	MqttAnonymous  bool
	Cooldown       int
	Cameras        map[string]CameraConfig
	Language       string
	VideoEnabled   bool
	VideoUrlOnly   bool
	MaxVideoSizeMB int
	ExternalURL    string

	m sync.Mutex
}

// ObjectFilter represents which objects to watch for a camera
type ObjectFilter struct {
	Enabled bool     `json:"enabled"`
	Objects []string `json:"objects"`
}

type CameraConfig struct {
	Name         string       `json:"name"`
	Active       bool         `json:"active"`
	ObjectFilter ObjectFilter `json:"objectFilter"`
}

// Apprise configuration constants
const (
	AppriseEnabledKey   = "enabled"
	AppriseConfigIDKey  = "configID"
	AppriseServerURLKey = "serverURL"
	AppriseTimeoutKey   = "timeout"
	AppriseFormatKey    = "format"
)

// AppriseConfig represents the Apprise notification configuration
type AppriseConfig struct {
	Enabled   bool   `json:"enabled"`
	ConfigID  string `json:"configID"`
	ServerURL string `json:"serverURL"`
	Timeout   int    `json:"timeout"`
	Format    string `json:"format"`
}

type FNDNotificationConfigurationMap struct {
	Map map[string]string
}

type FNDNotificationConfiguration struct {
	Conf      map[string]FNDNotificationConfigurationMap
	Apprise   AppriseConfig         `json:"apprise"`
	Templates NotificationTemplates `json:"templates"`
}

// NotificationTemplates represents customizable notification templates
type NotificationTemplates struct {
	Global     NotificationTemplate            `json:"global"`
	PerService map[string]NotificationTemplate `json:"perService"`
}

// NotificationTemplate represents a single notification template
type NotificationTemplate struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// TemplateVariables represents available variables for templates
type TemplateVariables struct {
	Camera      string
	Object      string
	Date        string
	Time        string
	VideoURL    string
	HasVideo    bool
	EventID     string
	HasSnapshot bool
	SnapshotURL string
	HasFaces    bool
	RecognizedFaces string
	UnknownFaces string
	FaceCount   int
}

// Logging configuration constants
const (
	LOG_LEVEL_DEBUG_VALUE = 0
	LOG_LEVEL_INFO_VALUE  = 1
	LOG_LEVEL_WARN_VALUE  = 2
	LOG_LEVEL_ERROR_VALUE = 3

	DEFAULT_MAX_LOG_ENTRIES = 1000
	DEFAULT_LOG_LEVEL       = LOG_LEVEL_INFO_VALUE
)

type FNDLoggingConfiguration struct {
	MaxEntries    int  `json:"maxEntries"`
	LogLevel      int  `json:"logLevel"`
	EnableFile    bool `json:"enableFile"`
	EnableConsole bool `json:"enableConsole"`
}

// Facial Recognition configuration constants
const (
	FacialRecognitionEnabledKey = "enabled"
	CodeProjectAIHostKey        = "host"
	CodeProjectAIPortKey        = "port"
	CodeProjectAIUseSSLKey      = "useSSL"
	CodeProjectAITimeoutKey     = "timeout"
	FaceDetectionEnabledKey     = "faceDetectionEnabled"
	FaceRecognitionEnabledKey   = "faceRecognitionEnabled"
	FaceDatabasePathKey         = "faceDatabasePath"
)

// FNDFacialRecognitionConfiguration represents the facial recognition configuration
type FNDFacialRecognitionConfiguration struct {
	Enabled              bool   `json:"enabled"`
	CodeProjectAIHost    string `json:"codeProjectAIHost"`
	CodeProjectAIPort    int    `json:"codeProjectAIPort"`
	CodeProjectAIUseSSL  bool   `json:"codeProjectAIUseSSL"`
	CodeProjectAITimeout int    `json:"codeProjectAITimeout"`
	FaceDetectionEnabled bool   `json:"faceDetectionEnabled"`
	FaceRecognitionEnabled bool `json:"faceRecognitionEnabled"`
	FaceDatabasePath     string `json:"faceDatabasePath"`
	
	m sync.Mutex
}

// FaceRecord represents a face record in the database
type FaceRecord struct {
	ID          string    `json:"id"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Notes       string    `json:"notes"`
	FaceID      string    `json:"faceId"`
	ImagePath   string    `json:"imagePath"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	IsActive    bool      `json:"isActive"`
}

// FaceDatabase represents the face database structure
type FaceDatabase struct {
	Faces []FaceRecord `json:"faces"`
	
	m sync.RWMutex
}

func LoadFNDConf(filename string) *FNDConfiguration {
	LogDebug("CONFIG", "Loading configuration", fmt.Sprintf("File: %s", filename))
	
	conf, err := NewFNDConfigurationFromFile(filename)
	if err != nil {
		LogWarn("CONFIG", "No configuration found, using default", err.Error())
		fmt.Println("No Configuration found. Using default.")
	} else {
		LogInfo("CONFIG", "Configuration loaded successfully", fmt.Sprintf("File: %s", filename))
	}
	
	LogDebug("CONFIG", "Configuration details", fmt.Sprintf("Frigate host: %s, MQTT server: %s, Language: %s", conf.Frigate.Host, conf.Frigate.MqttServer, conf.Frigate.Language))
	return conf
}

func NEWDefaultFNDConfiguration() *FNDConfiguration {
	LogDebug("CONFIG", "Creating default configuration", "")
	
	conf := &FNDConfiguration{
		Frigate: FNDFrigateConfiguration{
			Host:           "frigate",
			Port:           "5000",
			MqttServer:     "mqtt-server",
			MqttPort:       "1883",
			MqttUsername:   "",
			MqttPassword:   "",
			MqttAnonymous:  true,
			Cooldown:       60,
			Cameras:        make(map[string]CameraConfig),
			Language:       "en",
			VideoEnabled:   true,
			VideoUrlOnly:   false,
			MaxVideoSizeMB: 10,
			ExternalURL:    "",
		},
		Notify: FNDNotificationConfiguration{
			Conf:    make(map[string]FNDNotificationConfigurationMap),
			Apprise: NewDefaultAppriseConfig(),
			Templates: NotificationTemplates{
				Global: NotificationTemplate{
					Title:   "New Event",
					Message: "A new event has occurred: {{.Object}} at {{.Camera}} on {{.Date}} {{.Time}}{{if .HasVideo}}\n🎥 Video: {{.VideoURL}}{{end}}{{if .HasSnapshot}}\n📸 Snapshot attached{{end}}{{if .HasFaces}}\n👤 Faces detected: {{.FaceCount}}{{if .RecognizedFaces}}\n✅ Recognized: {{.RecognizedFaces}}{{end}}{{if .UnknownFaces}}\n❓ Unknown: {{.UnknownFaces}}{{end}}{{end}}",
				},
				PerService: make(map[string]NotificationTemplate),
			},
		},
		Logging: NewDefaultLoggingConfiguration(),
		FacialRecognition: FNDFacialRecognitionConfiguration{
			Enabled: false,
			CodeProjectAIHost: "localhost",
			CodeProjectAIPort: 8000,
			CodeProjectAIUseSSL: false,
			CodeProjectAITimeout: 30,
			FaceDetectionEnabled: true,
			FaceRecognitionEnabled: true,
			FaceDatabasePath: "./face_db",
		},
	}
	
	LogDebug("CONFIG", "Default configuration created", fmt.Sprintf("Frigate host: %s, Cooldown: %d, Language: %s", conf.Frigate.Host, conf.Frigate.Cooldown, conf.Frigate.Language))
	return conf
}

// NewDefaultLoggingConfiguration creates a default logging configuration
func NewDefaultLoggingConfiguration() FNDLoggingConfiguration {
	LogDebug("CONFIG", "Creating default logging configuration", "")
	
	config := FNDLoggingConfiguration{
		MaxEntries:    DEFAULT_MAX_LOG_ENTRIES,
		LogLevel:      DEFAULT_LOG_LEVEL,
		EnableFile:    true,
		EnableConsole: true,
	}
	
	LogDebug("CONFIG", "Default logging configuration created", fmt.Sprintf("MaxEntries: %d, LogLevel: %d, EnableFile: %t, EnableConsole: %t", config.MaxEntries, config.LogLevel, config.EnableFile, config.EnableConsole))
	return config
}

func NEWDefaultFNDNotificationConfigurationMap() FNDNotificationConfigurationMap {
	LogDebug("CONFIG", "Creating default notification configuration map", "")
	
	config := FNDNotificationConfigurationMap{
		Map: make(map[string]string),
	}
	
	LogDebug("CONFIG", "Default notification configuration map created", "")
	return config
}

// NewDefaultAppriseConfig creates a default Apprise configuration
func NewDefaultAppriseConfig() AppriseConfig {
	LogDebug("CONFIG", "Creating default Apprise configuration", "")
	
	config := AppriseConfig{
		Enabled:   false,
		ConfigID:  "",
		ServerURL: "http://apprise:8000",
		Timeout:   30,
		Format:    "text",
	}
	
	LogDebug("CONFIG", "Default Apprise configuration created", fmt.Sprintf("Enabled: %t, ServerURL: %s, Timeout: %d", config.Enabled, config.ServerURL, config.Timeout))
	return config
}

// ToMap converts AppriseConfig to a map for backward compatibility
func (ac *AppriseConfig) ToMap() map[string]string {
	LogDebug("CONFIG", "Converting AppriseConfig to map", fmt.Sprintf("Enabled: %t, ConfigID: %s", ac.Enabled, ac.ConfigID))
	
	configMap := map[string]string{
		AppriseEnabledKey:   fmt.Sprintf("%t", ac.Enabled),
		AppriseConfigIDKey:  ac.ConfigID,
		AppriseServerURLKey: ac.ServerURL,
		AppriseTimeoutKey:   fmt.Sprintf("%d", ac.Timeout),
		AppriseFormatKey:    ac.Format,
	}
	
	LogDebug("CONFIG", "AppriseConfig converted to map", fmt.Sprintf("Map size: %d", len(configMap)))
	return configMap
}

// FromMap populates AppriseConfig from a map for backward compatibility
func (ac *AppriseConfig) FromMap(m map[string]string) {
	LogDebug("CONFIG", "Populating AppriseConfig from map", fmt.Sprintf("Map size: %d", len(m)))
	
	if enabled, exists := m[AppriseEnabledKey]; exists {
		ac.Enabled = enabled == "true"
		LogDebug("CONFIG", "AppriseConfig enabled set", fmt.Sprintf("Value: %t", ac.Enabled))
	}
	if configID, exists := m[AppriseConfigIDKey]; exists {
		ac.ConfigID = configID
		LogDebug("CONFIG", "AppriseConfig ConfigID set", fmt.Sprintf("Value: %s", ac.ConfigID))
	}
	if serverURL, exists := m[AppriseServerURLKey]; exists {
		ac.ServerURL = serverURL
		LogDebug("CONFIG", "AppriseConfig ServerURL set", fmt.Sprintf("Value: %s", ac.ServerURL))
	}
	if timeout, exists := m[AppriseTimeoutKey]; exists {
		if t, err := strconv.Atoi(timeout); err == nil {
			ac.Timeout = t
			LogDebug("CONFIG", "AppriseConfig Timeout set", fmt.Sprintf("Value: %d", ac.Timeout))
		} else {
			LogWarn("CONFIG", "Invalid timeout value in AppriseConfig", fmt.Sprintf("Value: %s, Error: %s", timeout, err.Error()))
		}
	}
	if format, exists := m[AppriseFormatKey]; exists {
		ac.Format = format
		LogDebug("CONFIG", "AppriseConfig Format set", fmt.Sprintf("Value: %s", ac.Format))
	}
	
	LogDebug("CONFIG", "AppriseConfig populated from map", fmt.Sprintf("Enabled: %t, ConfigID: %s, ServerURL: %s", ac.Enabled, ac.ConfigID, ac.ServerURL))
}

func NewFNDConfigurationFromFile(filename string) (*FNDConfiguration, error) {
	LogDebug("CONFIG", "Loading configuration from file", fmt.Sprintf("File: %s", filename))
	
	conf := NEWDefaultFNDConfiguration()
	_, err := os.Stat(filename)
	if err != nil {
		LogDebug("CONFIG", "Configuration file does not exist", fmt.Sprintf("File: %s, Error: %s", filename, err.Error()))
		return conf, err
	}

	LogDebug("CONFIG", "Configuration file exists, reading content", fmt.Sprintf("File: %s", filename))
	data, err := os.ReadFile(filename)
	if err != nil {
		LogError("CONFIG", "Failed to read configuration file", fmt.Sprintf("File: %s, Error: %s", filename, err.Error()))
		return conf, err
	}

	LogDebug("CONFIG", "Configuration file read successfully", fmt.Sprintf("File: %s, Size: %d bytes", filename, len(data)))
	err = json.Unmarshal(data, &conf)
	if err != nil {
		LogError("CONFIG", "Failed to parse configuration JSON", fmt.Sprintf("File: %s, Error: %s", filename, err.Error()))
		return conf, err
	}

	LogInfo("CONFIG", "Configuration loaded from file successfully", fmt.Sprintf("File: %s", filename))
	return conf, nil
}

func (conf *FNDConfiguration) WriteToFile(filename string) error {
	LogDebug("CONFIG", "Writing configuration to file", fmt.Sprintf("File: %s", filename))
	
	file, err := json.MarshalIndent(conf, "", " ")
	if err != nil {
		LogError("CONFIG", "Failed to marshal configuration to JSON", fmt.Sprintf("File: %s, Error: %s", filename, err.Error()))
		return err
	}

	LogDebug("CONFIG", "Configuration marshaled to JSON", fmt.Sprintf("File: %s, Size: %d bytes", filename, len(file)))
	err = os.WriteFile(filename, file, 0644)

	if err != nil {
		LogError("CONFIG", "Failed to write configuration file", fmt.Sprintf("File: %s, Error: %s", filename, err.Error()))
		return err
	}

	LogInfo("CONFIG", "Configuration file written successfully", fmt.Sprintf("File: %s, Size: %d bytes", filename, len(file)))
	return nil
}

// If the Camera doesnt exists, add a Default one and return that
func (fConf *FNDFrigateConfiguration) checkOrAddCamera(name string) CameraConfig {
	LogDebug("CAMERA", "Checking or adding camera", fmt.Sprintf("Camera name: %s", name))
	
	fConf.m.Lock()
	defer fConf.m.Unlock()

	cam, avail := fConf.Cameras[name]
	if avail {
		LogDebug("CAMERA", "Camera already exists", fmt.Sprintf("Camera: %s, Active: %t", name, cam.Active))
		return cam
	}

	LogInfo("CAMERA", "New camera discovered", fmt.Sprintf("Camera: %s", name))
	cam.Name = name
	cam.Active = false

	fConf.Cameras[cam.Name] = cam
	LogDebug("CAMERA", "Camera added to configuration", fmt.Sprintf("Camera: %s, Total cameras: %d", name, len(fConf.Cameras)))

	return cam
}

func (fConf *FNDFrigateConfiguration) activateCameras(activeList []string) {
	LogDebug("CAMERA", "Updating camera activation", fmt.Sprintf("Active list: %v, Total cameras: %d", activeList, len(fConf.Cameras)))
	
	fConf.m.Lock()
	defer fConf.m.Unlock()

	// Deactivate all cameras first
	deactivatedCount := 0
	for k := range fConf.Cameras {
		buffer, avail := fConf.Cameras[k]
		if !avail {
			continue
		}
		if buffer.Active {
			buffer.Active = false
			fConf.Cameras[k] = buffer
			deactivatedCount++
		}
	}
	LogDebug("CAMERA", "Cameras deactivated", fmt.Sprintf("Count: %d", deactivatedCount))

	// Activate selected cameras
	activatedCount := 0
	for _, c := range activeList {
		buffer, avail := fConf.Cameras[c]
		if !avail {
			LogWarn("CAMERA", "Camera not found for activation", fmt.Sprintf("Camera: %s", c))
			continue
		}
		buffer.Active = true
		fConf.Cameras[c] = buffer
		activatedCount++
		LogDebug("CAMERA", "Camera activated", fmt.Sprintf("Camera: %s", c))
	}

	LogInfo("CAMERA", "Camera activation updated", fmt.Sprintf("Active cameras: %v, Activated: %d, Deactivated: %d", activeList, activatedCount, deactivatedCount))
}

// NewDefaultFacialRecognitionConfiguration creates a default facial recognition configuration
func NewDefaultFacialRecognitionConfiguration() FNDFacialRecognitionConfiguration {
	LogDebug("CONFIG", "Creating default facial recognition configuration", "")
	
	config := FNDFacialRecognitionConfiguration{
		Enabled:              false,
		CodeProjectAIHost:    "localhost",
		CodeProjectAIPort:    8000,
		CodeProjectAIUseSSL:  false,
		CodeProjectAITimeout: 30,
		FaceDetectionEnabled: true,
		FaceRecognitionEnabled: true,
		FaceDatabasePath:     "./face_db",
	}
	
	LogDebug("CONFIG", "Default facial recognition configuration created", fmt.Sprintf("Enabled: %t, Host: %s, Port: %d", config.Enabled, config.CodeProjectAIHost, config.CodeProjectAIPort))
	return config
}

// PopulateFacialRecognitionConfigFromMap populates facial recognition configuration from a map
func (frc *FNDFacialRecognitionConfiguration) PopulateFacialRecognitionConfigFromMap(m map[string]string) {
	LogDebug("CONFIG", "Populating facial recognition configuration from map", fmt.Sprintf("Map keys: %v", getMapKeys(m)))
	
	if enabled, exists := m[FacialRecognitionEnabledKey]; exists {
		frc.Enabled = enabled == "true"
		LogDebug("CONFIG", "FacialRecognitionConfig Enabled set", fmt.Sprintf("Value: %t", frc.Enabled))
	}
	if host, exists := m[CodeProjectAIHostKey]; exists {
		frc.CodeProjectAIHost = host
		LogDebug("CONFIG", "FacialRecognitionConfig Host set", fmt.Sprintf("Value: %s", frc.CodeProjectAIHost))
	}
	if port, exists := m[CodeProjectAIPortKey]; exists {
		if p, err := strconv.Atoi(port); err == nil {
			frc.CodeProjectAIPort = p
			LogDebug("CONFIG", "FacialRecognitionConfig Port set", fmt.Sprintf("Value: %d", frc.CodeProjectAIPort))
		} else {
			LogWarn("CONFIG", "Invalid port value in FacialRecognitionConfig", fmt.Sprintf("Value: %s, Error: %s", port, err.Error()))
		}
	}
	if useSSL, exists := m[CodeProjectAIUseSSLKey]; exists {
		frc.CodeProjectAIUseSSL = useSSL == "true"
		LogDebug("CONFIG", "FacialRecognitionConfig UseSSL set", fmt.Sprintf("Value: %t", frc.CodeProjectAIUseSSL))
	}
	if timeout, exists := m[CodeProjectAITimeoutKey]; exists {
		if t, err := strconv.Atoi(timeout); err == nil {
			frc.CodeProjectAITimeout = t
			LogDebug("CONFIG", "FacialRecognitionConfig Timeout set", fmt.Sprintf("Value: %d", frc.CodeProjectAITimeout))
		} else {
			LogWarn("CONFIG", "Invalid timeout value in FacialRecognitionConfig", fmt.Sprintf("Value: %s, Error: %s", timeout, err.Error()))
		}
	}
	if faceDetectionEnabled, exists := m[FaceDetectionEnabledKey]; exists {
		frc.FaceDetectionEnabled = faceDetectionEnabled == "true"
		LogDebug("CONFIG", "FacialRecognitionConfig FaceDetectionEnabled set", fmt.Sprintf("Value: %t", frc.FaceDetectionEnabled))
	}
	if faceRecognitionEnabled, exists := m[FaceRecognitionEnabledKey]; exists {
		frc.FaceRecognitionEnabled = faceRecognitionEnabled == "true"
		LogDebug("CONFIG", "FacialRecognitionConfig FaceRecognitionEnabled set", fmt.Sprintf("Value: %t", frc.FaceRecognitionEnabled))
	}
	if faceDatabasePath, exists := m[FaceDatabasePathKey]; exists {
		frc.FaceDatabasePath = faceDatabasePath
		LogDebug("CONFIG", "FacialRecognitionConfig FaceDatabasePath set", fmt.Sprintf("Value: %s", frc.FaceDatabasePath))
	}
	
	LogDebug("CONFIG", "FacialRecognitionConfig populated from map", fmt.Sprintf("Enabled: %t, Host: %s, Port: %d", frc.Enabled, frc.CodeProjectAIHost, frc.CodeProjectAIPort))
}

// getMapKeys is a helper function to get keys from a map for logging
func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// GetCameraConfig returns the configuration for a specific camera
func (c *FNDConfiguration) GetCameraConfig(cameraName string) CameraConfig {
	c.Frigate.m.Lock()
	defer c.Frigate.m.Unlock()
	
	if config, exists := c.Frigate.Cameras[cameraName]; exists {
		return config
	}
	
	// Default configuration with object filter disabled
	return CameraConfig{
		Name: cameraName, 
		Active: true,
		ObjectFilter: ObjectFilter{
			Enabled: false,
			Objects: []string{},
		},
	}
}

// ShouldAnalyzeObject checks if an object should be analyzed for a specific camera
func (c *FNDConfiguration) ShouldAnalyzeObject(cameraName, objectType string) bool {
	cameraConfig := c.GetCameraConfig(cameraName)
	
	// If camera is not active, don't analyze
	if !cameraConfig.Active {
		return false
	}
	
	// If object filter is not enabled, analyze all objects
	if !cameraConfig.ObjectFilter.Enabled {
		return true
	}
	
	// Check if the object type is in the allowed list
	for _, allowedObject := range cameraConfig.ObjectFilter.Objects {
		if allowedObject == objectType {
			return true
		}
	}
	
	return false
}

// GetAvailableObjects returns a list of commonly detected objects
func GetAvailableObjects() []string {
	return []string{
		"person",
		"car",
		"truck",
		"bicycle",
		"motorcycle",
		"bus",
		"train",
		"boat",
		"airplane",
		"bird",
		"cat",
		"dog",
		"horse",
		"sheep",
		"cow",
		"elephant",
		"bear",
		"zebra",
		"giraffe",
		"backpack",
		"umbrella",
		"handbag",
		"tie",
		"suitcase",
		"frisbee",
		"skis",
		"snowboard",
		"sports ball",
		"kite",
		"baseball bat",
		"baseball glove",
		"skateboard",
		"surfboard",
		"tennis racket",
		"bottle",
		"wine glass",
		"cup",
		"fork",
		"knife",
		"spoon",
		"bowl",
		"banana",
		"apple",
		"sandwich",
		"orange",
		"broccoli",
		"carrot",
		"hot dog",
		"pizza",
		"donut",
		"cake",
		"chair",
		"couch",
		"potted plant",
		"bed",
		"dining table",
		"toilet",
		"tv",
		"laptop",
		"mouse",
		"remote",
		"keyboard",
		"cell phone",
		"microwave",
		"oven",
		"toaster",
		"sink",
		"refrigerator",
		"book",
		"clock",
		"vase",
		"scissors",
		"teddy bear",
		"hair drier",
		"toothbrush",
	}
}
