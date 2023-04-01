# ffmpeg-webrtc
ffmpeg-webrtc is an example app that demonstrates how to stream a h264 capable web cam via Pion WebRTC on linux based systems

## Dependencies
* ffmpeg
* v4l2
* h264 capable usb cam

## Instructions
Install v4l-utils
```
sudo apt-get install v4l-utils
```
Install ffmpeg
```
sudo apt-get install ffmpeg
```
Build
```
git clone https://github.com/dwoja22/ffmpeg-webrtc.git
cd ffmpeg-webrtc
go build
```
Run it
```
./ffmpeg-webrtc
```
* open Firefox or Google Chrome and navigate to localhost:7000
* click play
