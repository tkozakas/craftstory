package app

import (
	"craftstory/internal/content/reddit"
	"craftstory/internal/distribution"
	"craftstory/internal/distribution/telegram"
	"craftstory/internal/llm"
	"craftstory/internal/search"
	"craftstory/internal/search/google"
	"craftstory/internal/search/tenor"
	"craftstory/internal/speech"
	"craftstory/internal/storage"
	"craftstory/internal/video"
	"craftstory/pkg/config"
)

type Service struct {
	cfg         *config.Config
	llm         llm.Client
	tts         speech.Provider
	uploader    distribution.Uploader
	assembler   *video.Assembler
	storage     *storage.LocalStorage
	reddit      *reddit.Client
	imageSearch *google.Client
	gifSearch   *tenor.Client
	fetcher     *search.Fetcher
	approval    *telegram.ApprovalService
}

type ServiceOptions struct {
	Config      *config.Config
	LLM         llm.Client
	TTS         speech.Provider
	Uploader    distribution.Uploader
	Assembler   *video.Assembler
	Storage     *storage.LocalStorage
	Reddit      *reddit.Client
	ImageSearch *google.Client
	GIFSearch   *tenor.Client
	Approval    *telegram.ApprovalService
}

func NewService(opts ServiceOptions) *Service {
	var fetcher *search.Fetcher
	if opts.ImageSearch != nil || opts.GIFSearch != nil {
		var gifSearcher search.GIFSearcher
		if opts.GIFSearch != nil {
			gifSearcher = opts.GIFSearch
		}
		fetcher = search.NewFetcher(opts.ImageSearch, gifSearcher, search.FetcherConfig{
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

func (s *Service) Approval() *telegram.ApprovalService {
	return s.approval
}
