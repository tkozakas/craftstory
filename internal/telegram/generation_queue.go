package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxGenerationQueueSize = 10

type GenerationRequest struct {
	Topic      string    `json:"topic"`
	ChatID     int64     `json:"chat_id"`
	FromReddit bool      `json:"from_reddit"`
	AddedAt    time.Time `json:"added_at"`
	Status     string    `json:"status"`
}

type GenerationQueue struct {
	requests []GenerationRequest
	mu       sync.RWMutex
	dataFile string
}

func NewGenerationQueue(dataDir string) *GenerationQueue {
	q := &GenerationQueue{
		requests: make([]GenerationRequest, 0, maxGenerationQueueSize),
		dataFile: filepath.Join(dataDir, "generation_queue.json"),
	}
	q.load()
	return q
}

func (q *GenerationQueue) Add(request GenerationRequest) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.requests) >= maxGenerationQueueSize {
		return fmt.Errorf("generation queue is full (%d/%d)", len(q.requests), maxGenerationQueueSize)
	}

	request.AddedAt = time.Now()
	request.Status = "pending"
	q.requests = append(q.requests, request)
	q.save()
	return nil
}

func (q *GenerationQueue) Pop() (*GenerationRequest, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, req := range q.requests {
		if req.Status == "pending" {
			q.requests[i].Status = "generating"
			q.save()
			return &q.requests[i], nil
		}
	}
	return nil, fmt.Errorf("no pending requests")
}

func (q *GenerationQueue) Complete(chatID int64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := len(q.requests) - 1; i >= 0; i-- {
		if q.requests[i].ChatID == chatID && q.requests[i].Status == "generating" {
			q.requests = append(q.requests[:i], q.requests[i+1:]...)
			break
		}
	}
	q.save()
}

func (q *GenerationQueue) Fail(chatID int64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := len(q.requests) - 1; i >= 0; i-- {
		if q.requests[i].ChatID == chatID && q.requests[i].Status == "generating" {
			q.requests = append(q.requests[:i], q.requests[i+1:]...)
			break
		}
	}
	q.save()
}

func (q *GenerationQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.requests)
}

func (q *GenerationQueue) IsGenerating() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, req := range q.requests {
		if req.Status == "generating" {
			return true
		}
	}
	return false
}

func (q *GenerationQueue) List() []GenerationRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]GenerationRequest, len(q.requests))
	copy(result, q.requests)
	return result
}

func (q *GenerationQueue) IsFull() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.requests) >= maxGenerationQueueSize
}

func (q *GenerationQueue) load() {
	data, err := os.ReadFile(q.dataFile)
	if err != nil {
		return
	}

	var requests []GenerationRequest
	if err := json.Unmarshal(data, &requests); err != nil {
		return
	}

	for i := range requests {
		if requests[i].Status == "generating" {
			requests[i].Status = "pending"
		}
	}
	q.requests = requests
}

func (q *GenerationQueue) save() {
	data, err := json.MarshalIndent(q.requests, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Dir(q.dataFile), 0755)
	_ = os.WriteFile(q.dataFile, data, 0644)
}
