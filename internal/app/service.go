package app

import (
	"craftstory/internal/deepseek"
	"craftstory/internal/imagesearch"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/pkg/config"
)

type Service struct {
	cfg         *config.Config
	deepseek    *deepseek.Client
	tts         tts.Provider
	uploader    uploader.Uploader
	assembler   *video.Assembler
	storage     *storage.LocalStorage
	reddit      *reddit.Client
	imageSearch *imagesearch.Client
}

func NewService(
	cfg *config.Config,
	deepseek *deepseek.Client,
	ttsProvider tts.Provider,
	up uploader.Uploader,
	assembler *video.Assembler,
	storage *storage.LocalStorage,
	reddit *reddit.Client,
	imageSearch *imagesearch.Client,
) *Service {
	return &Service{
		cfg:         cfg,
		deepseek:    deepseek,
		tts:         ttsProvider,
		uploader:    up,
		assembler:   assembler,
		storage:     storage,
		reddit:      reddit,
		imageSearch: imageSearch,
	}
}

func (s *Service) Config() *config.Config {
	return s.cfg
}

func (s *Service) DeepSeek() *deepseek.Client {
	return s.deepseek
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
