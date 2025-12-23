package app

import (
	"craftstory/internal/imagesearch"
	"craftstory/internal/llm"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/telegram"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/internal/visuals"
	"craftstory/pkg/config"
)

type Service struct {
	cfg         *config.Config
	llm         llm.Client
	tts         tts.Provider
	uploader    uploader.Uploader
	assembler   *video.Assembler
	storage     *storage.LocalStorage
	reddit      *reddit.Client
	imageSearch *imagesearch.Client
	fetcher     *visuals.Fetcher
	approval    *telegram.ApprovalService
}

func NewService(
	cfg *config.Config,
	llmClient llm.Client,
	ttsProvider tts.Provider,
	up uploader.Uploader,
	assembler *video.Assembler,
	storage *storage.LocalStorage,
	reddit *reddit.Client,
	imageSearch *imagesearch.Client,
	approval *telegram.ApprovalService,
) *Service {
	var fetcher *visuals.Fetcher
	if imageSearch != nil {
		fetcher = visuals.NewFetcher(imageSearch, visuals.Config{
			DisplayTime: cfg.Visuals.DisplayTime,
			ImageWidth:  cfg.Visuals.ImageWidth,
			ImageHeight: cfg.Visuals.ImageHeight,
			MinGap:      cfg.Visuals.MinGap,
		})
	}

	return &Service{
		cfg:         cfg,
		llm:         llmClient,
		tts:         ttsProvider,
		uploader:    up,
		assembler:   assembler,
		storage:     storage,
		reddit:      reddit,
		imageSearch: imageSearch,
		fetcher:     fetcher,
		approval:    approval,
	}
}

func (s *Service) Config() *config.Config {
	return s.cfg
}

func (s *Service) LLM() llm.Client {
	return s.llm
}

func (s *Service) TTS() tts.Provider {
	return s.tts
}

func (s *Service) Uploader() uploader.Uploader {
	return s.uploader
}

func (s *Service) Assembler() *video.Assembler {
	return s.assembler
}

func (s *Service) Storage() *storage.LocalStorage {
	return s.storage
}

func (s *Service) Reddit() *reddit.Client {
	return s.reddit
}

func (s *Service) ImageSearch() *imagesearch.Client {
	return s.imageSearch
}

func (s *Service) Fetcher() *visuals.Fetcher {
	return s.fetcher
}

func (s *Service) Approval() *telegram.ApprovalService {
	return s.approval
}
