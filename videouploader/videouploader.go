package videouploader

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	default_client_secret_file = "client_secret.json"
	default_client_token_file  = "client_token.json"
)

type Videos struct {
	Videos map[string]Video
}

type Video struct {
	Title       string
	Description string
	Privacy     string
	Tags        []string
	CategoryId  string
}

func getTokenFromFile(tokenFilePath string) *oauth2.Token {
	file, err := os.Open(tokenFilePath)
	if err != nil {
		log.Fatal(err)
	}
	token := &oauth2.Token{}
	err = json.NewDecoder(file).Decode(token)
	file.Close()
	return token
}

func UploadVideos(videos Videos, videoContent map[string]string) {
	for videoId, video := range videos.Videos {
		Upload(videoContent[videoId], &video)
	}
}

func Upload(videoFilePath string, video *Video) {
	ctx := context.Background()

	// Get config using google client config secret file
	byteData, err := ioutil.ReadFile(default_client_secret_file)
	if err != nil {
		log.Fatal(err)
	}
	config, err := google.ConfigFromJSON(byteData, youtube.YoutubeUploadScope)
	if err != nil {
		log.Fatal(err)
	}

	// Get Token file
	token := getTokenFromFile(default_client_token_file)

	// Initialize service
	service, err := youtube.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		log.Fatal(err)
	}

	// Create upload parameter object
	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       video.Title,
			Description: video.Description,
			CategoryId:  video.CategoryId,
		},
		Status: &youtube.VideoStatus{PrivacyStatus: video.Privacy},
	}
	upload.Snippet.Tags = video.Tags
	call := service.Videos.Insert([]string{"snippet,status"}, upload)

	// Open video file
	file, err := os.Open(videoFilePath)
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}

	// Upload video
	response, err := call.Media(file).Do()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Upload successful! Video ID: %v\n", response.Id)
}
