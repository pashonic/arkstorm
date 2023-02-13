package videouploader

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"github.com/pashonic/arkstorm/src/utils/sendsns"
	"github.com/pashonic/arkstorm/src/videobuilder"
)

const (
	default_client_secret_file = "client_secret.json"
	default_client_token_file  = "client_token.json"
)

type YoutubeVideos struct {
	Videos map[string]YoutubeVideo
}

type YoutubeVideo struct {
	Title       string
	Description string
	Privacy     string
	Tags        []string
	CategoryId  string
	SnsAlertArn string
}

func getTokenFromFile(tokenFilePath string) (*oauth2.Token, error) {
	file, err := os.Open(tokenFilePath)
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}
	token := &oauth2.Token{}
	if err := json.NewDecoder(file).Decode(token); err != nil {
		return nil, err
	}
	return token, nil
}

func UploadVideos(youtubeVideos *YoutubeVideos, videos map[string]videobuilder.OutputVideo) error {

	for videoId, youtubeVideo := range youtubeVideos.Videos {
		video, exists := videos[videoId]
		if !exists {
			return errors.New("Generated video ID doesn't exist")
		}
		if err := upload(video, youtubeVideo); err != nil {
			return err
		}
	}
	return nil
}

func upload(video videobuilder.OutputVideo, youtubeVideo YoutubeVideo) error {
	ctx := context.Background()

	// Get config using google client config secret file
	byteData, err := ioutil.ReadFile(default_client_secret_file)
	if err != nil {
		return err
	}
	config, err := google.ConfigFromJSON(byteData, youtube.YoutubeUploadScope)
	if err != nil {
		return err
	}

	// Get Token file
	token, err := getTokenFromFile(default_client_token_file)
	if err != nil {
		return err
	}

	// Initialize service
	service, err := youtube.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		return err
	}

	description := youtubeVideo.Description + "\n\n"
	for _, clip := range video.Clips {
		timeString := fmt.Sprintf(secondsToMinutes(clip.StartTimeSec))
		description += fmt.Sprintf("%v %v\n", timeString, clip.Name)
	}

	// Create upload parameter object
	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       youtubeVideo.Title,
			Description: description,
			CategoryId:  youtubeVideo.CategoryId,
		},
		Status: &youtube.VideoStatus{PrivacyStatus: youtubeVideo.Privacy},
	}
	upload.Snippet.Tags = youtubeVideo.Tags
	call := service.Videos.Insert([]string{"snippet,status"}, upload)

	// Open video file
	file, err := os.Open(video.FilePath)
	defer file.Close()
	if err != nil {
		return err
	}

	// Upload video
	response, err := call.Media(file).Do()
	if err != nil {
		return err
	}
	log.Printf("Upload successful! Video ID: %v\n", response.Id)

	// Send SNS alert
	youtubeLink := "https://youtu.be/" + response.Id
	if youtubeVideo.SnsAlertArn != "" {
		if err := sendsns.SendSNS("Washington Weather Video Uploaded", youtubeLink, youtubeVideo.SnsAlertArn); err != nil {
			return err
		}
	}
	return nil
}

func secondsToMinutes(inSeconds int) string {
	minutes := inSeconds / 60
	seconds := inSeconds % 60
	return fmt.Sprintf("%v:%02d", minutes, seconds)
}
