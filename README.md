# Craftstory

AI-powered YouTube Shorts automation.

## Prerequisites

- [mise](https://mise.jdx.dev) - tool version manager
- [ffmpeg](https://ffmpeg.org) - video processing

## Setup

```bash
mise install
mise exec -- task setup
```

The interactive wizard handles API keys, directories, and OAuth flows.

> Manual setup? See [SETUP.md](SETUP.md)

## Usage

### Single Video

```bash
# Generate from topic
task run -- once --topic "ancient mysteries"

# Generate from Reddit
task run -- once --reddit

# Generate and upload
task run -- once --topic "space facts" --upload
```

### Continuous Mode

```bash
# Generate + approve via Telegram
task run -- run

# Custom interval
task run -- run --interval 30m

# Auto-upload (no approval)
task run -- run --upload
```


