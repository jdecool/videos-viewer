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
)

type VideoFile struct {
	Name   string
	Path   string
	Viewed bool
}

type TemplateData struct {
	ReadmeContent    string
	Videos           []VideoFile
	CurrentVideo     string
	CurrentVideoFile *VideoFile
	FolderName       string
}

func main() {
	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()

	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s <directory_path>\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	path := os.Args[1]
	folderName := filepath.Base(path)

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

	// Load viewed videos from JSON if it exists
	viewedVideos := make(map[string]bool)
	if jsonData, err := os.ReadFile(path + "/viewed_videos.json"); err == nil {
		var savedVideos []VideoFile
		if err := json.Unmarshal(jsonData, &savedVideos); err == nil {
			for _, v := range savedVideos {
				viewedVideos[v.Name] = v.Viewed
			}
		}
	}

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if videoExtensions[ext] {
				base := filepath.Base(path)
				// Check if filename matches pattern "[number] - [name].[ext]"
				if parts := strings.Split(base, " - "); len(parts) == 2 {
					if num := strings.TrimSpace(parts[0]); num != "" {
						videoFiles = append(videoFiles, VideoFile{
							Name:   base,
							Path:   path,
							Viewed: viewedVideos[base],
						})
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking through directory: %v\n", err)
		os.Exit(1)
	}

	sort.Slice(videoFiles, func(i, j int) bool {
		numI := strings.Split(videoFiles[i].Name, " - ")[0]
		numJ := strings.Split(videoFiles[j].Name, " - ")[0]

		iNum, _ := strconv.Atoi(strings.TrimSpace(numI))
		jNum, _ := strconv.Atoi(strings.TrimSpace(numJ))
		return iNum < jNum
	})

	// Create HTML template
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
            <video width="100%" controls onended="onVideoEnded()">
                <source src="/video/{{.CurrentVideoFile.Name}}" type="video/mp4">
                Your browser does not support the video tag.
            </video>
        </div>
        {{else}}
        <h1 class="folder-name">{{.FolderName}}</h1>
        <h2>Select a video from the sidebar</h2>
		<p>{{.ReadmeContent}}</p>
        {{end}}
    </div>
</body>
</html>`
	t := template.Must(template.New("videoList").Parse(tmpl))

	// Handle root path - redirect to first video if available
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := TemplateData{
			ReadmeContent: readReadmeFile(path),
			Videos:        videoFiles,
			FolderName:    folderName,
		}
		t.Execute(w, data)
	})

	// Handle watch path
	http.HandleFunc("/watch/", func(w http.ResponseWriter, r *http.Request) {
		fileName := strings.TrimPrefix(r.URL.Path, "/watch/")
		var currentVideo *VideoFile
		for _, video := range videoFiles {
			if video.Name == fileName {
				currentVideo = &video
				break
			}
		}

		// Check if video was marked as ended
		if r.URL.Query().Get("ended") != "" && currentVideo != nil {
			endedFilename := r.URL.Query().Get("ended")
			// Mark video as viewed and persist to file
			for i := range videoFiles {
				if videoFiles[i].Name == endedFilename {
					videoFiles[i].Viewed = true
					// Save updated video files to JSON file
					jsonData, err := json.Marshal(videoFiles)
					if err != nil {
						log.Printf("Error marshaling video files: %v", err)
					} else {
						prettyJSON := &bytes.Buffer{}
						if err := json.Indent(prettyJSON, jsonData, "", "    "); err == nil {
							err = os.WriteFile(path+"/viewed_videos.json", prettyJSON.Bytes(), 0644)
						}
						if err != nil {
							log.Printf("Error saving viewed videos: %v", err)
						}
					}
					break
				}
			}
		}

		data := TemplateData{
			Videos:           videoFiles,
			CurrentVideo:     fileName,
			CurrentVideoFile: currentVideo,
			FolderName:       folderName,
		}
		t.Execute(w, data)
	})

	// Handle unview path
	http.HandleFunc("/unview/", func(w http.ResponseWriter, r *http.Request) {
		fileName := strings.TrimPrefix(r.URL.Path, "/unview/")
		// Remove ended query parameter if present
		if endedParam := r.URL.Query().Get("ended"); endedParam != "" {
			fileName = strings.TrimSuffix(fileName, "?ended="+endedParam)
		}

		for i := range videoFiles {
			if videoFiles[i].Name == fileName {
				videoFiles[i].Viewed = false
				// Save updated video files to JSON file
				jsonData, err := json.Marshal(videoFiles)
				if err != nil {
					log.Printf("Error marshaling video files: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				prettyJSON := &bytes.Buffer{}
				if err := json.Indent(prettyJSON, jsonData, "", "    "); err == nil {
					err = os.WriteFile(path+"/viewed_videos.json", prettyJSON.Bytes(), 0644)
				}
				if err != nil {
					log.Printf("Error saving viewed videos: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				// Get referer URL and remove any ended query parameter
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

				// Fallback to home if no valid referer
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
		http.NotFound(w, r)
	})

	// Handle video files
	http.HandleFunc("/video/", func(w http.ResponseWriter, r *http.Request) {
		fileName := strings.TrimPrefix(r.URL.Path, "/video/")
		for _, video := range videoFiles {
			if video.Name == fileName {
				http.ServeFile(w, r, video.Path)
				return
			}
		}
		http.NotFound(w, r)
	})

	fmt.Printf("Starting server at http://localhost:%s\n", *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

func readReadmeFile(basePath string) string {
	readmePaths := []string{
		"README.md",
		"README.txt",
		"readme.md",
		"readme.txt",
	}

	for _, path := range readmePaths {
		content, err := os.ReadFile(basePath + "/" + path)
		if err == nil {
			return string(content)
		}
	}

	return ""
}
