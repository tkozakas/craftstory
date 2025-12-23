package app

import (
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
	imageSearch *visuals.SearchClient
	fetcher     *visuals.Fetcher
	approval    *telegram.ApprovalService
}

type ServiceOptions struct {
	Config      *config.Config
	LLM         llm.Client
	TTS         tts.Provider
	Uploader    uploader.Uploader
	Assembler   *video.Assembler
	Storage     *storage.LocalStorage
	Reddit      *reddit.Client
	ImageSearch *visuals.SearchClient
	Approval    *telegram.ApprovalService
}

func NewService(opts ServiceOptions) *Service {
	var fetcher *visuals.Fetcher
	if opts.ImageSearch != nil {
		fetcher = visuals.NewFetcher(opts.ImageSearch, visuals.Config{
			DisplayTime: opts.Config.Visuals.DisplayTime,
			ImageWidth:  opts.Config.Visuals.ImageWidth,
			ImageHeight: opts.Config.Visuals.ImageHeight,
			MinGap:      opts.Config.Visuals.MinGap,
		})
	}

	return &Service{
		cfg:         opts.Config,
		llm:         opts.LLM,
		tts:         opts.TTS,
		uploader:    opts.Uploader,
		assembler:   opts.Assembler,
		storage:     opts.Storage,
		reddit:      opts.Reddit,
		imageSearch: opts.ImageSearch,
		fetcher:     fetcher,
		approval:    opts.Approval,
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

func (s *Service) ImageSearch() *visuals.SearchClient {
	return s.imageSearch
}

func (s *Service) Fetcher() *visuals.Fetcher {
	return s.fetcher
}

func (s *Service) Approval() *telegram.ApprovalService {
	return s.approval
}
