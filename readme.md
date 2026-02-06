# Stromboli

A simple golang application to provide a web UI file browser and video player.

Will use native video streaming for supported video files and transcode anything that isn't supported.

## Running

```
cd stromboli
go run . -d /your/video/directory/ -p 8080
```

Then access the servers IP address via a web browser on port `8080`.

## Limitations
* Uses the host CPU for transcoding so you'll need something reasonably powerful
* Doesn't support soft subtitles
* You can't select anything past the first audio channel
* Seeking isn't supported on transcoded files
* The UI on mobile isn't great