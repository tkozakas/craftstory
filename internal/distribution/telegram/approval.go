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
)

type ApprovalService struct {
	client          *Client
	defaultChatID   int64
	previewDuration float64
	reviewers       map[int64]Reviewer
	reviewersMu     sync.RWMutex
	dataFile        string
	pollOffset      int
	stopPoll        chan struct{}
	pollWg          sync.WaitGroup
	queue           *VideoQueue
	pendingVideo    *QueuedVideo
	pendingMu       sync.Mutex
	resultChan      chan *ApprovalResult
	generationQueue *GenerationQueue
	genRequestChan  chan GenerationRequest
}

type ApprovalRequest struct {
	VideoPath   string
	PreviewPath string
	Title       string
	Script      string
	Tags        []string
}

type ApprovalResult struct {
	Approved   bool
	Message    string
	ReviewerID int64
}

func NewApprovalService(client *Client, dataDir string, defaultChatID int64, previewDuration float64) *ApprovalService {
	if previewDuration <= 0 {
		previewDuration = 30
	}
	svc := &ApprovalService{
		client:          client,
		defaultChatID:   defaultChatID,
		previewDuration: previewDuration,
		reviewers:       make(map[int64]Reviewer),
		dataFile:        filepath.Join(dataDir, "reviewers.json"),
		stopPoll:        make(chan struct{}),
		queue:           NewVideoQueue(dataDir),
		resultChan:      make(chan *ApprovalResult, 1),
		generationQueue: NewGenerationQueue(dataDir),
		genRequestChan:  make(chan GenerationRequest, maxGenerationQueueSize),
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

func (s *ApprovalService) GenerationQueue() *GenerationQueue {
	return s.generationQueue
}

func (s *ApprovalService) QueueVideo(video QueuedVideo) error {
	if err := s.queue.Add(video); err != nil {
		return err
	}
	slog.Info("Video queued for review", "title", video.Title, "queue_size", s.queue.Len(), "has_preview", video.PreviewPath != "")

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
		slog.Debug("Skipping send: video already pending review", "pending_title", s.pendingVideo.Title)
		return
	}

	video, err := s.queue.Pop()
	if err != nil {
		s.pendingMu.Unlock()
		slog.Debug("Skipping send: queue empty")
		return
	}

	s.pendingVideo = video
	s.pendingMu.Unlock()

	videoToSend := video.VideoPath
	if video.PreviewPath != "" {
		videoToSend = video.PreviewPath
	}
	slog.Debug("Sending video for review", "title", video.Title, "path", videoToSend, "has_preview", video.PreviewPath != "")

	caption := fmt.Sprintf("*%s*\n\n📹 Video %d/%d remaining in queue", video.Title, s.queue.Len()+1, maxQueueSize)
	if video.PreviewPath != "" {
		caption += fmt.Sprintf("\n\n⏱ Preview (%.0fs)", s.previewDuration)
	}
	keyboard := NewApprovalKeyboard(callbackApprove, callbackReject)

	resp, err := s.client.SendVideo(chatID, videoToSend, caption, keyboard)
	if err != nil {
		slog.Error("Failed to send video", "error", err)
		s.pendingMu.Lock()
		s.pendingVideo = nil
		s.pendingMu.Unlock()
		_ = s.queue.Add(*video)
		return
	}

	s.pendingMu.Lock()
	s.pendingVideo.MessageID = resp.MessageID
	s.pendingVideo.ChatID = chatID
	s.pendingMu.Unlock()

	slog.Info("Video sent for review", "title", video.Title, "chat_id", chatID, "message_id", resp.MessageID)
}

func (s *ApprovalService) notifyQueueStatus() {
	s.reviewersMu.RLock()
	defer s.reviewersMu.RUnlock()

	count := s.queue.Len()
	msg := fmt.Sprintf("📹 New video queued (%d/%d in queue)\n\nType /review to review.", count, maxQueueSize)
	for _, reviewer := range s.reviewers {
		_ = s.client.SendMessage(reviewer.ChatID, msg)
	}
}

func (s *ApprovalService) pollCommands() {
	defer s.pollWg.Done()
	slog.Info("Telegram bot started")

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
	case strings.HasPrefix(text, "/generate"):
		s.handleGenerateCommand(chat, text)
	case strings.HasPrefix(text, "/review"):
		s.handleReviewCommand(chat, user)
	case strings.HasPrefix(text, "/queue"):
		s.handleQueueCommand(chat)
	case strings.HasPrefix(text, "/status"):
		s.handleStatusCommand(chat)
	case strings.HasPrefix(text, "/stop"):
		s.handleStopCommand(chat, user)
	case strings.HasPrefix(text, "/help"), strings.HasPrefix(text, "/start"):
		s.handleHelpCommand(chat)
	}
}

func (s *ApprovalService) handleHelpCommand(chat *Chat) {
	msg := `*Craftstory Bot*

*Commands:*
/generate [topic] - Generate video (Reddit topic if empty)
/status - Generation queue status
/help - Show this message

*Admin:*
/review - Review next video
/queue - Approval queue status
/stop - Unsubscribe from notifications`
	_ = s.client.SendMessage(chat.ID, msg)
}

