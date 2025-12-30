package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type PersistentQueue[T any] struct {
	items    []T
	mu       sync.RWMutex
	dataFile string
	maxSize  int
}

func NewPersistentQueue[T any](dataDir, filename string, maxSize int) *PersistentQueue[T] {
	q := &PersistentQueue[T]{
		items:    make([]T, 0, maxSize),
		dataFile: filepath.Join(dataDir, filename),
		maxSize:  maxSize,
	}
	q.load()
	return q
}

func (q *PersistentQueue[T]) Add(item T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) >= q.maxSize {
		return fmt.Errorf("queue is full (%d/%d)", len(q.items), q.maxSize)
	}

	q.items = append(q.items, item)
	q.save()
	return nil
}

func (q *PersistentQueue[T]) Pop() (*T, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.save()
	return &item, nil
}

func (q *PersistentQueue[T]) Peek() (*T, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.items) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	return &q.items[0], nil
}

func (q *PersistentQueue[T]) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

func (q *PersistentQueue[T]) List() []T {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]T, len(q.items))
	copy(result, q.items)
	return result
}

func (q *PersistentQueue[T]) IsFull() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items) >= q.maxSize
}

func (q *PersistentQueue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make([]T, 0, q.maxSize)
	q.save()
}

func (q *PersistentQueue[T]) Update(fn func(items []T) []T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = fn(q.items)
	q.save()
}

func (q *PersistentQueue[T]) FindAndRemove(predicate func(T) bool) *T {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, item := range q.items {
		if predicate(item) {
			q.items = append(q.items[:i], q.items[i+1:]...)
			q.save()
			return &item
		}
	}
	return nil
}

func (q *PersistentQueue[T]) FindFirst(predicate func(T) bool) *T {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for i := range q.items {
		if predicate(q.items[i]) {
			return &q.items[i]
		}
	}
	return nil
}

func (q *PersistentQueue[T]) load() {
	data, err := os.ReadFile(q.dataFile)
	if err != nil {
		return
	}

	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}

	q.items = items
}

func (q *PersistentQueue[T]) save() {
	data, err := json.MarshalIndent(q.items, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Dir(q.dataFile), 0755)
	_ = os.WriteFile(q.dataFile, data, 0644)
}
