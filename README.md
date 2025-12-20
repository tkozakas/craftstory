# Craftstory

YouTube Shorts automation tool that generates videos from Reddit stories with AI voiceover.

## Setup

### 1. Install mise

```bash
curl https://mise.run | sh

# Add to shell
echo 'eval "$(~/.local/bin/mise activate zsh)"' >> ~/.zshrc

# Reload shell
source ~/.zshrc
```

### 2. Install tools

```bash
mise install
```

### 3. Install FFmpeg

```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# Mac
brew install ffmpeg
```

### 4. Create `.env`

```env
DEEPSEEK_API_KEY=
ELEVENLABS_API_KEY=
YOUTUBE_CLIENT_ID=
YOUTUBE_CLIENT_SECRET=
GCS_BUCKET=                        # optional
GOOGLE_APPLICATION_CREDENTIALS=    # optional
```

### 5. Add background videos

Put vertical videos (9:16) in `./assets/backgrounds/`

## Usage

```bash
# Authenticate YouTube (first time only)
task run -- auth

# Generate video
task run -- generate -topic "weird history fact"

# Generate and upload
task run -- generate -topic "space theory" -upload
```
