# Facial Recognition System

## Overview

The Facial Recognition system integrates with CodeProject.AI to provide face detection and recognition capabilities for FND (Frigate Notification Daemon). When enabled, the system will analyze images from Frigate events and attempt to identify known individuals.

## Features

- **Face Detection**: Automatically detects faces in event images
- **Face Recognition**: Identifies known individuals from the face database
- **Face Database Management**: Add, edit, and delete face records with personal information
- **Integration with Notifications**: Recognition results are included in notification messages
- **Template Variables**: New template variables for facial recognition data

## Configuration

### CodeProject.AI Setup

1. **Install CodeProject.AI**: Follow the official installation guide at [CodeProject.AI](https://www.codeproject.com/Articles/5322557/CodeProject-AI-Server-AI-the-easy-way)
2. **Enable Face Recognition Module**: Ensure the face recognition module is installed and enabled
3. **Note the API Endpoint**: Default is usually `http://localhost:8000`

### FND Configuration

1. **Access Facial Recognition Settings**: Navigate to Settings → Facial Recognition in the FND web interface
2. **Enable the System**: Check "Enable Facial Recognition"
3. **Configure CodeProject.AI Connection**:
   - **Host**: CodeProject.AI server hostname/IP
   - **Port**: CodeProject.AI server port (default: 8000)
   - **Use SSL**: Enable if using HTTPS
   - **Timeout**: Connection timeout in seconds
4. **Configure Features**:
   - **Face Detection**: Enable to detect faces in images
   - **Face Recognition**: Enable to identify known individuals
   - **Database Path**: Local path for storing face database
5. **Test Connection**: Use the "Test Connection" button to verify CodeProject.AI connectivity

## Face Database Management

### Adding Faces

1. Navigate to Settings → Face Management
2. Click "Add New Face"
3. Fill in the form:
   - **First Name**: Person's first name
   - **Last Name**: Person's last name
   - **Email**: Contact email (optional)
   - **Phone**: Contact phone (optional)
   - **Notes**: Additional information (optional)
   - **Face Image**: Upload a clear image of the person's face
4. Click "Add Face"

### Managing Faces

- **View All Faces**: See a list of all registered faces
- **Edit Face**: Modify personal information or status
- **Delete Face**: Remove face from database (also removes from CodeProject.AI)

## Integration with Notifications

### Template Variables

The following new template variables are available for notifications:

- `{{.HasFaces}}`: Boolean indicating if faces were detected
- `{{.FaceCount}}`: Total number of faces detected
- `{{.RecognizedFaces}}`: List of recognized faces with confidence scores
- `{{.UnknownFaces}}`: Number of unknown faces detected

### Example Template

```
A new event has occurred: {{.Object}} at {{.Camera}} on {{.Date}} {{.Time}}
{{if .HasVideo}}🎥 Video: {{.VideoURL}}{{end}}
{{if .HasSnapshot}}📸 Snapshot attached{{end}}
{{if .HasFaces}}
👤 Faces detected: {{.FaceCount}}
{{if .RecognizedFaces}}✅ Recognized: {{.RecognizedFaces}}{{end}}
{{if .UnknownFaces}}❓ Unknown: {{.UnknownFaces}}{{end}}
{{end}}
```

### Notification Flow

1. **Event Detection**: Frigate detects an object (e.g., person)
2. **Image Analysis**: If facial recognition is enabled, the image is sent to CodeProject.AI
3. **Face Detection**: CodeProject.AI detects faces in the image
4. **Face Recognition**: Known faces are identified from the database
5. **Notification**: Results are included in the notification message

## File Structure

```
face_db/
├── faces.json          # Face database metadata
└── images/             # Stored face images
    ├── face1.jpg
    ├── face2.jpg
    └── ...
```

## Troubleshooting

### Common Issues

1. **Connection Failed**:
   - Verify CodeProject.AI is running
   - Check host and port settings
   - Ensure firewall allows the connection

2. **No Faces Detected**:
   - Verify face detection is enabled
   - Check image quality (clear, well-lit faces)
   - Ensure CodeProject.AI face module is active

3. **Recognition Not Working**:
   - Verify face recognition is enabled
   - Check that faces are properly registered in the database
   - Ensure face images are clear and representative

4. **Database Issues**:
   - Check file permissions for the database path
   - Verify sufficient disk space
   - Check logs for database errors

### Logs

Facial recognition activities are logged with the prefix `FACIAL_RECOGNITION`. Check the logs for detailed information about:

- Service initialization
- Face detection/recognition attempts
- Database operations
- Connection issues

## Security Considerations

- **Local Storage**: Face database is stored locally on the FND server
- **Image Privacy**: Face images are stored in the local file system
- **Network Security**: Ensure secure connection to CodeProject.AI if using HTTPS
- **Access Control**: Restrict access to the FND web interface

## Performance

- **Processing Time**: Face recognition adds processing time to notifications
- **Image Size**: Larger images take longer to process
- **Database Size**: Large face databases may impact performance
- **Network Latency**: CodeProject.AI response time affects notification delivery

## API Endpoints

### Configuration
- `GET /facial-recognition` - Configuration page
- `POST /api/facial-recognition/config` - Update configuration
- `POST /api/facial-recognition/test` - Test connection

### Face Management
- `GET /face-management` - Face management page
- `GET /api/facial-recognition/faces` - List all faces
- `GET /api/facial-recognition/faces/add` - Add face form
- `POST /api/facial-recognition/faces` - Save new face
- `GET /api/facial-recognition/faces/edit/:id` - Edit face form
- `PUT /api/facial-recognition/faces/:id` - Update face
- `DELETE /api/facial-recognition/faces/:id` - Delete face

### Status
- `GET /api/overview/facial-recognition-status` - System status for overview

## Future Enhancements

- **Batch Processing**: Process multiple images simultaneously
- **Face Quality Assessment**: Automatically assess face image quality
- **Advanced Recognition**: Support for multiple face angles and expressions
- **Integration with External Databases**: Connect to external identity databases
- **Real-time Recognition**: Stream processing for live video feeds
