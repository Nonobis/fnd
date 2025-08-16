package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FacialRecognitionService manages facial recognition operations using CodeProject.AI
type FacialRecognitionService struct {
	config     *FNDFacialRecognitionConfiguration
	faceDB     *FaceDatabase
	httpClient *http.Client
	baseURL    string

	m sync.RWMutex
}

// CodeProjectAIResponse represents the response from CodeProject.AI
type CodeProjectAIResponse struct {
	Success bool                `json:"success"`
	Error   string              `json:"error,omitempty"`
	Data    []CodeProjectAIData `json:"data,omitempty"`
}

// CodeProjectAIData represents individual detection/recognition data
type CodeProjectAIData struct {
	Confidence float64 `json:"confidence"`
	Label      string  `json:"label"`
	X1         int     `json:"x1"`
	Y1         int     `json:"y1"`
	X2         int     `json:"x2"`
	Y2         int     `json:"y2"`
	FaceID     string  `json:"faceId,omitempty"`
}

// FaceDetectionResult represents the result of face detection
type FaceDetectionResult struct {
	FacesDetected int                 `json:"facesDetected"`
	Faces         []CodeProjectAIData `json:"faces"`
	ImagePath     string              `json:"imagePath"`
	Timestamp     time.Time           `json:"timestamp"`
}

// FaceRecognitionResult represents the result of face recognition
type FaceRecognitionResult struct {
	RecognizedFaces []RecognizedFace    `json:"recognizedFaces"`
	UnknownFaces    []CodeProjectAIData `json:"unknownFaces"`
	ImagePath       string              `json:"imagePath"`
	Timestamp       time.Time           `json:"timestamp"`
}

// RecognizedFace represents a recognized face with person information
type RecognizedFace struct {
	FaceData   CodeProjectAIData `json:"faceData"`
	Person     *FaceRecord       `json:"person"`
	Confidence float64           `json:"confidence"`
}

// NewFacialRecognitionService creates a new facial recognition service
func NewFacialRecognitionService(config *FNDFacialRecognitionConfiguration) *FacialRecognitionService {
	LogDebug("FACIAL_RECOGNITION", "Creating facial recognition service", fmt.Sprintf("Host: %s, Port: %d", config.CodeProjectAIHost, config.CodeProjectAIPort))

	protocol := "http"
	if config.CodeProjectAIUseSSL {
		protocol = "https"
	}

	baseURL := fmt.Sprintf("%s://%s:%d", protocol, config.CodeProjectAIHost, config.CodeProjectAIPort)

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: time.Duration(config.CodeProjectAITimeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Allow self-signed certificates
		},
	}

	service := &FacialRecognitionService{
		config:     config,
		httpClient: httpClient,
		baseURL:    baseURL,
		faceDB:     &FaceDatabase{Faces: make([]FaceRecord, 0)},
	}

	// Load face database
	if err := service.loadFaceDatabase(); err != nil {
		LogWarn("FACIAL_RECOGNITION", "Failed to load face database", err.Error())
	}

	LogInfo("FACIAL_RECOGNITION", "Facial recognition service created", fmt.Sprintf("Base URL: %s", baseURL))
	return service
}

// DetectFaces detects faces in an image using CodeProject.AI
func (s *FacialRecognitionService) DetectFaces(imageData []byte) (*FaceDetectionResult, error) {
	if !s.config.Enabled || !s.config.FaceDetectionEnabled {
		LogDebug("FACIAL_RECOGNITION", "Face detection disabled", "")
		return &FaceDetectionResult{FacesDetected: 0, Faces: []CodeProjectAIData{}}, nil
	}

	LogDebug("FACIAL_RECOGNITION", "Detecting faces in image", fmt.Sprintf("Image size: %d bytes", len(imageData)))

	// Prepare the request
	url := fmt.Sprintf("%s/v1/vision/face", s.baseURL)

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add image file
	part, err := writer.CreateFormFile("image", "snapshot.jpg")
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to create form file", err.Error())
		return nil, err
	}

	_, err = part.Write(imageData)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to write image data", err.Error())
		return nil, err
	}

	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to create request", err.Error())
		return nil, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to send face detection request", err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to read response body", err.Error())
		return nil, err
	}

	// Parse response
	var apiResponse CodeProjectAIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to parse API response", err.Error())
		return nil, err
	}

	if !apiResponse.Success {
		LogError("FACIAL_RECOGNITION", "Face detection API error", apiResponse.Error)
		return nil, fmt.Errorf("face detection failed: %s", apiResponse.Error)
	}

	result := &FaceDetectionResult{
		FacesDetected: len(apiResponse.Data),
		Faces:         apiResponse.Data,
		Timestamp:     time.Now(),
	}

	LogInfo("FACIAL_RECOGNITION", "Face detection completed", fmt.Sprintf("Faces detected: %d", result.FacesDetected))
	return result, nil
}

