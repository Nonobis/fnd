package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

type FNDConfiguration struct {
	Frigate FNDFrigateConfiguration
	Notify  FNDNotificationConfiguration
	Logging FNDLoggingConfiguration
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

type CameraConfig struct {
	Name   string
	Active bool
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
					Message: "A new event has occurred: {{.Object}} at {{.Camera}} on {{.Date}} {{.Time}}{{if .HasVideo}}\n🎥 Video: {{.VideoURL}}{{end}}{{if .HasSnapshot}}\n📸 Snapshot attached{{end}}",
				},
				PerService: make(map[string]NotificationTemplate),
			},
		},
		Logging: NewDefaultLoggingConfiguration(),
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
