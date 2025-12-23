# Craftstory

YouTube Shorts automation with AI voiceover.

## Setup

```bash
sudo apt install ffmpeg  # or: brew install ffmpeg
task setup
```

### Manual Setup

Create a `.env` file instead:

```bash
GROQ_API_KEY=gsk_...
ELEVENLABS_API_KEY=sk_...
TELEGRAM_BOT_TOKEN=123456:ABC...  # optional
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
