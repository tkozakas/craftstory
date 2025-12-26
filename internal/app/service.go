package app

import (
	"craftstory/internal/llm"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/telegram"
	"craftstory/internal/tenor"
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
	gifSearch   *tenor.Client
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
	GIFSearch   *tenor.Client
	Approval    *telegram.ApprovalService
}

func NewService(opts ServiceOptions) *Service {
	var fetcher *visuals.Fetcher
	if opts.ImageSearch != nil || opts.GIFSearch != nil {
		fetcher = visuals.NewFetcher(opts.ImageSearch, opts.GIFSearch, visuals.Config{
			MaxDisplayTime: opts.Config.Visuals.MaxDisplayTime,
			ImageWidth:     opts.Config.Visuals.ImageWidth,
			ImageHeight:    opts.Config.Visuals.ImageHeight,
			MinGap:         opts.Config.Visuals.MinGap,
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
		gifSearch:   opts.GIFSearch,
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

func (s *Service) GIFSearch() *tenor.Client {
	return s.gifSearch
}

func (s *Service) Fetcher() *visuals.Fetcher {
	return s.fetcher
}

func (s *Service) Approval() *telegram.ApprovalService {
	return s.approval
}
