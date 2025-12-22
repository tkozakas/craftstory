package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxQueueSize = 5

type QueuedVideo struct {
	VideoPath string    `json:"video_path"`
	Title     string    `json:"title"`
	Script    string    `json:"script"`
	Topic     string    `json:"topic"`
	AddedAt   time.Time `json:"added_at"`
}

type VideoQueue struct {
	videos   []QueuedVideo
	mu       sync.RWMutex
	dataFile string
}

func NewVideoQueue(dataDir string) *VideoQueue {
	q := &VideoQueue{
		videos:   make([]QueuedVideo, 0, maxQueueSize),
		dataFile: filepath.Join(dataDir, "video_queue.json"),
	}
	q.load()
	return q
}

func (q *VideoQueue) Add(video QueuedVideo) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.videos) >= maxQueueSize {
		return fmt.Errorf("queue is full (%d/%d videos)", len(q.videos), maxQueueSize)
	}

	video.AddedAt = time.Now()
	q.videos = append(q.videos, video)
	q.save()
	return nil
}

func (q *VideoQueue) Pop() (*QueuedVideo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.videos) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	video := q.videos[0]
	q.videos = q.videos[1:]
	q.save()
	return &video, nil
}

func (q *VideoQueue) Peek() (*QueuedVideo, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.videos) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	return &q.videos[0], nil
}

func (q *VideoQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.videos)
}

func (q *VideoQueue) List() []QueuedVideo {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]QueuedVideo, len(q.videos))
	copy(result, q.videos)
	return result
}

func (q *VideoQueue) IsFull() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.videos) >= maxQueueSize
}

func (q *VideoQueue) load() {
	data, err := os.ReadFile(q.dataFile)
	if err != nil {
		return
	}

	var videos []QueuedVideo
	if err := json.Unmarshal(data, &videos); err != nil {
		return
	}

	q.videos = videos
}

func (q *VideoQueue) save() {
	data, err := json.MarshalIndent(q.videos, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Dir(q.dataFile), 0755)
	_ = os.WriteFile(q.dataFile, data, 0644)
}
