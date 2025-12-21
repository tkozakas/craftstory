#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "Usage: $0 <character> <youtube_url> [start_time] [duration]"
    echo ""
    echo "Arguments:"
    echo "  character    Character name (creates folder in assets/characters/)"
    echo "  youtube_url  YouTube video URL"
    echo "  start_time   Start timestamp (optional, e.g., 00:00:30)"
    echo "  duration     Duration in seconds (optional, default: 20)"
    echo ""
    echo "Example:"
    echo "  $0 morgan_freeman 'https://youtube.com/watch?v=xxx' 00:01:30 15"
    exit 1
}

if [[ $# -lt 2 ]]; then
    usage
fi

CHARACTER="$1"
YOUTUBE_URL="$2"
START_TIME="${3:-}"
DURATION="${4:-20}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CHARACTER_DIR="$PROJECT_DIR/assets/characters/$CHARACTER"
TEMP_FILE="$PROJECT_DIR/.cache/temp_voice_$$.wav"
OUTPUT_FILE="$CHARACTER_DIR/voice.wav"

# Check dependencies
for cmd in yt-dlp ffmpeg; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Error: $cmd is required but not installed."
        exit 1
    fi
done

# Create directories
mkdir -p "$CHARACTER_DIR"
mkdir -p "$PROJECT_DIR/.cache"

echo "Downloading audio from YouTube..."
yt-dlp -x --audio-format wav -o "$TEMP_FILE" "$YOUTUBE_URL" --quiet --no-warnings

# Trim audio if start time provided, otherwise take first N seconds
if [[ -n "$START_TIME" ]]; then
    echo "Trimming from $START_TIME for ${DURATION}s..."
    ffmpeg -i "$TEMP_FILE" -ss "$START_TIME" -t "$DURATION" -ar 22050 -ac 1 "$OUTPUT_FILE" -y -loglevel error
else
    echo "Extracting first ${DURATION}s..."
    ffmpeg -i "$TEMP_FILE" -t "$DURATION" -ar 22050 -ac 1 "$OUTPUT_FILE" -y -loglevel error
fi

# Clean up
rm -f "$TEMP_FILE"

# Create character.yaml if it doesn't exist
YAML_FILE="$CHARACTER_DIR/character.yaml"
if [[ ! -f "$YAML_FILE" ]]; then
    echo "Creating character.yaml..."
    cat > "$YAML_FILE" << EOF
name: $CHARACTER
voice_sample: voice.wav
image: character.png
subtitle_color: "#FFFFFF"
EOF
fi

echo ""
echo "Done! Voice sample saved to: $OUTPUT_FILE"
echo "Character config: $YAML_FILE"
echo ""
echo "Tips for best results:"
echo "  - Listen to the sample and re-run with different start_time if needed"
echo "  - Ideal: 10-30s of clear speech, no music/background noise"
echo "  - Update subtitle_color in character.yaml if desired"
