package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	callbackApprove = "approve"
	callbackReject  = "reject"
	pollTimeout     = 10 * time.Minute
)

type ApprovalService struct {
	client        *Client
	defaultChatID int64
	reviewers     map[int64]Reviewer
	reviewersMu   sync.RWMutex
	dataFile      string
	pollOffset    int
	stopPoll      chan struct{}
	pollWg        sync.WaitGroup
	queue         *VideoQueue
	pendingVideo  *QueuedVideo
	pendingMu     sync.Mutex
	resultChan    chan *ApprovalResult
}

type ApprovalRequest struct {
	VideoPath string
	Title     string
	Script    string
}

type ApprovalResult struct {
	Approved   bool
	Message    string
	ReviewerID int64
}

func NewApprovalService(client *Client, dataDir string, defaultChatID int64) *ApprovalService {
	svc := &ApprovalService{
		client:        client,
		defaultChatID: defaultChatID,
		reviewers:     make(map[int64]Reviewer),
		dataFile:      filepath.Join(dataDir, "reviewers.json"),
		stopPoll:      make(chan struct{}),
		queue:         NewVideoQueue(dataDir),
		resultChan:    make(chan *ApprovalResult, 1),
	}
	svc.loadReviewers()
	return svc
}

func (s *ApprovalService) StartBot() {
	s.pollWg.Add(1)
	go s.pollCommands()
}

func (s *ApprovalService) StopBot() {
	close(s.stopPoll)
	s.pollWg.Wait()
}

func (s *ApprovalService) Queue() *VideoQueue {
	return s.queue
}

func (s *ApprovalService) QueueVideo(video QueuedVideo) error {
	if err := s.queue.Add(video); err != nil {
		return err
	}
	slog.Info("Video queued for review", "title", video.Title, "queue_size", s.queue.Len())

	if s.defaultChatID != 0 {
		s.sendNextVideoTo(s.defaultChatID)
	} else {
		s.notifyQueueStatus()
	}
	return nil
}

func (s *ApprovalService) sendNextVideoTo(chatID int64) {
	s.pendingMu.Lock()
	if s.pendingVideo != nil {
		s.pendingMu.Unlock()
		return
	}

	video, err := s.queue.Pop()
	if err != nil {
		s.pendingMu.Unlock()
		return
	}

	s.pendingVideo = video
	s.pendingMu.Unlock()

	caption := fmt.Sprintf("*%s*\n\n📹 Video %d/%d remaining in queue", video.Title, s.queue.Len()+1, maxQueueSize)
	keyboard := NewApprovalKeyboard(callbackApprove, callbackReject)

	_, err = s.client.SendVideo(chatID, video.VideoPath, caption, keyboard)
	if err != nil {
		slog.Error("Failed to send video", "error", err)
		s.pendingMu.Lock()
		s.pendingVideo = nil
		s.pendingMu.Unlock()
		_ = s.queue.Add(*video)
		return
	}

	slog.Info("Video sent for review", "title", video.Title, "chat_id", chatID)
}

func (s *ApprovalService) notifyQueueStatus() {
	s.reviewersMu.RLock()
	defer s.reviewersMu.RUnlock()

	count := s.queue.Len()
	msg := fmt.Sprintf("📹 New video queued! (%d/%d in queue)\n\nType /review to review the next video.", count, maxQueueSize)
	for _, reviewer := range s.reviewers {
		_ = s.client.SendMessage(reviewer.ChatID, msg)
	}
}

func (s *ApprovalService) pollCommands() {
	defer s.pollWg.Done()
	slog.Info("Telegram bot started, listening for commands...")

	for {
		select {
		case <-s.stopPoll:
			return
		default:
		}

		updates, err := s.client.GetUpdates(s.pollOffset)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		for _, update := range updates {
			s.pollOffset = update.UpdateID + 1
			s.handleUpdate(update)
		}
	}
}

func (s *ApprovalService) handleUpdate(update Update) {
	if update.CallbackQuery != nil {
		s.handleCallbackQuery(update.CallbackQuery)
		return
	}

	if update.Message == nil || update.Message.Text == "" {
		return
	}

	text := strings.TrimSpace(update.Message.Text)
	chat := update.Message.Chat
	user := update.Message.From

	switch {
	case strings.HasPrefix(text, "/review"):
		s.handleReviewCommand(chat, user)
	case strings.HasPrefix(text, "/queue"):
		s.handleQueueCommand(chat)
	case strings.HasPrefix(text, "/stop"):
		s.handleStopCommand(chat, user)
	case strings.HasPrefix(text, "/start"):
		s.handleStartCommand(chat)
	}
}

func (s *ApprovalService) handleStartCommand(chat *Chat) {
	msg := `Welcome to Craftstory! 🎬

Commands:
/review - Review the next video in queue
/queue - Show queue status
/stop - Stop receiving notifications`
	_ = s.client.SendMessage(chat.ID, msg)
}

func (s *ApprovalService) handleReviewCommand(chat *Chat, user *User) {
	s.reviewersMu.Lock()
	if _, exists := s.reviewers[chat.ID]; !exists {
		reviewer := Reviewer{
			ChatID:   chat.ID,
			UserName: user.UserName,
			Name:     user.FirstName,
		}
		s.reviewers[chat.ID] = reviewer
		s.saveReviewers()
		slog.Info("Reviewer registered", "name", user.FirstName, "chat_id", chat.ID)
		_ = s.client.SendMessage(chat.ID, "✅ You're now registered as a reviewer!")
	}
	s.reviewersMu.Unlock()

	s.pendingMu.Lock()
	if s.pendingVideo != nil {
		s.pendingMu.Unlock()
		_ = s.client.SendMessage(chat.ID, "⏳ A video is already being reviewed. Please wait for the current review to finish.")
		return
	}
	s.pendingMu.Unlock()

	if s.queue.Len() == 0 {
		_ = s.client.SendMessage(chat.ID, "📭 No videos in queue. Generate some videos first!")
		return
	}

	s.sendNextVideoTo(chat.ID)
}