func (s *ApprovalService) handleGenerateCommand(chat *Chat, text string) {
	topic := strings.TrimSpace(strings.TrimPrefix(text, "/generate"))
	fromReddit := topic == ""

	if s.generationQueue.IsFull() {
		_ = s.client.SendMessage(chat.ID, "Queue full. Please wait.")
		return
	}

	request := GenerationRequest{
		Topic:      topic,
		ChatID:     chat.ID,
		FromReddit: fromReddit,
	}

	if err := s.generationQueue.Add(request); err != nil {
		_ = s.client.SendMessage(chat.ID, fmt.Sprintf("Failed to queue: %s", err.Error()))
		return
	}

	position := s.generationQueue.Len()
	var msg string
	if fromReddit {
		msg = fmt.Sprintf("Queued generation from Reddit\nPosition: %d", position)
	} else {
		msg = fmt.Sprintf("Queued generation\nTopic: %s\nPosition: %d", topic, position)
	}

	if s.generationQueue.IsGenerating() {
		msg += "\n\nGenerating another video..."
	}

	_ = s.client.SendMessage(chat.ID, msg)

	select {
	case s.genRequestChan <- request:
	default:
	}
}

func (s *ApprovalService) handleStatusCommand(chat *Chat) {
	requests := s.generationQueue.List()

	if len(requests) == 0 {
		_ = s.client.SendMessage(chat.ID, "Generation queue empty.\n\nUse /generate to create a video.")
		return
	}

	msg := fmt.Sprintf("*Generation Queue* (%d/%d)\n\n", len(requests), maxGenerationQueueSize)
	for i, req := range requests {
		status := "⏳"
		if req.Status == "generating" {
			status = "🔄"
		}
		topic := req.Topic
		if req.FromReddit {
			topic = "(Reddit)"
		}
		age := time.Since(req.AddedAt).Round(time.Second)
		msg += fmt.Sprintf("%s %d. %s (%v ago)\n", status, i+1, topic, age)
	}
	_ = s.client.SendMessage(chat.ID, msg)
}

func (s *ApprovalService) handleReviewCommand(chat *Chat, user *User) {
	if s.defaultChatID != 0 && chat.ID != s.defaultChatID {
		_ = s.client.SendMessage(chat.ID, "Review commands only available in admin chat.")
		return
	}

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
		_ = s.client.SendMessage(chat.ID, "Registered as reviewer.")
	}
	s.reviewersMu.Unlock()

	s.pendingMu.Lock()
	if s.pendingVideo != nil {
		s.pendingMu.Unlock()
		_ = s.client.SendMessage(chat.ID, "A video is being reviewed. Please wait.")
		return
	}
	s.pendingMu.Unlock()

	if s.queue.Len() == 0 {
		_ = s.client.SendMessage(chat.ID, "No videos in queue.")
		return
	}

	s.sendNextVideoTo(chat.ID)
}

func (s *ApprovalService) handleCallbackQuery(cb *CallbackQuery) {
	slog.Debug("Callback received", "data", cb.Data, "from", cb.From.ID)

	if cb.Message != nil && s.defaultChatID != 0 && cb.Message.Chat.ID != s.defaultChatID {
		slog.Debug("Callback rejected: wrong chat", "chat_id", cb.Message.Chat.ID, "expected", s.defaultChatID)
		_ = s.client.AnswerCallbackQuery(cb.ID, "Not authorized")
		return
	}

	s.pendingMu.Lock()
	video := s.pendingVideo
	s.pendingMu.Unlock()

	if video == nil {
		slog.Debug("Callback rejected: no pending video")
		_ = s.client.AnswerCallbackQuery(cb.ID, "No video pending")
		return
	}

	approved := cb.Data == callbackApprove
	slog.Info("Video decision", "approved", approved, "title", video.Title)

	_ = s.client.AnswerCallbackQuery(cb.ID, "")

	if cb.Message != nil {
		_ = s.client.EditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, nil)

		if approved {
			caption := fmt.Sprintf("*%s*\n\n⏳ Uploading...", video.Title)
			_ = s.client.EditMessageCaption(cb.Message.Chat.ID, cb.Message.MessageID, caption)
		} else {
			caption := fmt.Sprintf("*%s*\n\n❌ Rejected", video.Title)
			_ = s.client.EditMessageCaption(cb.Message.Chat.ID, cb.Message.MessageID, caption)
		}
	}

	result := &ApprovalResult{
		Approved:   approved,
		ReviewerID: cb.From.ID,
	}

	s.resultChan <- result

	remaining := s.queue.Len()
	if remaining > 0 && cb.Message != nil {
		msg := fmt.Sprintf("%d video(s) remaining. Type /review to continue.", remaining)
		_ = s.client.SendMessage(cb.Message.Chat.ID, msg)
	}
}

