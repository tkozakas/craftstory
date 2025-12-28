package app

import (
	"craftstory/internal/content/reddit"
	"craftstory/internal/distribution"
	"craftstory/internal/distribution/telegram"
	"craftstory/internal/llm"
	"craftstory/internal/search"
	"craftstory/internal/speech"
	"craftstory/internal/storage"
	"craftstory/internal/video"
	"craftstory/pkg/config"
)

type Service struct {
	cfg       *config.Config
	llm       llm.Client
	tts       speech.Provider
	uploader  distribution.Uploader
	assembler *video.Assembler
	storage   *storage.LocalStorage
	reddit    *reddit.Client
	fetcher   *search.Fetcher
	approval  *telegram.ApprovalService
}

type ServiceOptions struct {
	Config    *config.Config
	LLM       llm.Client
	TTS       speech.Provider
	Uploader  distribution.Uploader
	Assembler *video.Assembler
	Storage   *storage.LocalStorage
	Reddit    *reddit.Client
	Fetcher   *search.Fetcher
	Approval  *telegram.ApprovalService
}

func NewService(opts ServiceOptions) *Service {
	return &Service{
		cfg:       opts.Config,
		llm:       opts.LLM,
		tts:       opts.TTS,
		uploader:  opts.Uploader,
		assembler: opts.Assembler,
		storage:   opts.Storage,
		reddit:    opts.Reddit,
		fetcher:   opts.Fetcher,
		approval:  opts.Approval,
	}
}

func (s *Service) Approval() *telegram.ApprovalService {
	return s.approval
}
