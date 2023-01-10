package main

import (
	"log"

	"github.com/BurntSushi/toml"
	"github.com/pashonic/arkstorm/providers/weatherbell"
	"github.com/pashonic/arkstorm/videobuilder"
	"github.com/pashonic/arkstorm/videouploader"
)

const (
	default_assets_dir        = "assets"
	default_output_videos_dir = "videos"
	default_config_file       = "example-configs/simple.toml"
)

type config struct {
	Providers weatherbell.Sources
	Videos    map[string]videobuilder.Video
	Youtube   videouploader.Videos
}

func main() {

	// Load configuration
	var conf config
	_, err := toml.DecodeFile(default_config_file, &conf)
	if err != nil {
		log.Fatal(err)
	}

	// Download weatherbell assets
	views := weatherbell.Download(&conf.Providers.Weatherbell, default_assets_dir)

	// Make videos from created views
	videoContent := videobuilder.CreateVideos(views, conf.Videos, default_output_videos_dir)

	// Upload video
	videouploader.UploadVideos(&conf.Youtube, videoContent)
}
