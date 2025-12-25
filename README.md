# Craftstory

YouTube Shorts automation with AI voiceover.

## Setup

- [mise](https://mise.jdx.dev)
- ffmpeg

```bash
curl https://mise.run | sh && export PATH="$HOME/.local/bin:$PATH"
sudo apt install ffmpeg
mise install
mise exec -- task setup
```

### Manual Setup

Create a `.env` file instead:

```bash
GROQ_API_KEY=gsk_...
ELEVENLABS_API_KEY=sk_...
TELEGRAM_BOT_TOKEN=123456:ABC...  # optional

# For YouTube uploads
YOUTUBE_CLIENT_ID=...
YOUTUBE_CLIENT_SECRET=...

# For image search in videos
GOOGLE_SEARCH_API_KEY=...
GOOGLE_SEARCH_ENGINE_ID=...
```

## Usage

```bash
task run -- run                        # cron mode: generate, approve via Telegram, repeat
task run -- run -interval 30m          # custom interval
task run -- run -upload                # cron mode: generate and upload directly

task run -- once -topic "weird facts"  # single video
task run -- once -reddit               # single from Reddit
task run -- once -reddit -upload       # single + upload
```
