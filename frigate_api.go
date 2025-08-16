package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type FNDFrigateApi struct {
	url         string
	externalURL string
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

func NewFNDFrigateApi(url string, externalURL string) *FNDFrigateApi {
	LogDebug("API", "Creating new Frigate API client", fmt.Sprintf("URL: %s, ExternalURL: %s", url, externalURL))

	api := &FNDFrigateApi{
		url:         strings.TrimSuffix(url, "/"),
		externalURL: strings.TrimSuffix(externalURL, "/"),
	}

	LogDebug("API", "Frigate API client created", fmt.Sprintf("Internal URL: %s, External URL: %s", api.url, api.externalURL))
	return api
}

func (api *FNDFrigateApi) getSnapshotByID(id string) ([]byte, error) {
	LogDebug("API", "Fetching snapshot by ID", fmt.Sprintf("Event ID: %s", id))

	snapshotURL := api.url + "/api/events/" + id + "/snapshot.jpg"
	LogDebug("API", "Snapshot URL", fmt.Sprintf("URL: %s", snapshotURL))

	var data bytes.Buffer

	response, err := http.Get(snapshotURL)
	if err != nil {
		LogError("API", "Failed to fetch snapshot", fmt.Sprintf("Event ID: %s, URL: %s, Error: %s", id, snapshotURL, err.Error()))
		CaptureError(err, map[string]interface{}{
			"component": "frigate_api",
			"action":    "get_snapshot",
			"event_id":  id,
			"url":       snapshotURL,
		})
		return nil, err
	}
	defer response.Body.Close()

	LogDebug("API", "Snapshot response received", fmt.Sprintf("Event ID: %s, Status: %d", id, response.StatusCode))

	if response.StatusCode != http.StatusOK {
		LogError("API", "Snapshot request failed", fmt.Sprintf("Event ID: %s, Status: %d", id, response.StatusCode))
		err := errors.New("Statuscode: " + strconv.Itoa(response.StatusCode))
		CaptureError(err, map[string]interface{}{
			"component": "frigate_api",
			"action":    "get_snapshot",
			"event_id":  id,
			"url":       snapshotURL,
			"status":    response.StatusCode,
		})
		return nil, err
	}

	_, err = io.Copy(&data, response.Body)
	if err != nil {
		LogError("API", "Failed to read snapshot data", fmt.Sprintf("Event ID: %s, Error: %s", id, err.Error()))
		return nil, err
	}

	LogDebug("API", "Snapshot retrieved successfully", fmt.Sprintf("Event ID: %s, Size: %d bytes", id, data.Len()))
	return data.Bytes(), nil
}

func (api *FNDFrigateApi) getClipByID(id string) ([]byte, error) {
	LogDebug("API", "Fetching clip by ID", fmt.Sprintf("Event ID: %s", id))

	clipURL := api.url + "/api/events/" + id + "/clip.mp4"
	LogDebug("API", "Clip URL", fmt.Sprintf("URL: %s", clipURL))

	var data bytes.Buffer

	response, err := http.Get(clipURL)
	if err != nil {
		LogError("API", "Failed to fetch clip", fmt.Sprintf("Event ID: %s, URL: %s, Error: %s", id, clipURL, err.Error()))
		return nil, err
	}
	defer response.Body.Close()

	LogDebug("API", "Clip response received", fmt.Sprintf("Event ID: %s, Status: %d", id, response.StatusCode))

	if response.StatusCode != http.StatusOK {
		LogError("API", "Clip request failed", fmt.Sprintf("Event ID: %s, Status: %d", id, response.StatusCode))
		return nil, errors.New("Statuscode: " + strconv.Itoa(response.StatusCode))
	}

	_, err = io.Copy(&data, response.Body)
	if err != nil {
		LogError("API", "Failed to read clip data", fmt.Sprintf("Event ID: %s, Error: %s", id, err.Error()))
		return nil, err
	}

	LogDebug("API", "Clip retrieved successfully", fmt.Sprintf("Event ID: %s, Size: %d bytes", id, data.Len()))
	return data.Bytes(), nil
}

func (api *FNDFrigateApi) getClipURL(id string) string {
	LogDebug("API", "Generating clip URL", fmt.Sprintf("Event ID: %s", id))

	// Use external URL if configured, otherwise use internal URL
	var clipURL string
	if api.externalURL != "" {
		clipURL = api.externalURL + "/api/events/" + id + "/clip.mp4"
		LogDebug("API", "Using external URL for clip", fmt.Sprintf("Event ID: %s, URL: %s", id, clipURL))
	} else {
		clipURL = api.url + "/api/events/" + id + "/clip.mp4"
		LogDebug("API", "Using internal URL for clip", fmt.Sprintf("Event ID: %s, URL: %s", id, clipURL))
	}

	return clipURL
}

func (api *FNDFrigateApi) getSnapshotURL(id string) string {
	LogDebug("API", "Generating snapshot URL", fmt.Sprintf("Event ID: %s", id))

	// Use external URL if configured, otherwise use internal URL
	var snapshotURL string
	if api.externalURL != "" {
		snapshotURL = api.externalURL + "/api/events/" + id + "/snapshot.jpg"
		LogDebug("API", "Using external URL for snapshot", fmt.Sprintf("Event ID: %s, URL: %s", id, snapshotURL))
	} else {
		snapshotURL = api.url + "/api/events/" + id + "/snapshot.jpg"
		LogDebug("API", "Using internal URL for snapshot", fmt.Sprintf("Event ID: %s, URL: %s", id, snapshotURL))
	}

	return snapshotURL
}

func (api *FNDFrigateApi) getLiveSnapshotByCamera(camera string) ([]byte, error) {
	LogDebug("API", "Fetching live snapshot by camera", fmt.Sprintf("Camera: %s", camera))

	snapshotURL := api.url + "/api/" + camera + "/latest.jpg"
	LogDebug("API", "Live snapshot URL", fmt.Sprintf("Camera: %s, URL: %s", camera, snapshotURL))

	var data bytes.Buffer

	response, err := http.Get(snapshotURL)
	if err != nil {
		LogError("API", "Failed to fetch live snapshot", fmt.Sprintf("Camera: %s, URL: %s, Error: %s", camera, snapshotURL, err.Error()))
		return nil, err
	}
	defer response.Body.Close()

	LogDebug("API", "Live snapshot response received", fmt.Sprintf("Camera: %s, Status: %d", camera, response.StatusCode))

	if response.StatusCode != http.StatusOK {
		LogError("API", "Live snapshot request failed", fmt.Sprintf("Camera: %s, Status: %d", camera, response.StatusCode))
		return nil, errors.New("Statuscode: " + strconv.Itoa(response.StatusCode))
	}

	_, err = io.Copy(&data, response.Body)
	if err != nil {
		LogError("API", "Failed to read live snapshot data", fmt.Sprintf("Camera: %s, Error: %s", camera, err.Error()))
		return nil, err
	}

	LogDebug("API", "Live snapshot retrieved successfully", fmt.Sprintf("Camera: %s, Size: %d bytes", camera, data.Len()))
	return data.Bytes(), nil
}

func (api *FNDFrigateApi) getLiveSnapshotURL(camera string) string {
	LogDebug("API", "Generating live snapshot URL", fmt.Sprintf("Camera: %s", camera))

	// Use external URL if configured, otherwise use internal URL
	var snapshotURL string
	if api.externalURL != "" {
		snapshotURL = api.externalURL + "/api/" + camera + "/latest.jpg"
		LogDebug("API", "Using external URL for live snapshot", fmt.Sprintf("Camera: %s, URL: %s", camera, snapshotURL))
	} else {
		snapshotURL = api.url + "/api/" + camera + "/latest.jpg"
		LogDebug("API", "Using internal URL for live snapshot", fmt.Sprintf("Camera: %s, URL: %s", camera, snapshotURL))
	}

	return snapshotURL
}

func (api *FNDFrigateApi) getCameras() (APIStats, error) {
	LogDebug("API", "Fetching camera statistics", "")

	var c APIStats
	statsURL := api.url + "/api/stats"
	LogDebug("API", "Stats URL", fmt.Sprintf("URL: %s", statsURL))

	response, err := http.Get(statsURL)
	if err != nil {
		LogError("API", "Failed to fetch camera statistics", fmt.Sprintf("URL: %s, Error: %s", statsURL, err.Error()))
		return c, err
	}
	defer response.Body.Close()

	LogDebug("API", "Stats response received", fmt.Sprintf("Status: %d", response.StatusCode))

	if response.StatusCode != http.StatusOK {
		LogError("API", "Stats request failed", fmt.Sprintf("Status: %d", response.StatusCode))
		return c, errors.New("Statuscode: " + strconv.Itoa(response.StatusCode))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		LogError("API", "Failed to read stats response body", fmt.Sprintf("Error: %s", err.Error()))
		return c, err
	}

	LogDebug("API", "Stats response body read", fmt.Sprintf("Size: %d bytes", len(body)))

	err = json.Unmarshal(body, &c)
	if err != nil {
		LogError("API", "Failed to parse stats JSON", fmt.Sprintf("Error: %s", err.Error()))
		return c, err
	}

	LogInfo("API", "Camera statistics retrieved successfully", fmt.Sprintf("Cameras found: %d", len(c.Cameras)))
	return c, nil
}
