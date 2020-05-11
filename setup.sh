#!/bin/bash

if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root" 
   exit 1
fi

touch config.json
chown logname config.json

#install stuff for ffpeg
apt -y install autoconf automake build-essential libass-dev libfreetype6-dev libsdl1.2-dev libtheora-dev libtool libva-dev libvdpau-dev libvorbis-dev libxcb1-dev libxcb-shm0-dev libxcb-xfixes0-dev pkg-config texi2html zlib1g-dev libavdevice-dev libavfilter-dev libswscale-dev libavcodec-dev libavformat-dev libswresample-dev libavutil-dev yasm
#prep for ffmpeg
export FFMPEG_ROOT=$HOME/ffmpeg
export CGO_LDFLAGS="-L$FFMPEG_ROOT/lib/ -lavcodec -lavformat -lavutil -lswscale -lswresample -lavdevice -lavfilter"
export CGO_CFLAGS="-I$FFMPEG_ROOT/include"
export LD_LIBRARY_PATH=$HOME/ffmpeg/lib


#install goav avformat
go get github.com/giorgisio/goav/avformat

#build ffmpeg
chmod +x ../scripts/install_ffmpeg.sh
./scripts/install_ffmpeg.sh

#get goydl
go get -u github.com/BrianAllred/goydl
go get -u google.golang.org/api/youtube/v3
go get -u google.golang.org/api/googleapi/transport
go get -u github.com/jinzhu/configor

#install youtubedl
curl -L https://yt-dl.org/latest/youtube-dl -o /usr/local/bin/youtube-dl
chmod a+rx /usr/local/bin/youtube-dl