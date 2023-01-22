package videobuilder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type view struct {
	Title string
	Speed string
}

type Video struct {
	Filename        string
	ImagesPerSecond string
	Views           map[string]view
}

func CreateVideos(videos map[string]Video, assetDir string, outputDir string) map[string]string {

	// Make sure output directory exists
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	// Process videos
	videoContent := make(map[string]string)
	for videoId, video := range videos {
		outputFilePath := filepath.Join(outputDir, video.Filename+".mp4")
		create(&video, assetDir, outputFilePath)
		videoContent[videoId] = outputFilePath
	}
	return videoContent
}

func create(video *Video, assetDir string, outputFilePath string) {

	// Add views to input stream
	var streamInputs []*ffmpeg.Stream
	for viewName, view := range video.Views {
		sourcePath := filepath.Join(assetDir, viewName, "%03d.png")
		streamInput := ffmpeg.Input(sourcePath, ffmpeg.KwArgs{"r": video.ImagesPerSecond}).Filter("setpts", ffmpeg.Args{fmt.Sprintf("%v*PTS", view.Speed)})
		streamInputs = append(streamInputs, streamInput)
	}

	// Create video
	err := ffmpeg.Concat(streamInputs).Output(outputFilePath).OverWriteOutput().Run()
	if err != nil {
		log.Fatal(err)
	}
}
