package telegram

import (
	"fmt"
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
	*PersistentQueue[GenerationRequest]
}

func NewGenerationQueue(dataDir string) *GenerationQueue {
	q := &GenerationQueue{
		PersistentQueue: NewPersistentQueue[GenerationRequest](dataDir, "generation_queue.json", maxGenerationQueueSize),
	}
	q.resetStuckGenerations()
	return q
}

func (q *GenerationQueue) resetStuckGenerations() {
	q.Update(func(items []GenerationRequest) []GenerationRequest {
		for i := range items {
			if items[i].Status == "generating" {
				items[i].Status = "pending"
			}
		}
		return items
	})
}

func (q *GenerationQueue) Add(request GenerationRequest) error {
	request.AddedAt = time.Now()
	request.Status = "pending"
	return q.PersistentQueue.Add(request)
}

func (q *GenerationQueue) Pop() (*GenerationRequest, error) {
	req := q.FindFirst(func(r GenerationRequest) bool {
		return r.Status == "pending"
	})
	if req == nil {
		return nil, fmt.Errorf("no pending requests")
	}

	q.Update(func(items []GenerationRequest) []GenerationRequest {
		for i := range items {
			if items[i].ChatID == req.ChatID && items[i].Status == "pending" {
				items[i].Status = "generating"
				break
			}
		}
		return items
	})

	return q.FindFirst(func(r GenerationRequest) bool {
		return r.ChatID == req.ChatID && r.Status == "generating"
	}), nil
}

func (q *GenerationQueue) Complete(chatID int64) {
	q.FindAndRemove(func(r GenerationRequest) bool {
		return r.ChatID == chatID && r.Status == "generating"
	})
}

func (q *GenerationQueue) Fail(chatID int64) {
	q.FindAndRemove(func(r GenerationRequest) bool {
		return r.ChatID == chatID && r.Status == "generating"
	})
}

func (q *GenerationQueue) IsGenerating() bool {
	return q.FindFirst(func(r GenerationRequest) bool {
		return r.Status == "generating"
	}) != nil
}
