package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/youtube/v3"
)

func main() {

	// Check arguments.
	if len(os.Args) != 2 {
		fmt.Println("app [client secret file path]")
		os.Exit(0)
	}
	clientSecretFilePath := os.Args[1]

	// Load client config
	byteData, err := ioutil.ReadFile(clientSecretFilePath)
	if err != nil {
		log.Fatal(err)
	}
	config, err := google.ConfigFromJSON(byteData, youtube.YoutubeUploadScope)
	if err != nil {
		log.Fatal(err)
	}

	// Ask user to access link from browser to get auth code
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Browser URL: \n%v\n", authURL)

	// Wait for user to enter code
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatal(err)
	}

	// Confirm code from user
	token, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatal(err)
	}

	// Save token file
	tokenCacheFilePath := path.Join(filepath.Dir(clientSecretFilePath), "client_token.json")
	fmt.Printf("Saving credential file to: %s\n", tokenCacheFilePath)
	file, err := os.OpenFile(tokenCacheFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatal(err)
	}
	json.NewEncoder(file).Encode(token)
	file.Close()
}
