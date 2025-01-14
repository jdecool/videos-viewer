# Videos Viewer

This project is a simple web application that allows users to browse and play video files stored in a specified directory.

## Features

- **Video Browsing**: Users can view a list of videos in a specified directory.
- **Video Playback**: Users can play videos directly in the browser.
- **Viewed Status**: The application tracks which videos have been viewed and allows users to mark them as unviewed.

## Requirements

- Go (version 1.23 or higher)
- A directory containing video files (supported formats: .mp4, .avi, .mkv, .mov, .wmv, .flv, .webm)
- A JSON file `viewed_videos.json` will be created in the videos directory to store the viewed status.

## Installation

1. Clone the repository:

   ```bash
   git clone <repository-url>
   cd <repository-directory>
   ```
2. Build the application:
   ```bash
   go build -o video-player main.go
   ```

3. Run the application:
   ```bash
   ./video-player <directory_path>
   ```

   Replace `<directory_path>` with the path to the directory containing your video files.
