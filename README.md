# Craftstory

YouTube Shorts automation tool that generates videos from Reddit stories with AI voiceover.

## Setup

- [mise](https://mise.jdx.dev/) (run `mise install` to install tools)
- FFmpeg

Create `.env`:

```env
DEEPSEEK_API_KEY=
ELEVENLABS_API_KEY=
YOUTUBE_CLIENT_ID=
YOUTUBE_CLIENT_SECRET=
GCS_BUCKET=                        # optional
GOOGLE_APPLICATION_CREDENTIALS=    # optional
```

## Usage

```bash
task run -- -topic "weird history fact"
task run -- -topic "space theory" -upload
```

