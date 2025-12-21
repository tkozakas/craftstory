# Craftstory

YouTube Shorts automation with AI voiceover.

## Setup

```bash
sudo apt install ffmpeg  # or: brew install ffmpeg
curl https://mise.run | sh && ~/.local/bin/mise install
task run -- setup
```

## Usage

```bash
task run -- generate -topic "weird history fact"
task run -- generate -topic "space theory" -upload
```