// RecognizeFaces recognizes faces in an image using CodeProject.AI
func (s *FacialRecognitionService) RecognizeFaces(imageData []byte) (*FaceRecognitionResult, error) {
	if !s.config.Enabled || !s.config.FaceRecognitionEnabled {
		LogDebug("FACIAL_RECOGNITION", "Face recognition disabled", "")
		return &FaceRecognitionResult{RecognizedFaces: []RecognizedFace{}, UnknownFaces: []CodeProjectAIData{}}, nil
	}

	LogDebug("FACIAL_RECOGNITION", "Recognizing faces in image", fmt.Sprintf("Image size: %d bytes", len(imageData)))

	// First detect faces
	detectionResult, err := s.DetectFaces(imageData)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Face detection failed during recognition", err.Error())
		return nil, err
	}

	if detectionResult.FacesDetected == 0 {
		LogDebug("FACIAL_RECOGNITION", "No faces detected for recognition", "")
		return &FaceRecognitionResult{RecognizedFaces: []RecognizedFace{}, UnknownFaces: []CodeProjectAIData{}}, nil
	}

	// Prepare the request for face recognition
	url := fmt.Sprintf("%s/v1/vision/face/recognize", s.baseURL)

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add image file
	part, err := writer.CreateFormFile("image", "snapshot.jpg")
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to create form file", err.Error())
		return nil, err
	}

	_, err = part.Write(imageData)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to write image data", err.Error())
		return nil, err
	}

	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to create request", err.Error())
		return nil, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to send face recognition request", err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to read response body", err.Error())
		return nil, err
	}

	// Parse response
	var apiResponse CodeProjectAIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to parse API response", err.Error())
		return nil, err
	}

	if !apiResponse.Success {
		LogError("FACIAL_RECOGNITION", "Face recognition API error", apiResponse.Error)
		return nil, fmt.Errorf("face recognition failed: %s", apiResponse.Error)
	}

	// Process recognition results
	result := &FaceRecognitionResult{
		RecognizedFaces: []RecognizedFace{},
		UnknownFaces:    []CodeProjectAIData{},
		Timestamp:       time.Now(),
	}

	for _, faceData := range apiResponse.Data {
		if faceData.Label != "unknown" && faceData.Confidence > 0.7 {
			// Try to find person in database
			person := s.findPersonByFaceID(faceData.FaceID)
			if person != nil {
				result.RecognizedFaces = append(result.RecognizedFaces, RecognizedFace{
					FaceData:   faceData,
					Person:     person,
					Confidence: faceData.Confidence,
				})
			} else {
				result.UnknownFaces = append(result.UnknownFaces, faceData)
			}
		} else {
			result.UnknownFaces = append(result.UnknownFaces, faceData)
		}
	}

	LogInfo("FACIAL_RECOGNITION", "Face recognition completed", fmt.Sprintf("Recognized: %d, Unknown: %d", len(result.RecognizedFaces), len(result.UnknownFaces)))
	return result, nil
}

