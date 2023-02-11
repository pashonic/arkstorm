package main

import (
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/pashonic/arkstorm/src/providers/weatherbell"
	"github.com/pashonic/arkstorm/src/videobuilder"
	"github.com/pashonic/arkstorm/src/videouploader"
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
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		log.Fatalln(err)
		return
	}

	// Download weatherbell assets
	if err := weatherbell.Download(&conf.Providers.Weatherbell, default_assets_dir); err != nil {
		log.Fatalln(err)
		return
	}

	// Make videos from asset views
	videoContent, err := videobuilder.BuildVideos(conf.Videos, default_assets_dir, default_output_videos_dir)
	if err != nil {
		log.Fatalln(err)
		return
	}

	// Upload videos
	_, err = videouploader.UploadVideos(&conf.Youtube, videoContent)
	if err != nil {
		log.Fatalln(err)
		return
	}
}
