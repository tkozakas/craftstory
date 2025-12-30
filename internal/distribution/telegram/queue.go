package telegram

import (
	"time"
)

const maxQueueSize = 5

type QueuedVideo struct {
	VideoPath   string    `json:"video_path"`
	PreviewPath string    `json:"preview_path,omitempty"`
	Title       string    `json:"title"`
	Script      string    `json:"script"`
	Tags        []string  `json:"tags,omitempty"`
	Topic       string    `json:"topic"`
	AddedAt     time.Time `json:"added_at"`
	MessageID   int       `json:"message_id,omitempty"`
	ChatID      int64     `json:"chat_id,omitempty"`
}

type VideoQueue struct {
	*PersistentQueue[QueuedVideo]
}

func NewVideoQueue(dataDir string) *VideoQueue {
	return &VideoQueue{
		PersistentQueue: NewPersistentQueue[QueuedVideo](dataDir, "video_queue.json", maxQueueSize),
	}
}

func (q *VideoQueue) Add(video QueuedVideo) error {
	video.AddedAt = time.Now()
	return q.PersistentQueue.Add(video)
}