// AddFaceToDatabase adds a new face to the database
func (s *FacialRecognitionService) AddFaceToDatabase(faceRecord *FaceRecord, imageData []byte) error {
	LogDebug("FACIAL_RECOGNITION", "Adding face to database", fmt.Sprintf("Person: %s %s", faceRecord.FirstName, faceRecord.LastName))

	// Generate unique ID if not provided
	if faceRecord.ID == "" {
		faceRecord.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	faceRecord.CreatedAt = now
	faceRecord.UpdatedAt = now
	faceRecord.IsActive = true

	// Save image to file system
	imagePath, err := s.saveImageToFile(imageData, faceRecord.FirstName, faceRecord.LastName, faceRecord.ID)
	if err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to save face image", err.Error())
		return err
	}

	faceRecord.ImagePath = imagePath

	// Register face with CodeProject.AI
	if err := s.registerFaceWithAI(faceRecord, imageData); err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to register face with AI", err.Error())
		return err
	}

	// Add to database
	s.faceDB.m.Lock()
	s.faceDB.Faces = append(s.faceDB.Faces, *faceRecord)
	s.faceDB.m.Unlock()

	// Save database
	if err := s.saveFaceDatabase(); err != nil {
		LogError("FACIAL_RECOGNITION", "Failed to save face database", err.Error())
		return err
	}

	LogInfo("FACIAL_RECOGNITION", "Face added to database", fmt.Sprintf("Person: %s %s, ID: %s", faceRecord.FirstName, faceRecord.LastName, faceRecord.ID))
	return nil
}

// UpdateFaceInDatabase updates an existing face in the database
func (s *FacialRecognitionService) UpdateFaceInDatabase(faceRecord *FaceRecord) error {
	LogDebug("FACIAL_RECOGNITION", "Updating face in database", fmt.Sprintf("ID: %s", faceRecord.ID))

	s.faceDB.m.Lock()
	defer s.faceDB.m.Unlock()

	// Find and update the face record
	for i, face := range s.faceDB.Faces {
		if face.ID == faceRecord.ID {
			faceRecord.UpdatedAt = time.Now()
			s.faceDB.Faces[i] = *faceRecord

			// Save database
			if err := s.saveFaceDatabase(); err != nil {
				LogError("FACIAL_RECOGNITION", "Failed to save face database", err.Error())
				return err
			}

			LogInfo("FACIAL_RECOGNITION", "Face updated in database", fmt.Sprintf("ID: %s", faceRecord.ID))
			return nil
		}
	}

	return fmt.Errorf("face with ID %s not found", faceRecord.ID)
}

// UpdateFaceImage updates the image for an existing face
func (s *FacialRecognitionService) UpdateFaceImage(faceID string, imageData []byte) error {
	LogDebug("FACIAL_RECOGNITION", "Updating face image", fmt.Sprintf("ID: %s", faceID))

	s.faceDB.m.Lock()
	defer s.faceDB.m.Unlock()

	// Find the face record
	for i, face := range s.faceDB.Faces {
		if face.ID == faceID {
			// Remove old image file
			if face.ImagePath != "" {
				if err := os.Remove(face.ImagePath); err != nil {
					LogWarn("FACIAL_RECOGNITION", "Failed to delete old face image file", err.Error())
				}
			}

			// Save new image with organized structure
			newImagePath, err := s.saveImageToFile(imageData, face.FirstName, face.LastName, face.ID)
			if err != nil {
				LogError("FACIAL_RECOGNITION", "Failed to save new face image", err.Error())
				return err
			}

			// Update face record
			s.faceDB.Faces[i].ImagePath = newImagePath
			s.faceDB.Faces[i].UpdatedAt = time.Now()

			// Re-register with CodeProject.AI
			if err := s.registerFaceWithAI(&s.faceDB.Faces[i], imageData); err != nil {
				LogError("FACIAL_RECOGNITION", "Failed to re-register face with AI", err.Error())
				return err
			}

			// Save database
			if err := s.saveFaceDatabase(); err != nil {
				LogError("FACIAL_RECOGNITION", "Failed to save face database", err.Error())
				return err
			}

			LogInfo("FACIAL_RECOGNITION", "Face image updated", fmt.Sprintf("ID: %s", faceID))
			return nil
		}
	}

	return fmt.Errorf("face with ID %s not found", faceID)
}

