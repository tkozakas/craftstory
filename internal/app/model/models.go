package model

import "time"

const (
	StatusPending    VideoStatus = "pending"
	StatusProcessing VideoStatus = "processing"
	StatusCompleted  VideoStatus = "completed"
	StatusUploaded   VideoStatus = "uploaded"
	StatusFailed     VideoStatus = "failed"
)

type VideoStatus string

type Script struct {
	ID        int64
	Topic     string
	Content   string
	Hook      string
	CreatedAt time.Time
}

type AudioTrack struct {
	ID        int64
	ScriptID  int64
	FilePath  string
	Duration  float64
	CreatedAt time.Time
}

type Video struct {
	ID             int64
	ScriptID       int64
	AudioID        int64
	BackgroundPath string
	OutputPath     string
	Duration       float64
	Status         VideoStatus
	CreatedAt      time.Time
}

type YouTubeUpload struct {
	ID          int64
	VideoID     int64
	YouTubeID   string
	Title       string
	Description string
	Tags        []string
	UploadedAt  time.Time
}
