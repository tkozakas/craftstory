# Craftstory

YouTube Shorts automation tool that generates videos from Reddit stories with AI voiceover.

## Features

### Implemented
- [x] AI script generation (DeepSeek)
- [x] Text-to-speech with word-level timestamps (ElevenLabs)
- [x] Centered word-by-word subtitles (ASS format)
- [x] Video assembly with background clips (FFmpeg)
- [x] YouTube upload with OAuth2
- [x] Reddit story fetching
- [x] Multi-voice conversation mode
- [x] Visual overlays from Google Images
- [x] GCS storage support for backgrounds

### Planned
- [ ] TikTok upload support
- [ ] Instagram Reels upload support
- [ ] Background music with auto-ducking
- [ ] Scheduled posting

## Setup

### 1. Install mise

```bash
curl https://mise.run | sh && echo 'eval "$(~/.local/bin/mise activate zsh)"' >> ~/.zshrc && source ~/.zshrc

mise install
```

### 2. Install FFmpeg

```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# Mac
brew install ffmpeg
```

### 3. Create `.env`

```env
DEEPSEEK_API_KEY=
ELEVENLABS_API_KEY=
YOUTUBE_CLIENT_ID=
YOUTUBE_CLIENT_SECRET=
GOOGLE_SEARCH_API_KEY=             # optional, for visual overlays
GOOGLE_SEARCH_ENGINE_ID=           # optional, for visual overlays
GCS_BUCKET=                        # optional
GOOGLE_APPLICATION_CREDENTIALS=    # optional
```

### 4. Add background videos

Put vertical videos (9:16) in `./assets/backgrounds/`

## Usage

```bash
# Generate video
task run -- generate -topic "weird history fact"

# Generate and upload
task run -- generate -topic "space theory" -upload
```

## API Keys

| Service | Get Key |
|---------|---------|
| DeepSeek | https://platform.deepseek.com/api_keys |
| ElevenLabs | https://elevenlabs.io/app/settings/api-keys |
| YouTube | https://console.cloud.google.com/apis/credentials (OAuth 2.0 Client ID) |
| Google Search API | https://console.cloud.google.com/apis/credentials (API Key) |
| Google Search Engine ID | https://programmablesearchengine.google.com/controlpanel/all (Create search engine → Get Search engine ID) |
