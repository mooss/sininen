#!/usr/bin/env bash
set -Ee

#################
# Preconditions #
#################
usage="usage: $0 channel-name

channel-name is the last part of a YouTube channel's URL. For the channel Historia Civilis (https://www.youtube.com/c/HistoriaCivilis), it is \`HistoriaCivilis\`.
"
if [ $# -ne 1 ]; then
    >&2 echo "$usage"
    exit 1
fi
channel_name="$1"

if ! which youtube-dl >/dev/null; then
    >&2 echo "youtube-dl is a required dependency, see https://ytdl-org.github.io/youtube-dl/download.html to install it."
    exit 2
fi

#################
# Script proper #
#################
destfolder="subtitles/$channel_name"
mkdir -p "$destfolder"
youtube-dl --skip-download --all-subs "https://www.youtube.com/c/$channel_name/videos" -o "$destfolder/%(id)s.%(ext)s"
