package videobuilder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
	Name  string
	Speed int
	Time  int
}

type Video struct {
	Filename       string
	OutputFilePath string
	Scale          string
	Clips          []clip
}

type OutputClip struct {
	Name         string
	StartTimeSec int
}

type OutputVideo struct {
	FilePath string
	Clips    []OutputClip
}

func BuildVideos(videos map[string]Video, assetDir string, outputDir string) (map[string]OutputVideo, error) {
	returnVideos := map[string]OutputVideo{}

	// Make sure output directory exists
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return nil, err
	}

	// Process videos
	for videoId, video := range videos {
		var outputVideo OutputVideo
		outputVideo.FilePath = filepath.Join(outputDir, videos[videoId].Filename+".mp4")
		returnClips, err := build(&video, assetDir, outputVideo.FilePath)
		if err != nil {
			return nil, err
		}
		outputVideo.Clips = returnClips
		returnVideos[videoId] = outputVideo
	}
	return returnVideos, nil
}

func build(video *Video, assetDir string, outputFilePath string) ([]OutputClip, error) {
	returnClips := []OutputClip{}

	// Add views to input stream
	var streamInputs []*ffmpeg.Stream
	currentTimeSec := 0
	for _, clip := range video.Clips {
		var outputClip OutputClip

		// Create source paths
		sourceDir := filepath.Join(assetDir, clip.View)
		sourcePath := filepath.Join(sourceDir, "%03d.png")

		// Set loop identifer and calulate clip time
		fileList, err := ioutil.ReadDir(sourceDir) // Clip time depends on how many image files there are
		if err != nil {
			return nil, err
		}
		fileCount := float64(len(fileList))
		outputClip.StartTimeSec = currentTimeSec
		loop := "0"
		if clip.Time > 0 { // We want to handle static frame segments differently
			loop = "1"
			currentTimeSec += int(clip.Time)
		} else {
			speedFloat := float64(clip.Speed)
			currentTimeSec += int((fileCount * .04) * speedFloat)
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

		// Store return clip
		outputClip.Name = clip.Name
		returnClips = append(returnClips, outputClip)
	}

	// Scale and build video
	finalStream := ffmpeg.Concat(streamInputs)
	finalStream = finalStream.Filter("scale", ffmpeg.Args{video.Scale})
	return returnClips, finalStream.Output(outputFilePath).OverWriteOutput().Run()
}
