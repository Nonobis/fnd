package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type FNDFrigateApi struct {
	url string
}

type APICamera struct {
	CameraFps        float64 `json:"camera_fps"`
	ProcessFps       float64 `json:"process_fps"`
	SkippedFps       float64 `json:"skipped_fps"`
	DetectionFps     float64 `json:"detection_fps"`
	DetectionEnabled bool    `json:"detection_enabled"`
	Pid              int     `json:"pid"`
	CapturePid       int     `json:"capture_pid"`
	FfmpegPid        int     `json:"ffmpeg_pid"`
	AudioRms         float64 `json:"audio_rms"`
	AudioDBFS        float64 `json:"audio_dBFS"`
}

type APIStats struct {
	Cameras map[string]APICamera `json:"cameras"`
}

func NewFNDFrigateApi(url string) *FNDFrigateApi {
	return &FNDFrigateApi{url: strings.TrimSuffix(url, "/")}
}

func (api *FNDFrigateApi) getSnapshotByID(id string) ([]byte, error) {
	snapshotURL := api.url + "/api/events/" + id + "/snapshot.jpg"
	var data bytes.Buffer

	response, err := http.Get(snapshotURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Statuscode: " + strconv.Itoa(response.StatusCode))
	}

	_, err = io.Copy(&data, response.Body)
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func (api *FNDFrigateApi) getCameras() (APIStats, error) {
	var c APIStats
	statsURL := api.url + "/api/stats"

	response, err := http.Get(statsURL)
	if err != nil {
		return c, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return c, errors.New("Statuscode: " + strconv.Itoa(response.StatusCode))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(body, &c)
	return c, nil
}