func (s *ApprovalService) handleQueueCommand(chat *Chat) {
	videos := s.queue.List()
	if len(videos) == 0 {
		_ = s.client.SendMessage(chat.ID, "Approval queue empty.")
		return
	}

	msg := fmt.Sprintf("*Approval Queue* (%d/%d)\n\n", len(videos), maxQueueSize)
	for i, v := range videos {
		age := time.Since(v.AddedAt).Round(time.Minute)
		msg += fmt.Sprintf("%d. %s (%v ago)\n", i+1, v.Title, age)
	}
	msg += "\nType /review to review."
	_ = s.client.SendMessage(chat.ID, msg)
}

func (s *ApprovalService) handleStopCommand(chat *Chat, user *User) {
	s.reviewersMu.Lock()
	delete(s.reviewers, chat.ID)
	s.reviewersMu.Unlock()
	s.saveReviewers()

	slog.Info("Reviewer unregistered", "name", user.FirstName, "chat_id", chat.ID)
	_ = s.client.SendMessage(chat.ID, "Removed from reviewers.")
}

func (s *ApprovalService) WaitForResult(ctx context.Context) (*ApprovalResult, *QueuedVideo, error) {
	select {
	case result := <-s.resultChan:
		s.pendingMu.Lock()
		video := s.pendingVideo
		s.pendingVideo = nil
		s.pendingMu.Unlock()
		return result, video, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (s *ApprovalService) RequestApproval(ctx context.Context, request ApprovalRequest) (*ApprovalResult, error) {
	video := QueuedVideo{
		VideoPath:   request.VideoPath,
		PreviewPath: request.PreviewPath,
		Title:       request.Title,
		Script:      request.Script,
		Tags:        request.Tags,
	}

	if err := s.QueueVideo(video); err != nil {
		return nil, err
	}

	return &ApprovalResult{Approved: false, Message: "queued"}, nil
}

func (s *ApprovalService) NotifyUploadComplete(title, videoURL string, video *QueuedVideo) {
	caption := fmt.Sprintf("*%s*\n\n✅ Uploaded\n%s", title, videoURL)
	fallback := fmt.Sprintf("*%s* uploaded\n\n%s", title, videoURL)
	s.notifyResult(video, caption, fallback)
}

func (s *ApprovalService) NotifyUploadFailed(title string, err error, video *QueuedVideo) {
	caption := fmt.Sprintf("*%s*\n\n❌ Upload failed: %s", title, err.Error())
	fallback := fmt.Sprintf("Failed to upload *%s*\n\n%s", title, err.Error())
	s.notifyResult(video, caption, fallback)
}

func (s *ApprovalService) notifyResult(video *QueuedVideo, caption, fallbackMsg string) {
	if video != nil && video.MessageID != 0 && video.ChatID != 0 {
		_ = s.client.EditMessageCaption(video.ChatID, video.MessageID, caption)
		return
	}

	s.reviewersMu.RLock()
	defer s.reviewersMu.RUnlock()

	for _, reviewer := range s.reviewers {
		_ = s.client.SendMessage(reviewer.ChatID, fallbackMsg)
	}
}

func (s *ApprovalService) WaitForGenerationRequest(ctx context.Context) (*GenerationRequest, error) {
	req, err := s.generationQueue.Pop()
	if err == nil {
		return req, nil
	}

	select {
	case <-s.genRequestChan:
		return s.generationQueue.Pop()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *ApprovalService) NotifyGenerating(chatID int64, topic string) {
	var msg string
	if topic == "" {
		msg = "Generating video from Reddit...\n\nThis may take a few minutes."
	} else {
		msg = fmt.Sprintf("Generating video...\n\nTopic: %s\n\nThis may take a few minutes.", topic)
	}
	_ = s.client.SendMessage(chatID, msg)
}

func (s *ApprovalService) NotifyGenerationComplete(chatID int64, videoPath, previewPath, title, script string, tags []string) {
	caption := fmt.Sprintf("*%s*\n\nGenerated successfully.", title)

	videoToSend := videoPath
	if previewPath != "" {
		videoToSend = previewPath
		caption += fmt.Sprintf("\n\n⏱ Preview (%.0fs)", s.previewDuration)
	}

	_, err := s.client.SendVideo(chatID, videoToSend, caption, nil)
	if err != nil {
		slog.Error("Failed to send video to requester", "chat_id", chatID, "error", err)
	}

	if s.defaultChatID != 0 && chatID != s.defaultChatID {
		video := QueuedVideo{
			VideoPath:   videoPath,
			PreviewPath: previewPath,
			Title:       title,
			Script:      script,
			Tags:        tags,
		}
		if err := s.QueueVideo(video); err != nil {
			slog.Error("Failed to queue video for approval", "error", err)
		}
	}
}

func (s *ApprovalService) NotifyGenerationFailed(chatID int64, errMsg string) {
	msg := fmt.Sprintf("Generation failed\n\n%s", errMsg)
	_ = s.client.SendMessage(chatID, msg)
}

func (s *ApprovalService) CompleteGeneration(chatID int64) {
	s.generationQueue.Complete(chatID)
}

func (s *ApprovalService) FailGeneration(chatID int64) {
	s.generationQueue.Fail(chatID)
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