// DeleteFaceFromDatabase removes a face from the database
func (s *FacialRecognitionService) DeleteFaceFromDatabase(faceID string) error {
	LogDebug("FACIAL_RECOGNITION", "Deleting face from database", fmt.Sprintf("ID: %s", faceID))

	s.faceDB.m.Lock()
	defer s.faceDB.m.Unlock()

	// Find and remove the face record
	for i, face := range s.faceDB.Faces {
		if face.ID == faceID {
			// Remove from slice
			s.faceDB.Faces = append(s.faceDB.Faces[:i], s.faceDB.Faces[i+1:]...)

			// Delete image file
			if face.ImagePath != "" {
				if err := os.Remove(face.ImagePath); err != nil {
					LogWarn("FACIAL_RECOGNITION", "Failed to delete face image file", err.Error())
				} else {
					// Try to remove empty directory
					s.cleanupEmptyDirectory(face.ImagePath)
				}
			}

			// Remove from CodeProject.AI
			if err := s.removeFaceFromAI(face.FaceID); err != nil {
				LogWarn("FACIAL_RECOGNITION", "Failed to remove face from AI", err.Error())
			}

			// Save database
			if err := s.saveFaceDatabase(); err != nil {
				LogError("FACIAL_RECOGNITION", "Failed to save face database", err.Error())
				return err
			}

			LogInfo("FACIAL_RECOGNITION", "Face deleted from database", fmt.Sprintf("ID: %s", faceID))
			return nil
		}
	}

	return fmt.Errorf("face with ID %s not found", faceID)
}

// GetPersonImages returns all images for a specific person
func (s *FacialRecognitionService) GetPersonImages(firstName, lastName string) []string {
	personDir := sanitizeDirectoryName(fmt.Sprintf("%s_%s", firstName, lastName))
	personPath := filepath.Join("fnd_conf", personDir)

	var images []string

	// Check if directory exists
	if _, err := os.Stat(personPath); os.IsNotExist(err) {
		return images
	}

	// Read directory
	entries, err := os.ReadDir(personPath)
	if err != nil {
		LogDebug("FACIAL_RECOGNITION", "Failed to read person directory", personPath)
		return images
	}

	// Collect image files
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jpg") {
			images = append(images, filepath.Join(personPath, entry.Name()))
		}
	}

	return images
}

// GetAllFaces returns all faces in the database
func (s *FacialRecognitionService) GetAllFaces() []FaceRecord {
	s.faceDB.m.RLock()
	defer s.faceDB.m.RUnlock()

	faces := make([]FaceRecord, len(s.faceDB.Faces))
	copy(faces, s.faceDB.Faces)
	return faces
}

// GetFaceByID returns a specific face by ID
func (s *FacialRecognitionService) GetFaceByID(faceID string) *FaceRecord {
	s.faceDB.m.RLock()
	defer s.faceDB.m.RUnlock()

	for _, face := range s.faceDB.Faces {
		if face.ID == faceID {
			return &face
		}
	}
	return nil
}

// findPersonByFaceID finds a person by their face ID
func (s *FacialRecognitionService) findPersonByFaceID(faceID string) *FaceRecord {
	s.faceDB.m.RLock()
	defer s.faceDB.m.RUnlock()

	for _, face := range s.faceDB.Faces {
		if face.FaceID == faceID && face.IsActive {
			return &face
		}
	}
	return nil
}

// registerFaceWithAI registers a face with CodeProject.AI
func (s *FacialRecognitionService) registerFaceWithAI(faceRecord *FaceRecord, imageData []byte) error {
	LogDebug("FACIAL_RECOGNITION", "Registering face with AI", fmt.Sprintf("Person: %s %s", faceRecord.FirstName, faceRecord.LastName))

	url := fmt.Sprintf("%s/v1/vision/face/register", s.baseURL)

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add image file
	part, err := writer.CreateFormFile("image", "face.jpg")
	if err != nil {
		return err
	}

	_, err = part.Write(imageData)
	if err != nil {
		return err
	}

	// Add person name
	if err := writer.WriteField("name", fmt.Sprintf("%s %s", faceRecord.FirstName, faceRecord.LastName)); err != nil {
		return err
	}

	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse response
	var apiResponse CodeProjectAIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return err
	}

	if !apiResponse.Success {
		return fmt.Errorf("face registration failed: %s", apiResponse.Error)
	}

	// Extract face ID from response
	if len(apiResponse.Data) > 0 {
		faceRecord.FaceID = apiResponse.Data[0].FaceID
	}

	LogInfo("FACIAL_RECOGNITION", "Face registered with AI", fmt.Sprintf("Person: %s %s, FaceID: %s", faceRecord.FirstName, faceRecord.LastName, faceRecord.FaceID))
	return nil
}

