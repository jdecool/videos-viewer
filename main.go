package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	videoDataFile = "video_data.json"
)

var (
	isDebugMode bool
)

type VideoFile struct {
	// File information
	Name   string
	Path   string
	Viewed bool

	// User progression information
	Current  time.Time
	Progress float64
}

type TemplateData struct {
	ReadmeContent    string
	Videos           []VideoFile
	CurrentVideo     string
	CurrentVideoFile *VideoFile
	FolderName       string
}

func loadViewedVideos(path string) (map[string]VideoFile, error) {
	viewedVideos := make(map[string]VideoFile)

	jsonData, err := os.ReadFile(filepath.Join(path, videoDataFile))
	if err != nil {
		return viewedVideos, nil
	}

	var savedVideos []VideoFile
	if err := json.Unmarshal(jsonData, &savedVideos); err != nil {
		return nil, err
	}

	for _, v := range savedVideos {
		viewedVideos[v.Name] = v
	}

	return viewedVideos, nil
}

func main() {
	var port string
	flag.StringVar(&port, "port", "8080", "port to listen on")
	flag.BoolVar(&isDebugMode, "debug", false, "enable debug mode")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <directory_path>\n\nOptions:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	path := flag.Arg(0)
	folderName := filepath.Base(path)

	debug("Load \"%s\"", path)

	videoFiles, err := loadVideoFiles(path)
	if err != nil {
		log.Fatalf("Error loading video files: %v", err)
	}

	tmpl := createTemplate()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRoot(w, r, path, videoFiles, folderName, tmpl)
	})

	http.HandleFunc("/watch/", func(w http.ResponseWriter, r *http.Request) {
		handleWatch(w, r, videoFiles, folderName, tmpl, path)
	})

	http.HandleFunc("/unview/", func(w http.ResponseWriter, r *http.Request) {
		handleUnview(w, r, videoFiles, path)
	})

	http.HandleFunc("/video/", func(w http.ResponseWriter, r *http.Request) {
		handleVideo(w, r, videoFiles)
	})

	http.HandleFunc("/update-progress/", func(w http.ResponseWriter, r *http.Request) {
		handleUpdateProgress(w, r, path)
	})

	fmt.Printf("Starting server at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func loadVideoFiles(path string) ([]VideoFile, error) {
	videoExtensions := map[string]bool{
		".mp4":  true,
		".avi":  true,
		".mkv":  true,
		".mov":  true,
		".wmv":  true,
		".flv":  true,
		".webm": true,
	}

	var videoFiles []VideoFile

	viewedVideos, err := loadViewedVideos(path)
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		ext := strings.ToLower(filepath.Ext(path))
		if videoExtensions[ext] {
			base := filepath.Base(path)
			videoFile := VideoFile{
				Name:     base,
				Path:     path,
				Viewed:   viewedVideos[base].Viewed,
				Current:  viewedVideos[base].Current,
				Progress: viewedVideos[base].Progress,
			}
			videoFiles = append(videoFiles, videoFile)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(videoFiles, func(i, j int) bool {
		numI, _ := strconv.Atoi(strings.TrimSpace(strings.Split(videoFiles[i].Name, " - ")[0]))
		numJ, _ := strconv.Atoi(strings.TrimSpace(strings.Split(videoFiles[j].Name, " - ")[0]))

		return numI < numJ
	})

	return videoFiles, nil
}

func createTemplate() *template.Template {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Video Player</title>
    <style>
        body { 
            font-family: Arial, sans-serif; 
            margin: 0;
            display: flex;
        }
        .sidebar {
            width: 300px;
            background: #f5f5f5;
            height: 100vh;
            overflow-y: auto;
            padding: 20px;
            box-sizing: border-box;
        }
        .main-content {
            flex-grow: 1;
            padding: 20px;
        }
        .video-list { 
            list-style: none; 
            padding: 0; 
        }
        .video-item { 
            margin: 10px 0; 
            padding: 10px; 
            border: 1px solid #ddd;
            border-radius: 4px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .video-link {
            text-decoration: none;
            color: #333;
            flex-grow: 1;
        }
        .video-link:hover {
            color: #007bff;
        }
        .current-video {
            background: #e0e0e0;
        }
        .video-container {
            max-width: 1280px;
            margin: 0 auto;
        }
        .folder-name {
            text-align: center;
            color: #333;
            margin-bottom: 30px;
        }
        .viewed::after {
            content: "✓";
            color: green;
            margin-left: 5px;
        }
        .unview-btn {
            background: none;
            border: none;
            color: red;
            cursor: pointer;
            padding: 2px 5px;
            margin-left: 5px;
            font-size: 12px;
            display: none;
        }
        .viewed .unview-btn {
            display: inline;
        }
    </style>
    <script>
        function onVideoEnded() {
            const currentVideo = document.querySelector('.current-video a');
            const nextVideo = currentVideo.parentElement.nextElementSibling?.querySelector('a');
            if (nextVideo) {
                window.location.href = nextVideo.href + '?ended=' + currentVideo.textContent;
            }
        }
        
        function unviewVideo(videoName, event) {
            event.preventDefault();
            fetch('/unview/' + videoName)
                .then(response => {
                    if (response.ok) {
                        window.location.reload();
                    }
                });
        }
        
        let time = 0;
        function updateProgress(videoName, exactTime) {
            const current = Math.floor(exactTime);
            if (current === time) {
                return;
            }

            time = current;
            if (time % 10 !== 0) {
                return;
            }

            fetch('/update-progress/' + videoName + '/' + exactTime);
        }
    </script>
</head>
<body>
    <div class="sidebar">
        <h2>Video List</h2>
        <ul class="video-list">
            {{range .Videos}}
            <li class="video-item {{if eq .Name $.CurrentVideo}}current-video{{end}} {{if .Viewed}}viewed{{end}}">
                <a href="/watch/{{.Name}}" class="video-link">{{.Name}}</a>
                <button class="unview-btn" onclick="unviewVideo('{{.Name}}', event)">×</button>
            </li>
            {{end}}
        </ul>
    </div>
    <div class="main-content">
        {{if .CurrentVideoFile}}
        <div class="video-container">
            <h1>{{.CurrentVideoFile.Name}}</h1>
            <video width="100%" controls onended="onVideoEnded()" ontimeupdate="updateProgress('{{.CurrentVideoFile.Name}}', this.currentTime)">
                <source src="/video/{{.CurrentVideoFile.Name}}" type="video/mp4">
                Your browser does not support the video tag.
            </video>
            <button onclick="onVideoEnded()">Next Video</button>
            <script>
                document.querySelector('video').addEventListener('loadedmetadata', function() {
                    this.currentTime = {{.CurrentVideoFile.Progress}};
                });
            </script>
        </div>
        {{else}}
        <h1 class="folder-name">{{.FolderName}}</h1>
        <h2>Select a video from the sidebar</h2>
		<p>{{.ReadmeContent}}</p>
        {{end}}
    </div>
</body>
</html>`

	return template.Must(template.New("videoList").Parse(tmpl))
}

func handleRoot(w http.ResponseWriter, r *http.Request, path string, videoFiles []VideoFile, folderName string, tmpl *template.Template) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := TemplateData{
		ReadmeContent: readReadmeFile(path),
		Videos:        videoFiles,
		FolderName:    folderName,
	}

	tmpl.Execute(w, data)
}

func handleWatch(w http.ResponseWriter, r *http.Request, videoFiles []VideoFile, folderName string, tmpl *template.Template, path string) {
	fileName := strings.TrimPrefix(r.URL.Path, "/watch/")

	var currentVideo *VideoFile
	for _, video := range videoFiles {
		if video.Name == fileName {
			currentVideo = &video
			break
		}
	}

	if r.URL.Query().Get("ended") != "" && currentVideo != nil {
		markVideoAsViewed(r.URL.Query().Get("ended"), videoFiles, path)
	}

	data := TemplateData{
		Videos:           videoFiles,
		CurrentVideo:     fileName,
		CurrentVideoFile: currentVideo,
		FolderName:       folderName,
	}

	tmpl.Execute(w, data)
}

func markVideoAsViewed(endedFilename string, videoFiles []VideoFile, path string) {
	for i := range videoFiles {
		if videoFiles[i].Name == endedFilename {
			videoFiles[i].Viewed = true
			videoFiles[i].Current = time.Now()
			videoFiles[i].Progress = 0

			saveViewedVideos(videoFiles, path)
			break
		}
	}
}

func saveViewedVideos(videoFiles []VideoFile, path string) {
	jsonData, err := json.Marshal(videoFiles)
	if err != nil {
		log.Printf("Error marshaling video files: %v", err)
		return
	}

	prettyJSON := &bytes.Buffer{}
	if err := json.Indent(prettyJSON, jsonData, "", "    "); err == nil {
		err = os.WriteFile(filepath.Join(path, videoDataFile), prettyJSON.Bytes(), 0644)
		if err != nil {
			log.Printf("Error saving viewed videos: %v", err)
			return
		}
	}
}

func handleUnview(w http.ResponseWriter, r *http.Request, videoFiles []VideoFile, path string) {
	fileName := strings.TrimPrefix(r.URL.Path, "/unview/")
	for i := range videoFiles {
		if videoFiles[i].Name == fileName {
			videoFiles[i].Viewed = false
			saveViewedVideos(videoFiles, path)
			redirectAfterUnview(w, r)
			return
		}
	}

	http.NotFound(w, r)
}

func redirectAfterUnview(w http.ResponseWriter, r *http.Request) {
	referer := r.Header.Get("Referer")
	if referer != "" {
		if refererURL, err := url.Parse(referer); err == nil {
			q := refererURL.Query()
			q.Del("ended")
			refererURL.RawQuery = q.Encode()
			http.Redirect(w, r, refererURL.String(), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleVideo(w http.ResponseWriter, r *http.Request, videoFiles []VideoFile) {
	fileName := strings.TrimPrefix(r.URL.Path, "/video/")
	for _, video := range videoFiles {
		if video.Name == fileName {
			http.ServeFile(w, r, video.Path)
			return
		}
	}

	http.NotFound(w, r)
}

func handleUpdateProgress(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(r.URL.Path, "/")
	progress, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		log.Printf("Invalid progress vaule: %v", err)
		http.Error(w, "Invalid progress value", http.StatusBadRequest)
		return
	}

	videoFiles, err := loadVideoFiles(path)
	if err != nil {
		log.Printf("Error loading video progress: %v", err)
		http.Error(w, "Error loading video progress", http.StatusInternalServerError)
		return
	}

	fileName := parts[len(parts)-2]
	for k, video := range videoFiles {
		if video.Name == fileName {
			videoFiles[k].Current = time.Now()
			videoFiles[k].Progress = progress
			saveViewedVideos(videoFiles, path)
			break
		}
	}

	w.WriteHeader(http.StatusOK)
}

func readReadmeFile(basePath string) string {
	readmePaths := []string{
		"README.md",
		"README.txt",
		"readme.md",
		"readme.txt",
	}

	for _, path := range readmePaths {
		content, err := os.ReadFile(filepath.Join(basePath, path))
		if err == nil {
			return string(content)
		}
	}

	return ""
}

func debug(format string, v ...any) {
	if !isDebugMode {
		return
	}

	log.Printf(format, v...)
}
