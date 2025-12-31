# Manual Setup

If you prefer to configure API keys manually instead of using the interactive wizard.

## Required Keys

### GROQ API Key
1. Go to [console.groq.com/keys](https://console.groq.com/keys)
2. Create an API key
3. Add to `.env`: `GROQ_API_KEY=gsk_...`

### ElevenLabs API Key
1. Go to [elevenlabs.io/app/settings/api-keys](https://elevenlabs.io/app/settings/api-keys)
2. Create an API key
3. Add to `.env`: `ELEVENLABS_API_KEY=sk_...`

## Optional Keys

### YouTube Upload
For uploading videos to YouTube:

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create or select a project
3. Enable the YouTube Data API v3
4. Go to **Credentials** > **Create Credentials** > **OAuth client ID**
5. Choose **Desktop app**
6. Add to `.env`:
   ```
   YOUTUBE_CLIENT_ID=...
   YOUTUBE_CLIENT_SECRET=...
   ```
7. Run `craftstory auth youtube` to complete OAuth flow

### Google Image Search
For fetching images in videos:

1. Go to [Google Cloud Console Credentials](https://console.cloud.google.com/apis/credentials)
2. Create an API Key
3. Go to [Programmable Search Engine](https://programmablesearchengine.google.com)
4. Create a search engine (enable "Search the entire web")
5. Copy the Search Engine ID
6. Add to `.env`:
   ```
   GOOGLE_SEARCH_API_KEY=...
   GOOGLE_SEARCH_ENGINE_ID=...
   ```

### Tenor GIFs
For animated GIF overlays:

1. Go to [Tenor API Quickstart](https://developers.google.com/tenor/guides/quickstart)
2. Create a project and get an API key
3. Add to `.env`: `TENOR_API_KEY=...`

### Telegram Bot
For video approval workflow:

1. Message [@BotFather](https://t.me/BotFather) on Telegram
2. Create a new bot with `/newbot`
3. Copy the token
4. Add to `.env`: `TELEGRAM_BOT_TOKEN=123456:ABC...`

## Example .env

```bash
GROQ_API_KEY=gsk_...
ELEVENLABS_API_KEY=sk_...

# YouTube (optional)
YOUTUBE_CLIENT_ID=...
YOUTUBE_CLIENT_SECRET=...

# Image search (optional)
GOOGLE_SEARCH_API_KEY=...
GOOGLE_SEARCH_ENGINE_ID=...

# Extras (optional)
TENOR_API_KEY=...
TELEGRAM_BOT_TOKEN=...
```

## Asset Directories

Create these directories and add your content:

```
assets/
  backgrounds/   # Background videos (mp4)
  music/         # Background music (mp3, optional)
output/          # Generated videos
```
