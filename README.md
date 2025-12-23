# Craftstory

YouTube Shorts automation with AI voiceover.

## Setup

```bash
sudo apt install ffmpeg  # or: brew install ffmpeg
curl https://mise.run | sh && ~/.local/bin/mise install
gcloud auth application-default login
task setup
```

Add your API keys:
```bash
echo -n 'YOUR_GROQ_KEY' | gcloud secrets versions add groq-api-key --data-file=-
echo -n 'YOUR_BOT_TOKEN' | gcloud secrets versions add telegram-bot-token --data-file=-  # optional
```

## Usage

```bash
task run -- generate -topic "weird history fact"
task run -- generate -topic "space theory" -upload
task run -- generate -topic "golang tips" -approve
task run -- generate -reddit
task run -- bot
```
