package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type FNDConfiguration struct {
	Frigate FNDFrigateConfiguration
	Notify  FNDNotificationConfiguration
}

type FNDFrigateConfiguration struct {
	Host       string
	Port       string
	MqttServer string
	MqttPort   string
	Cooldown   int
	Cameras    map[string]CameraConfig
	Language   string

	m sync.Mutex
}

type CameraConfig struct {
	Name   string
	Active bool
}

type FNDNotificationConfigurationMap struct {
	Map map[string]string
}

type FNDNotificationConfiguration struct {
	Conf map[string]FNDNotificationConfigurationMap
}

func LoadFNDConf(filename string) *FNDConfiguration {
	conf, err := NewFNDConfigurationFromFile(filename)
	if err != nil {
		//TODO: Logging
		fmt.Println("No Configuration found. Using default.")
	}
	return conf
}

func NEWDefaultFNDConfiguration() *FNDConfiguration {
	return &FNDConfiguration{
		Frigate: FNDFrigateConfiguration{
			Host:       "frigate",
			Port:       "5000",
			MqttServer: "mqtt-server",
			MqttPort:   "1883",
			Cooldown:   60,
			Cameras:    make(map[string]CameraConfig),
			Language:   "en",
		},
		Notify: FNDNotificationConfiguration{
			Conf: make(map[string]FNDNotificationConfigurationMap),
		}}
}

func NEWDefaultFNDNotificationConfigurationMap() FNDNotificationConfigurationMap {
	return FNDNotificationConfigurationMap{
		Map: make(map[string]string),
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
		return err
	}

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

	cam.Name = name
	cam.Active = false

	fConf.Cameras[cam.Name] = cam

	return cam
}

func (fConf *FNDFrigateConfiguration) activateCameras(activeList []string) {
	fConf.m.Lock()
	defer fConf.m.Unlock()

	for k := range fConf.Cameras {
		buffer, avail := fConf.Cameras[k]
		if !avail {
			continue
		}
		buffer.Active = false
		fConf.Cameras[k] = buffer
	}

	for _, c := range activeList {
		buffer, avail := fConf.Cameras[c]
		if !avail {
			continue
		}
		buffer.Active = true
		fConf.Cameras[c] = buffer
	}
}
