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
)

var rootDir string

type FileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	IsVideo  bool   `json:"isVideo"`
	CanPlay  bool   `json:"canPlay"`
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
    <title>Video Browser</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #1a1a1a;
            color: #e0e0e0;
            height: 100vh;
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
            flex: 1;
            overflow: hidden;
        }
        .browser {
            width: 350px;
            background: #242424;
            border-right: 1px solid #3d3d3d;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        .breadcrumb {
            padding: 1rem;
            background: #2d2d2d;
            border-bottom: 1px solid #3d3d3d;
            font-size: 0.9rem;
            overflow-x: auto;
            white-space: nowrap;
        }
        .breadcrumb span {
            color: #4a9eff;
            cursor: pointer;
            padding: 0.2rem 0.4rem;
            border-radius: 3px;
        }
        .breadcrumb span:hover { background: #3d3d3d; }
        .file-list {
            flex: 1;
            overflow-y: auto;
            padding: 0.5rem;
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
            flex: 1;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 2rem;
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
    </style>
</head>
<body>
    <header>
        <h1>&#x1F4F9; Video Browser</h1>
    </header>
    <div class="container">
        <div class="browser">
            <div class="breadcrumb" id="breadcrumb"></div>
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

        function browse(path = '') {
            currentPath = path;
            fetch('/api/browse?path=' + encodeURIComponent(path))
                .then(r => r.json())
                .then(files => {
                    updateBreadcrumb(path);
                    renderFileList(files);
                })
                .catch(err => {
                    document.getElementById('fileList').innerHTML = 
                        '<div class="loading">Error loading directory</div>';
                });
        }

        function updateBreadcrumb(path) {
            const parts = path ? path.split('/').filter(p => p) : [];
            const breadcrumb = document.getElementById('breadcrumb');
            
            let html = '<span onclick="browse(\'\')">&#x1F3E0; Home</span>';
            let accumulated = '';
            
            parts.forEach(part => {
                accumulated += (accumulated ? '/' : '') + part;
                const thisPath = accumulated;
                html += ' / <span onclick="browse(\'' + thisPath + '\')">' + part + '</span>';
            });
            
            breadcrumb.innerHTML = html;
        }

        function renderFileList(files) {
            const list = document.getElementById('fileList');
            
            if (files.length === 0) {
                list.innerHTML = '<div class="loading">Empty directory</div>';
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
                '<div class="transcoding-notice">&#x26A1; Transcoding on-the-fly</div>';

            player.innerHTML = transcodeNotice +
                '<video controls autoplay>' +
                    '<source src="' + videoUrl + '" type="video/mp4">' +
                    'Your browser does not support the video tag.' +
                '</video>';
            
            currentVideo = path;
        }

        // Initial load
        browse();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, tmpl)
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

		relativePath := filepath.Join(path, entry.Name())

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    relativePath,
			IsDir:   info.IsDir(),
			IsVideo: isVideo,
			CanPlay: canPlay,
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

	// Set headers for streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")

	// FFmpeg command to transcode to H.264/AAC MP4
	cmd := exec.Command("ffmpeg",
		"-i", fullPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"pipe:1",
	)

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

	// Copy output to response
	_, err = io.Copy(w, stdout)
	if err != nil {
		log.Printf("Error streaming video: %v", err)
	}

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		log.Printf("FFmpeg error: %v", err)
	}
}
