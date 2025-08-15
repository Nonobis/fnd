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
	Name           string
	Active         bool
	ObjectFilter   ObjectFilterConfig
	TimeSchedule   TimeScheduleConfig
}

// ObjectFilterConfig defines which objects to include/exclude for a camera
type ObjectFilterConfig struct {
	Enabled     bool     `json:"enabled"`
	Mode        string   `json:"mode"` // "include" or "exclude"
	Objects     []string `json:"objects"`
	MinScore    float32  `json:"minScore"`
	MaxScore    float32  `json:"maxScore"`
}

// TimeScheduleConfig defines when notifications should be sent for a camera
type TimeScheduleConfig struct {
	Enabled     bool                `json:"enabled"`
	Days        []time.Weekday      `json:"days"`
	StartTime   string              `json:"startTime"`   // Format: "HH:MM"
	EndTime     string              `json:"endTime"`     // Format: "HH:MM"
	TimeZone    string              `json:"timeZone"`
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
	Conf    map[string]FNDNotificationConfigurationMap
	Apprise AppriseConfig `json:"apprise"`
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
	conf, err := NewFNDConfigurationFromFile(filename)
	if err != nil {
		LogWarn("CONFIG", "No configuration found, using default", err.Error())
		fmt.Println("No Configuration found. Using default.")
	} else {
		LogInfo("CONFIG", "Configuration loaded successfully", fmt.Sprintf("File: %s", filename))
	}
	return conf
}

func NEWDefaultFNDConfiguration() *FNDConfiguration {
	return &FNDConfiguration{
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
		},
		Logging: NewDefaultLoggingConfiguration(),
	}
}

// NewDefaultLoggingConfiguration creates a default logging configuration
func NewDefaultLoggingConfiguration() FNDLoggingConfiguration {
	return FNDLoggingConfiguration{
		MaxEntries:    DEFAULT_MAX_LOG_ENTRIES,
		LogLevel:      DEFAULT_LOG_LEVEL,
		EnableFile:    true,
		EnableConsole: true,
	}
}

func NEWDefaultFNDNotificationConfigurationMap() FNDNotificationConfigurationMap {
	return FNDNotificationConfigurationMap{
		Map: make(map[string]string),
	}
}

// NewDefaultAppriseConfig creates a default Apprise configuration
func NewDefaultAppriseConfig() AppriseConfig {
	return AppriseConfig{
		Enabled:   false,
		ConfigID:  "",
		ServerURL: "http://apprise:8000",
		Timeout:   30,
		Format:    "text",
	}
}

// NewDefaultObjectFilterConfig creates a default object filter configuration
func NewDefaultObjectFilterConfig() ObjectFilterConfig {
	return ObjectFilterConfig{
		Enabled:  false,
		Mode:     "include",
		Objects:  []string{},
		MinScore: 0.0,
		MaxScore: 1.0,
	}
}

// NewDefaultTimeScheduleConfig creates a default time schedule configuration
func NewDefaultTimeScheduleConfig() TimeScheduleConfig {
	return TimeScheduleConfig{
		Enabled:   false,
		Days:      []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday},
		StartTime: "00:00",
		EndTime:   "23:59",
		TimeZone:  "UTC",
	}
}

// ToMap converts AppriseConfig to a map for backward compatibility
func (ac *AppriseConfig) ToMap() map[string]string {
	return map[string]string{
		AppriseEnabledKey:   fmt.Sprintf("%t", ac.Enabled),
		AppriseConfigIDKey:  ac.ConfigID,
		AppriseServerURLKey: ac.ServerURL,
		AppriseTimeoutKey:   fmt.Sprintf("%d", ac.Timeout),
		AppriseFormatKey:    ac.Format,
	}
}

// FromMap populates AppriseConfig from a map for backward compatibility
func (ac *AppriseConfig) FromMap(m map[string]string) {
	if enabled, exists := m[AppriseEnabledKey]; exists {
		ac.Enabled = enabled == "true"
	}
	if configID, exists := m[AppriseConfigIDKey]; exists {
		ac.ConfigID = configID
	}
	if serverURL, exists := m[AppriseServerURLKey]; exists {
		ac.ServerURL = serverURL
	}
	if timeout, exists := m[AppriseTimeoutKey]; exists {
		if t, err := strconv.Atoi(timeout); err == nil {
			ac.Timeout = t
		}
	}
	if format, exists := m[AppriseFormatKey]; exists {
		ac.Format = format
	}
}

