package main

import (
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/pashonic/arkstorm/providers/weatherbell"
	"github.com/pashonic/arkstorm/videobuilder"
	"github.com/pashonic/arkstorm/videouploader"
)

const (
	default_assets_dir        = "assets"
	default_output_videos_dir = "videos"
	default_config_file       = "config.toml"
)

type config struct {
	Providers struct {
		Weatherbell weatherbell.Weatherbell
	}
	Videos  map[string]videobuilder.Video
	Youtube videouploader.Videos
}

func main() {

	// Check for client secret file path
	var configFile string
	if len(os.Args) == 2 {
		configFile = os.Args[1]
	} else {
		configFile = default_config_file
	}

	// Load configuration
	var conf config
	_, err := toml.DecodeFile(configFile, &conf)
	if err != nil {
		log.Fatal(err)
	}

	// Download weatherbell assets
	weatherbell.Download(&conf.Providers.Weatherbell, default_assets_dir)

	// Make videos from asset views
	videoContent := videobuilder.BuildVideos(conf.Videos, default_assets_dir, default_output_videos_dir)

	// Upload videos
	videouploader.UploadVideos(&conf.Youtube, videoContent)
}
