package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var rootDir string
var (
	transcodeMutex sync.Mutex
	activeCmd      *exec.Cmd
)

type FileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	IsVideo  bool   `json:"isVideo"`
	CanPlay  bool   `json:"canPlay"`
	NeedsTranscode bool `json:"needsTranscode"`
}

// Video formats that browsers can typically play natively
var nativeFormats = map[string]bool{
	".mp4":  true,
	".webm": true,
	".ogg":  true,
}

// All video formats we'll recognize
var videoFormats = map[string]bool{
	".mp4":  true,
	".webm": true,
	".ogg":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".m4v":  true,
	".mpg":  true,
	".mpeg": true,
	".3gp":  true,
}

func main() {
	dir := flag.String("d", ".", "Directory to serve")
	port := flag.String("p", "8080", "Port to listen on")
	flag.Parse()

	var err error
	rootDir, err = filepath.Abs(*dir)
	if err != nil {
		log.Fatal("Invalid directory:", err)
	}

	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		log.Fatal("Directory does not exist:", rootDir)
	}

	log.Printf("Serving directory: %s", rootDir)
	log.Printf("Server starting on http://localhost:%s", *port)

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/browse", handleBrowse)
	http.HandleFunc("/api/video/", handleVideo)
	http.HandleFunc("/api/stream/", handleStream)

	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Stromboli</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body { width: 100%; height: 100%; overflow: hidden; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #1a1a1a;
            color: #e0e0e0;
            min-height: 100svh;
            display: flex;
            flex-direction: column;
        }
        header {
            background: #2d2d2d;
            padding: 1rem 2rem;
            border-bottom: 2px solid #3d3d3d;
        }
        h1 { font-size: 1.5rem; color: #fff; }
        .container {
            display: flex;
            flex: 1 1 auto;
            min-height: 0;
            overflow: hidden;
        }
        .browser {
            width: clamp(240px, 30vw, 350px);
            background: #242424;
            border-right: 1px solid #3d3d3d;
            display: flex;
            flex-direction: column;
            overflow: hidden;
            min-height: 0;
        }
        .breadcrumb {
            padding: 1rem;
            background: #2d2d2d;
            border-bottom: 1px solid #3d3d3d;
            font-size: 0.9rem;
            display: flex;
            align-items: center;
            justify-content: space-between;
            gap: 0.5rem;
        }
        .breadcrumb-path {
            flex: 1;
            overflow: hidden;
            white-space: nowrap;
            text-overflow: ellipsis;
            min-width: 0;
        }
        .breadcrumb span {
            color: #4a9eff;
            cursor: pointer;
            padding: 0.2rem 0.4rem;
            border-radius: 3px;
            text-transform: capitalize;
        }
        .breadcrumb span:hover { background: #3d3d3d; }
        .filter-toggle {
            background: #3d3d3d;
            border: none;
            color: #e0e0e0;
            padding: 0.5rem 0.75rem;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.9rem;
            margin-left: 0.5rem;
            flex-shrink: 0;
        }
        .filter-toggle:hover { background: #4d4d4d; }
        .filter-toggle.active { background: #4a9eff; color: #000; }
        .filter-bar {
            padding: 0.75rem 1rem;
            background: #2d2d2d;
            border-bottom: 1px solid #3d3d3d;
            display: none;
        }
        .filter-bar.visible { display: block; }
        .filter-input {
            width: 100%;
            padding: 0.5rem;
            background: #1a1a1a;
            border: 1px solid #3d3d3d;
            border-radius: 4px;
            color: #e0e0e0;
            font-size: 0.9rem;
        }
        .filter-input:focus {
            outline: none;
            border-color: #4a9eff;
        }
        .filter-input::placeholder { color: #666; }
        .file-list {
            flex: 1 1 auto;
            overflow-y: auto;
            padding: 0.5rem;
            min-height: 0;
            overscroll-behavior: contain;
            -webkit-overflow-scrolling: touch;
        }
        .file-item {
            padding: 0.75rem 1rem;
            cursor: pointer;
            border-radius: 4px;
            margin-bottom: 0.25rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        .file-item:hover { background: #2d2d2d; }
        .file-item.active { background: #3d3d3d; }
        .icon {
            font-size: 1.2rem;
            width: 24px;
            text-align: center;
        }
        .player {
            flex: 1 1 auto;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 2rem;
            min-height: 0;
            overflow: hidden;
        }
        video {
            max-width: 100%;
            max-height: 100%;
            background: #000;
            border-radius: 8px;
        }
        .empty-state {
            text-align: center;
            color: #666;
        }
        .empty-state h2 { font-size: 1.5rem; margin-bottom: 0.5rem; }
        .loading {
            text-align: center;
            padding: 2rem;
            color: #666;
        }
        .transcoding-notice {
            position: absolute;
            top: 1rem;
            right: 1rem;
            background: #ff9800;
            color: #000;
            padding: 0.5rem 1rem;
            border-radius: 4px;
            font-size: 0.9rem;
            font-weight: 500;
        }
		@media (max-width: 768px) {
			.container {
				flex-direction: column;
			}

			.browser {
				width: 100%;
				max-height: 40svh;
				border-right: none;
				border-bottom: 1px solid #3d3d3d;
			}

			.player {
				padding: 1rem;
			}

			header {
				padding: 0.75rem 1rem;
			}

			h1 {
				font-size: 1.25rem;
			}
			.file-item {
				padding: 1rem;
				font-size: 1rem;
			}

			.breadcrumb span {
				padding: 0.4rem 0.6rem;
			}
			.transcoding-notice {
				top: auto;
				bottom: 1rem;
				right: 50%;
				transform: translateX(50%);
				font-size: 0.8rem;
			}
		}
    </style>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body>
    <header>
        <h1>Stromboli</h1>
    </header>
    <div class="container">
        <div class="browser">
            <div class="breadcrumb" id="breadcrumb">
                <div class="breadcrumb-path" id="breadcrumbPath"></div>
                <button class="filter-toggle" id="filterToggle" onclick="toggleFilter()">&#x1F50D;</button>
            </div>
            <div class="filter-bar" id="filterBar">
                <input type="text" class="filter-input" id="filterInput" placeholder="Filter files and folders..." oninput="applyFilter()">
            </div>
            <div class="file-list" id="fileList">
                <div class="loading">Loading...</div>
            </div>
        </div>
        <div class="player" id="player">
            <div class="empty-state">
                <h2>Select a video to play</h2>
                <p>Browse the directory tree on the left</p>
            </div>
        </div>
    </div>

    <script>
        let currentPath = '';
        let currentVideo = null;
        let allFiles = [];
        let filterVisible = false;

        function toggleFilter() {
            filterVisible = !filterVisible;
            const filterBar = document.getElementById('filterBar');
            const filterToggle = document.getElementById('filterToggle');
            const filterInput = document.getElementById('filterInput');

            if (filterVisible) {
                filterBar.classList.add('visible');
                filterToggle.classList.add('active');
                filterInput.focus();
            } else {
                filterBar.classList.remove('visible');
                filterToggle.classList.remove('active');
                filterInput.value = '';
                renderFileList(allFiles);
            }
        }

        function applyFilter() {
            const filterText = document.getElementById('filterInput').value.toLowerCase();

            if (!filterText) {
                renderFileList(allFiles);
                return;
            }

            const filtered = allFiles.filter(file =>
                file.name.toLowerCase().includes(filterText)
            );

            renderFileList(filtered);
        }

        function browse(path = '') {
            currentPath = path;
            fetch('/api/browse?path=' + encodeURIComponent(path))
                .then(r => r.json())
                .then(files => {
                    allFiles = files;
                    updateBreadcrumb(path);

                    // Clear filter when changing directories
                    document.getElementById('filterInput').value = '';
                    renderFileList(files);
                })
                .catch(err => {
                    document.getElementById('fileList').innerHTML =
                        '<div class="loading">Error loading directory</div>';
                });
        }

        function updateBreadcrumb(path) {
            const parts = path ? path.split('/').filter(p => p) : [];
            const breadcrumbPath = document.getElementById('breadcrumbPath');

            let html = '<span onclick="browse(\'\')">Home</span>';
            let accumulated = '';

            parts.forEach(part => {
                accumulated += (accumulated ? '/' : '') + part;
                const thisPath = accumulated;
                html += ' / <span onclick="browse(\'' + thisPath + '\')">' + part + '</span>';
            });

            breadcrumbPath.innerHTML = html;
        }

        function renderFileList(files) {
            const list = document.getElementById('fileList');

            if (files.length === 0) {
                list.innerHTML = '<div class="loading">No matches found</div>';
                return;
            }

            // Sort: directories first, then files
            files.sort((a, b) => {
                if (a.isDir !== b.isDir) return b.isDir - a.isDir;
                return a.name.localeCompare(b.name);
            });

            list.innerHTML = files.map(file => {
                const icon = file.isDir ? '&#x1F4C1;' : (file.isVideo ? '&#x1F3AC;' : '&#x1F4C4;');
                let onclick = '';
                let clickHandler = '';

                if (file.isDir) {
                    onclick = 'onclick="browse(\'' + file.path + '\')"';
                } else if (file.isVideo) {
                    onclick = 'onclick="playVideo(\'' + file.path + '\', ' + file.canPlay + ')"';
                }

                return '<div class="file-item" ' + onclick + ' data-path="' + file.path + '">' +
                    '<span class="icon">' + icon + '</span>' +
                    '<span>' + file.name + '</span>' +
                    '</div>';
            }).join('');
        }

        function playVideo(path, canPlayNatively) {
            const player = document.getElementById('player');

            // Highlight selected file
            document.querySelectorAll('.file-item').forEach(el => {
                el.classList.toggle('active', el.dataset.path === path);
            });

            const videoUrl = canPlayNatively
                ? '/api/video/' + encodeURIComponent(path)
                : '/api/stream/' + encodeURIComponent(path);

            const transcodeNotice = canPlayNatively ? '' :
                '<div class="transcoding-notice">Transcoding...</div>';

            player.innerHTML = transcodeNotice +
                '<video controls autoplay id="activeVideo">' +
                    '<source src="' + videoUrl + '" type="video/mp4">' +
                    'Your browser does not support the video tag.' +
                '</video>';

            currentVideo = path;

            // Add event listener for when video ends
            const videoElement = document.getElementById('activeVideo');
            videoElement.addEventListener('ended', function() {
                playNextVideo();
            });
        }

        function playNextVideo() {
            // Find the current video in the file list
            const currentIndex = allFiles.findIndex(f => f.path === currentVideo);

            if (currentIndex === -1) return;

            // Find the next video file after the current one
            for (let i = currentIndex + 1; i < allFiles.length; i++) {
                if (allFiles[i].isVideo && !allFiles[i].isDir) {
                    // Found next video, play it
                    playVideo(allFiles[i].path, allFiles[i].canPlay);

                    // Scroll the file list to show the now-playing video
                    const fileItems = document.querySelectorAll('.file-item');
                    const nextItem = Array.from(fileItems).find(
                        item => item.dataset.path === allFiles[i].path
                    );
                    if (nextItem) {
                        nextItem.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    }
                    return;
                }
            }

            // No next video found
            console.log('No more videos to play');
        }

        // Initial load
        browse();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, tmpl)
}

func needsTranscoding(filePath string) bool {
	// Use ffprobe to check audio codec
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		// If we can't determine, assume it needs transcoding
		return true
	}

	audioCodec := strings.TrimSpace(string(output))
	
	// Browser-compatible audio codecs
	compatibleAudio := map[string]bool{
		"aac":  true,
		"mp3":  true,
		"opus": true,
		"vorbis": true,
	}

	return !compatibleAudio[audioCodec]
}

func handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	fullPath := filepath.Join(rootDir, path)

	// Security check: ensure we're not escaping the root directory
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(rootDir)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "Cannot read directory", http.StatusInternalServerError)
		return
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		isVideo := videoFormats[ext]
		canPlay := nativeFormats[ext]
		needsTranscode := false

		relativePath := filepath.Join(path, entry.Name())
		fullFilePath := filepath.Join(rootDir, relativePath)

		if canPlay && isVideo && !info.IsDir() {
			needsTranscode = needsTranscoding(fullFilePath)
			if needsTranscode {
				canPlay = false // Mark as needing transcode route
			}
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    relativePath,
			IsDir:   info.IsDir(),
			IsVideo: isVideo,
			CanPlay: canPlay,
			NeedsTranscode: needsTranscode,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func handleVideo(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/")
	fullPath := filepath.Join(rootDir, path)

	// Security check
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(rootDir)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Serve the file directly
	http.ServeFile(w, r, fullPath)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	fullPath := filepath.Join(rootDir, path)

	// Security check
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(rootDir)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Kill any existing transcoding process before starting a new one
	transcodeMutex.Lock()
	if activeCmd != nil && activeCmd.Process != nil {
		log.Printf("Killing existing ffmpeg process to start new transcode")
		activeCmd.Process.Kill()
		activeCmd.Wait() // Wait for it to fully exit
		activeCmd = nil
	}
	transcodeMutex.Unlock()

	// Set headers for streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")

	// FFmpeg command to transcode to H.264/AAC MP4
	cmd := exec.Command("ffmpeg",
		"-re", // Read input at native frame rate
		"-i", fullPath,
		"-map", "0:v:0", // First video stream only
		"-map", "0:a:0", // First audio stream only
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-crf", "23",
		"-maxrate", "3M",
		"-bufsize", "6M",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2", // Stereo audio
		"-movflags", "frag_keyframe+empty_moov+faststart",
		"-f", "mp4",
		"-loglevel", "warning",
		"pipe:1",
	)

	// Track this as the active command
	transcodeMutex.Lock()
	activeCmd = cmd
	transcodeMutex.Unlock()

	// Capture stderr for debugging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Error creating stderr pipe: %v", err)
		http.Error(w, "Transcoding error", http.StatusInternalServerError)
		return
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error creating stdout pipe: %v", err)
		http.Error(w, "Transcoding error", http.StatusInternalServerError)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Error starting ffmpeg: %v", err)
		http.Error(w, "Transcoding error", http.StatusInternalServerError)
		return
	}

	// Log stderr in background
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				log.Printf("FFmpeg: %s", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	// Monitor for client disconnect and kill ffmpeg if needed
	done := make(chan bool)
	go func() {
		// Copy output to response
		_, err = io.Copy(w, stdout)
		if err != nil {
			log.Printf("Error streaming video: %v", err)
		}
		done <- true
	}()

	// Wait for either completion or context cancellation
	select {
	case <-done:
		// Streaming finished normally
	case <-r.Context().Done():
		// Client disconnected
		log.Printf("Client disconnected, killing ffmpeg process for: %s", path)
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("Error killing ffmpeg: %v", err)
		}
	}

	// Clean up active command reference
	transcodeMutex.Lock()
	if activeCmd == cmd {
		activeCmd = nil
	}
	transcodeMutex.Unlock()

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		// Don't log error if we killed the process intentionally
		if r.Context().Err() == nil {
			log.Printf("FFmpeg error: %v", err)
		}
	}
}