// removeFaceFromAI removes a face from CodeProject.AI
func (s *FacialRecognitionService) removeFaceFromAI(faceID string) error {
	if faceID == "" {
		return nil
	}

	LogDebug("FACIAL_RECOGNITION", "Removing face from AI", fmt.Sprintf("FaceID: %s", faceID))

	url := fmt.Sprintf("%s/v1/vision/face/delete", s.baseURL)

	// Create form data
	formData := fmt.Sprintf("faceId=%s", faceID)

	// Create request
	req, err := http.NewRequest("POST", url, strings.NewReader(formData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse response
	var apiResponse CodeProjectAIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return err
	}

	if !apiResponse.Success {
		return fmt.Errorf("face deletion failed: %s", apiResponse.Error)
	}

	LogInfo("FACIAL_RECOGNITION", "Face removed from AI", fmt.Sprintf("FaceID: %s", faceID))
	return nil
}

// saveImageToFile saves an image to the file system with organized structure
func (s *FacialRecognitionService) saveImageToFile(imageData []byte, firstName, lastName, faceID string) (string, error) {
	// Create person directory name (sanitized)
	personDir := sanitizeDirectoryName(fmt.Sprintf("%s_%s", firstName, lastName))

	// Create full path: face_db/person_name/guid.jpg
	imagePath := filepath.Join("fnd_conf", personDir, faceID+".jpg")

	// Create directory if it doesn't exist
	dir := filepath.Dir(imagePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Write image file
	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		return "", err
	}

	return imagePath, nil
}

// sanitizeDirectoryName creates a safe directory name from person names
func sanitizeDirectoryName(name string) string {
	// Replace spaces and special characters with underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	sanitized := reg.ReplaceAllString(name, "_")

	// Remove multiple consecutive underscores
	reg = regexp.MustCompile(`_+`)
	sanitized = reg.ReplaceAllString(sanitized, "_")

	// Remove leading/trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	// Ensure it's not empty
	if sanitized == "" {
		sanitized = "unknown_person"
	}

	return sanitized
}

// cleanupEmptyDirectory removes empty directories after file deletion
func (s *FacialRecognitionService) cleanupEmptyDirectory(filePath string) {
	dir := filepath.Dir(filePath)

	// Check if directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		LogDebug("FACIAL_RECOGNITION", "Failed to read directory for cleanup", dir)
		return
	}

	// If directory is empty, remove it
	if len(entries) == 0 {
		if err := os.Remove(dir); err != nil {
			LogDebug("FACIAL_RECOGNITION", "Failed to remove empty directory", dir)
		} else {
			LogDebug("FACIAL_RECOGNITION", "Removed empty directory", dir)
		}
	}
}

// loadFaceDatabase loads the face database from file
func (s *FacialRecognitionService) loadFaceDatabase() error {
	dbPath := "fnd_conf/faces.json"

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Check if database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		LogDebug("FACIAL_RECOGNITION", "Face database file does not exist, creating new one", dbPath)
		return s.saveFaceDatabase()
	}

	// Read database file
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return err
	}

	// Parse JSON
	if err := json.Unmarshal(data, s.faceDB); err != nil {
		return err
	}

	LogInfo("FACIAL_RECOGNITION", "Face database loaded", fmt.Sprintf("Faces: %d", len(s.faceDB.Faces)))
	return nil
}

// saveFaceDatabase saves the face database to file
func (s *FacialRecognitionService) saveFaceDatabase() error {
	dbPath := "fnd_conf/faces.json"

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(s.faceDB, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(dbPath, data, 0644)
}

// TestConnection tests the connection to CodeProject.AI
func (s *FacialRecognitionService) TestConnection() error {
	LogDebug("FACIAL_RECOGNITION", "Testing connection to CodeProject.AI", s.baseURL)

	// Try to connect to the base URL first to check if the service is reachable
	url := fmt.Sprintf("%s/", s.baseURL)

	// Create a simple test request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept any 2xx or 4xx status code as the service is reachable
	// 4xx means the service is running but the endpoint doesn't exist (which is expected for root path)
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		LogInfo("FACIAL_RECOGNITION", "Connection test successful", s.baseURL)
		return nil
	}

	return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}