func NewFNDConfigurationFromFile(filename string) (*FNDConfiguration, error) {
	conf := NEWDefaultFNDConfiguration()
	_, err := os.Stat(filename)
	if err != nil {
		return conf, err
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return conf, err
	}

	err = json.Unmarshal(data, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

func (conf *FNDConfiguration) WriteToFile(filename string) error {
	file, _ := json.MarshalIndent(conf, "", " ")

	err := os.WriteFile(filename, file, 0644)

	if err != nil {
		LogError("CONFIG", "Failed to write configuration file", err.Error())
		return err
	}

	LogInfo("CONFIG", "Configuration file written successfully", fmt.Sprintf("File: %s", filename))
	return nil
}

// If the Camera doesnt exists, add a Default one and return that
func (fConf *FNDFrigateConfiguration) checkOrAddCamera(name string) CameraConfig {
	fConf.m.Lock()
	defer fConf.m.Unlock()

	cam, avail := fConf.Cameras[name]
	if avail {
		return cam
	}

	LogInfo("CAMERA", "New camera discovered", fmt.Sprintf("Camera: %s", name))
	cam.Name = name
	cam.Active = false
	cam.ObjectFilter = NewDefaultObjectFilterConfig()
	cam.TimeSchedule = NewDefaultTimeScheduleConfig()

	fConf.Cameras[cam.Name] = cam

	return cam
}

func (fConf *FNDFrigateConfiguration) activateCameras(activeList []string) {
	fConf.m.Lock()
	defer fConf.m.Unlock()

	// Deactivate all cameras first
	for k := range fConf.Cameras {
		buffer, avail := fConf.Cameras[k]
		if !avail {
			continue
		}
		buffer.Active = false
		fConf.Cameras[k] = buffer
	}

	// Activate selected cameras
	for _, c := range activeList {
		buffer, avail := fConf.Cameras[c]
		if !avail {
			continue
		}
		buffer.Active = true
		fConf.Cameras[c] = buffer
	}

	LogInfo("CAMERA", "Camera activation updated", fmt.Sprintf("Active cameras: %v", activeList))
}

// ShouldProcessEvent checks if an event should be processed based on camera settings
func (fConf *FNDFrigateConfiguration) ShouldProcessEvent(cameraName, objectLabel string, score float32) bool {
	camera := fConf.checkOrAddCamera(cameraName)
	
	// Check if camera is active
	if !camera.Active {
		return false
	}
	
	// Check object filter
	if camera.ObjectFilter.Enabled {
		if !fConf.isObjectAllowed(camera.ObjectFilter, objectLabel, score) {
			return false
		}
	}
	
	// Check time schedule
	if camera.TimeSchedule.Enabled {
		if !fConf.isTimeAllowed(camera.TimeSchedule) {
			return false
		}
	}
	
	return true
}

// isObjectAllowed checks if an object should be allowed based on filter settings
func (fConf *FNDFrigateConfiguration) isObjectAllowed(filter ObjectFilterConfig, objectLabel string, score float32) bool {
	// Check score range
	if score < filter.MinScore || score > filter.MaxScore {
		return false
	}
	
	// Check if object is in the filter list
	objectInList := false
	for _, obj := range filter.Objects {
		if obj == objectLabel {
			objectInList = true
			break
		}
	}
	
	// Apply mode logic
	if filter.Mode == "include" {
		return objectInList
	} else { // exclude mode
		return !objectInList
	}
}

// isTimeAllowed checks if the current time is within the allowed schedule
func (fConf *FNDFrigateConfiguration) isTimeAllowed(schedule TimeScheduleConfig) bool {
	now := time.Now()
	
	// Check if current day is allowed
	dayAllowed := false
	for _, day := range schedule.Days {
		if now.Weekday() == day {
			dayAllowed = true
			break
		}
	}
	if !dayAllowed {
		return false
	}
	
	// Parse start and end times
	startTime, err := time.Parse("15:04", schedule.StartTime)
	if err != nil {
		LogWarn("CAMERA", "Invalid start time format", fmt.Sprintf("Time: %s, Error: %s", schedule.StartTime, err.Error()))
		return false
	}
	
	endTime, err := time.Parse("15:04", schedule.EndTime)
	if err != nil {
		LogWarn("CAMERA", "Invalid end time format", fmt.Sprintf("Time: %s, Error: %s", schedule.EndTime, err.Error()))
		return false
	}
	
	// Get current time in the same format
	currentTime := time.Date(2000, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)
	
	// Check if current time is within the allowed range
	return currentTime.After(startTime) && currentTime.Before(endTime)
}