func (s *ApprovalService) handleCallbackQuery(cb *CallbackQuery) {
	s.pendingMu.Lock()
	video := s.pendingVideo
	s.pendingMu.Unlock()

	if video == nil {
		_ = s.client.AnswerCallbackQuery(cb.ID, "No video pending")
		return
	}

	approved := cb.Data == callbackApprove
	responseText := "❌ Rejected"
	if approved {
		responseText = "✅ Approved - uploading..."
	}

	_ = s.client.AnswerCallbackQuery(cb.ID, responseText)

	// Remove buttons
	if cb.Message != nil {
		_ = s.client.EditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, nil)
	}

	reviewer := "unknown"
	if cb.From != nil {
		reviewer = cb.From.FirstName
	}
	slog.Info("Review decision", "approved", approved, "reviewer", reviewer, "title", video.Title)

	result := &ApprovalResult{
		Approved:   approved,
		ReviewerID: cb.From.ID,
	}

	// Clear pending video
	s.pendingMu.Lock()
	s.pendingVideo = nil
	s.pendingMu.Unlock()

	// Send result to waiting channel
	select {
	case s.resultChan <- result:
	default:
	}

	// Notify about remaining queue
	remaining := s.queue.Len()
	if remaining > 0 {
		msg := fmt.Sprintf("📹 %d video(s) remaining in queue. Type /review to continue.", remaining)
		_ = s.client.SendMessage(cb.Message.Chat.ID, msg)
	}
}

func (s *ApprovalService) handleQueueCommand(chat *Chat) {
	videos := s.queue.List()
	if len(videos) == 0 {
		_ = s.client.SendMessage(chat.ID, "📭 Queue is empty.")
		return
	}

	msg := fmt.Sprintf("📹 *Queue Status* (%d/%d)\n\n", len(videos), maxQueueSize)
	for i, v := range videos {
		age := time.Since(v.AddedAt).Round(time.Minute)
		msg += fmt.Sprintf("%d. %s (%v ago)\n", i+1, v.Title, age)
	}
	msg += "\nType /review to review the next video."
	_ = s.client.SendMessage(chat.ID, msg)
}

func (s *ApprovalService) handleStopCommand(chat *Chat, user *User) {
	s.reviewersMu.Lock()
	delete(s.reviewers, chat.ID)
	s.reviewersMu.Unlock()
	s.saveReviewers()

	slog.Info("Reviewer unregistered", "name", user.FirstName, "chat_id", chat.ID)
	_ = s.client.SendMessage(chat.ID, "👋 You've been removed from reviewers.")
}

func (s *ApprovalService) HasReviewers() bool {
	s.reviewersMu.RLock()
	defer s.reviewersMu.RUnlock()
	return len(s.reviewers) > 0
}

func (s *ApprovalService) WaitForResult(ctx context.Context) (*ApprovalResult, *QueuedVideo, error) {
	s.pendingMu.Lock()
	video := s.pendingVideo
	s.pendingMu.Unlock()

	if video == nil {
		return nil, nil, fmt.Errorf("no video pending review")
	}

	select {
	case result := <-s.resultChan:
		return result, video, nil
	case <-ctx.Done():
		return nil, video, ctx.Err()
	}
}

func (s *ApprovalService) GetPendingVideo() *QueuedVideo {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	return s.pendingVideo
}

func (s *ApprovalService) RequestApproval(ctx context.Context, request ApprovalRequest) (*ApprovalResult, error) {
	video := QueuedVideo{
		VideoPath: request.VideoPath,
		Title:     request.Title,
		Script:    request.Script,
	}

	if err := s.QueueVideo(video); err != nil {
		return nil, err
	}

	return &ApprovalResult{Approved: false, Message: "queued"}, nil
}

func (s *ApprovalService) NotifyUploadComplete(title, videoURL string) {
	s.reviewersMu.RLock()
	defer s.reviewersMu.RUnlock()

	msg := fmt.Sprintf("✅ *%s* uploaded!\n\n%s", title, videoURL)
	for _, reviewer := range s.reviewers {
		_ = s.client.SendMessage(reviewer.ChatID, msg)
	}
}

func (s *ApprovalService) NotifyUploadFailed(title string, err error) {
	s.reviewersMu.RLock()
	defer s.reviewersMu.RUnlock()

	msg := fmt.Sprintf("❌ Failed to upload *%s*\n\n%s", title, err.Error())
	for _, reviewer := range s.reviewers {
		_ = s.client.SendMessage(reviewer.ChatID, msg)
	}
}

func (s *ApprovalService) loadReviewers() {
	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		return
	}

	var reviewers []Reviewer
	if err := json.Unmarshal(data, &reviewers); err != nil {
		return
	}

	s.reviewersMu.Lock()
	defer s.reviewersMu.Unlock()
	for _, r := range reviewers {
		s.reviewers[r.ChatID] = r
	}
	slog.Info("Loaded reviewers", "count", len(s.reviewers))
}

func (s *ApprovalService) saveReviewers() {
	s.reviewersMu.RLock()
	reviewers := make([]Reviewer, 0, len(s.reviewers))
	for _, r := range s.reviewers {
		reviewers = append(reviewers, r)
	}
	s.reviewersMu.RUnlock()

	data, err := json.MarshalIndent(reviewers, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(filepath.Dir(s.dataFile), 0755)
	_ = os.WriteFile(s.dataFile, data, 0644)
}
