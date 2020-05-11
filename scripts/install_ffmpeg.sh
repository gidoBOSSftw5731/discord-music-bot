mkdir tmp
cd tmp
rm ffmpeg-4.1.3.tar.xz*
wget https://ffmpeg.org/releases/ffmpeg-4.1.3.tar.xz 
tar -xvf ffmpeg-4.1.3.tar.xz
cd ffmpeg-4.1.3.tar.xz 
make -j16
make install -j16
cd ../tmp
