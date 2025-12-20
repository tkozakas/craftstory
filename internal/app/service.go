package app

import (
	"craftstory/internal/deepseek"
	"craftstory/internal/elevenlabs"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/pkg/config"
)

type Service struct {
	cfg        *config.Config
	deepseek   *deepseek.Client
	elevenlabs *elevenlabs.Client
	uploader   uploader.Uploader
	assembler  *video.Assembler
	storage    *storage.LocalStorage
	reddit     *reddit.Client
}

func NewService(
	cfg *config.Config,
	deepseek *deepseek.Client,
	elevenlabs *elevenlabs.Client,
	up uploader.Uploader,
	assembler *video.Assembler,
	storage *storage.LocalStorage,
	reddit *reddit.Client,
) *Service {
	return &Service{
		cfg:        cfg,
		deepseek:   deepseek,
		elevenlabs: elevenlabs,
		uploader:   up,
		assembler:  assembler,
		storage:    storage,
		reddit:     reddit,
	}
}

func (s *Service) Config() *config.Config {
	return s.cfg
}

func (s *Service) DeepSeek() *deepseek.Client {
	return s.deepseek
}

func (s *Service) ElevenLabs() *elevenlabs.Client {
	return s.elevenlabs
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
