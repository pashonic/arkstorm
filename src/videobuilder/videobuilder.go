package videobuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type text struct {
	Text  string
	Cords struct {
		X int
		Y int
	}
	Color string
	Size  int
}

type clip struct {
	View  string
	Texts []text
	Speed string
	Time  string
}

type Video struct {
	Filename string
	Scale    string
	Clips    []clip
}

func BuildVideos(videos map[string]Video, assetDir string, outputDir string) (map[string]string, error) {

	// Make sure output directory exists
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return nil, err
	}

	// Process videos
	videoContent := make(map[string]string)
	for videoId, video := range videos {
		outputFilePath := filepath.Join(outputDir, video.Filename+".mp4")
		if err := build(&video, assetDir, outputFilePath); err != nil {
			return nil, err
		}
		videoContent[videoId] = outputFilePath
	}
	return videoContent, nil
}

func build(video *Video, assetDir string, outputFilePath string) error {

	// Add views to input stream
	var streamInputs []*ffmpeg.Stream
	for _, clip := range video.Clips {
		sourcePath := filepath.Join(assetDir, clip.View, "%03d.png")
		loopIntValue, err := strconv.ParseInt(clip.Time, 10, 64)
		if err != nil {
			return err
		}
		loop := "0"
		if loopIntValue > 0 {
			loop = "1"
		}

		// Process speed settings
		streamInput := ffmpeg.Input(sourcePath, ffmpeg.KwArgs{"loop": loop, "t": clip.Time}).Filter("setpts", ffmpeg.Args{fmt.Sprintf("%v*PTS", clip.Speed)})

		// Apply title if specified
		for _, text := range clip.Texts {
			titleArgs := ffmpeg.Args{
				fmt.Sprintf("text='%v'", text.Text),
				fmt.Sprintf("x=%v", text.Cords.X),
				fmt.Sprintf("y=%v", text.Cords.Y),
				fmt.Sprintf("fontsize=%v", text.Size),
				fmt.Sprintf("fontcolor=%v", text.Color),
			}
			streamInput = streamInput.Filter("drawtext", titleArgs)
		}
		streamInputs = append(streamInputs, streamInput)
	}

	// Scale and build video
	finalStream := ffmpeg.Concat(streamInputs)
	finalStream = finalStream.Filter("scale", ffmpeg.Args{video.Scale})
	return finalStream.Output(outputFilePath).OverWriteOutput().Run()
}
