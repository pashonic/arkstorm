package videobuilder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type clip struct {
	View  string
	Title string
	Speed string
	Time  string
}

type Video struct {
	Filename string
	Clips    []clip
}

func BuildVideos(videos map[string]Video, assetDir string, outputDir string) map[string]string {

	// Make sure output directory exists
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	// Process videos
	videoContent := make(map[string]string)
	for videoId, video := range videos {
		outputFilePath := filepath.Join(outputDir, video.Filename+".mp4")
		build(&video, assetDir, outputFilePath)
		videoContent[videoId] = outputFilePath
	}
	return videoContent
}

func build(video *Video, assetDir string, outputFilePath string) {

	// Add views to input stream
	var streamInputs []*ffmpeg.Stream
	for _, clip := range video.Clips {
		sourcePath := filepath.Join(assetDir, clip.View, "%03d.png")
		loopIntValue, err := strconv.ParseInt(clip.Time, 10, 64)
		if err != nil {
			log.Fatalln(err)
		}
		loop := "0"
		if loopIntValue > 0 {
			loop = "1"
		}
		streamInput := ffmpeg.Input(sourcePath, ffmpeg.KwArgs{"loop": loop, "t": clip.Time}).Filter("setpts", ffmpeg.Args{fmt.Sprintf("%v*PTS", clip.Speed)})
		streamInputs = append(streamInputs, streamInput)
	}

	// Build video
	err := ffmpeg.Concat(streamInputs).Output(outputFilePath).OverWriteOutput().Run()
	if err != nil {
		log.Fatal(err)
	}
}
