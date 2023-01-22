package weatherbell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	login_url          = "https://www.weatherbell.com/login-captcha"
	api_image_url      = "https://maps.api.weatherbell.com/image/"
	image_stroage_url  = "https://images.weatherbell.com"
	env_username_name  = "WEATHERBELL_USERNAME"
	env_password_name  = "WEATHERBELL_PASSWORD"
	env_sessionid_name = "WEATHERBELL_SESSIONID"
	default_font_file  = "fonts/Yagora.ttf"
)

type Weatherbell struct {
	Views map[string]WeatherBellView
}

type WeatherBellView struct {
	Viewtype       string
	Product        string
	Region         string
	Parameter      string
	Label_timezone string
	Cyclehours     []int
}

func Download(weatherbell *Weatherbell, targetDir string) {

	// Get session ID
	sessionId := os.Getenv(env_sessionid_name)
	if sessionId == "" {
		sessionId = getSessionId()
	}

	// Process views
	for viewName, view := range weatherbell.Views {

		// Get cycle list from site
		cycleList := view.getCycleList(sessionId)

		// Find latest cycle time
		selectedCycleTime := view.selectCycleTime(cycleList)

		// Get image list
		imageUrlList := view.getImageUrlList(sessionId, selectedCycleTime)

		// Download Images
		targetDir := filepath.Join(targetDir, viewName)
		downloadImageSet(imageUrlList, view.Label_timezone, targetDir)
	}
}

func getSessionId() string {

	// Get Credentials from environment variable
	username := os.Getenv(env_username_name)
	password := os.Getenv(env_password_name)

	// Prepare request
	loginPayload := strings.NewReader(fmt.Sprintf("username=%s&password=%s&remember_me=1&do_login=Login", username, password))
	req, err := http.NewRequest("POST", login_url, loginPayload)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}

	// Process request
	regex, err := regexp.Compile(`PHPSESSID=(\w+)`)
	if err != nil {
		log.Fatalln(err)
	}
	setCookies := res.Header.Values("Set-Cookie")
	if len(setCookies) < 2 {
		log.Fatalln("Unable get to session ID, likely invalid credentials")
	}
	match := regex.FindStringSubmatch(setCookies[0])
	if match == nil {
		log.Fatal("Could not get session ID")
	}
	res.Body.Close()
	fmt.Println("Session ID: " + match[1])
	return match[1]
}

func downloadImageSet(imageUrlList []string, timeZone string, targetDir string) {
	for index, imageUrl := range imageUrlList {

		// Create and verify directory path
		err := os.MkdirAll(targetDir, os.ModePerm)
		if err != nil {
			log.Fatalln(err)
		}

		// Send request
		client := http.Client{}
		res, err := client.Get(imageUrl)
		if err != nil {
			log.Fatalln(err)
		}

		// Read image from body.
		body, err := ioutil.ReadAll(res.Body)
		img, _, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			log.Fatalln(err)
		}

		// Get timestamp from image url
		timeStampRegex := regexp.MustCompile(`(\d+)-\w+\.png`)
		timestampString := timeStampRegex.FindStringSubmatch(imageUrl)[1]

		// Convert timestamp to time object
		timeStampString := regexp.MustCompile(`\.\w+$`).ReplaceAllString(timestampString, "")
		intVar, err := strconv.ParseInt(timeStampString, 10, 64)
		if err != nil {
			log.Fatalln(err)
		}
		viewTime := time.Unix(intVar, 0)
		location, err := time.LoadLocation(timeZone)
		if err != nil {
			log.Fatalln(err)
		}
		viewTime = viewTime.In(location)
		dateTimeString := viewTime.Format("Mon, 2 Jan 15:04 PM MST")

		// Draw date/time label to image
		bounds := img.Bounds()
		imgRGBA := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(imgRGBA, imgRGBA.Bounds(), img, bounds.Min, draw.Src)
		addLabel(imgRGBA, 420, 25, dateTimeString) // Future version: make this configurable

		// Write final image to file
		localTargetPath := filepath.Join(targetDir, fmt.Sprintf("%03d.png", index))
		fmt.Println("Saving File: ", localTargetPath)
		out, _ := os.Create(localTargetPath)
		err = png.Encode(out, imgRGBA)
		if err != nil {
			log.Fatalln(err)
		}
		out.Close()
	}
}
func addLabel(img *image.RGBA, x, y int, label string) {

	// Load font
	fontFile, err := ioutil.ReadFile(default_font_file)
	if err != nil {
		log.Fatalln(err)
	}
	ttf, err := truetype.Parse(fontFile)
	if err != nil {
		log.Fatalln(err)
	}

	// Configure text
	face := truetype.NewFace(ttf, &truetype.Options{ // Future version: make this configurable
		Size:    24,
		DPI:     72,
		Hinting: font.HintingNone,
	})

	col := color.RGBA{255, 0, 0, 255} // red, Future version: make this configurable
	point := fixed.Point26_6{fixed.I(x), fixed.I(y)}

	// Draw text
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  point,
	}
	d.DrawString(label)
}

func (view *WeatherBellView) getImageUrlList(sessionId string, cycleTime string) []string {

	// Prepare request
	payload := strings.NewReader(fmt.Sprintf(`{"action":"forecast","type":"%s","product":"%s","domain":"%s","param":"%s","init":"%s"}`, view.Viewtype, view.Product, view.Region, view.Parameter, cycleTime))
	req, err := http.NewRequest("POST", api_image_url, payload)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("cookie", "PHPSESSID="+sessionId)
	req.Header.Add("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	// Process request
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	var imageList []string
	json.Unmarshal([]byte(body), &imageList)
	if err != nil {
		log.Fatal(err)
	}
	res.Body.Close()

	// Convert to URL list and return
	for index, imageName := range imageList {
		imageList[index] = fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s.png", image_stroage_url, view.Viewtype, view.Product, view.Region, view.Parameter, cycleTime, imageName)
	}
	return imageList
}

func (view *WeatherBellView) getCycleList(sessionId string) []string {

	// Prepare request
	payload := strings.NewReader(fmt.Sprintf(`{"action":"init","type":"%s","product":"%s","domain":"%s","param":"%s"}`, view.Viewtype, view.Product, view.Region, view.Parameter))
	req, err := http.NewRequest("POST", api_image_url, payload)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("cookie", "PHPSESSID="+sessionId)
	req.Header.Add("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	// Process request
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	var cycleList []string
	json.Unmarshal([]byte(body), &cycleList)
	if err != nil {
		log.Fatal(err)
	}
	res.Body.Close()
	return cycleList
}

func (view *WeatherBellView) selectCycleTime(cycleList []string) string {

	// Find latest cycle time
	for _, cycle := range cycleList {
		cycleNum, err := strconv.ParseInt(cycle, 10, 64)
		if err != nil {
			log.Fatal(err)
		}
		cycleHour := time.Unix(cycleNum, 0).UTC().Hour()
		for _, givenHour := range view.Cyclehours {
			if cycleHour == givenHour {
				return cycle
			}
		}
	}
	log.Fatal("Unable to find matching cycle")
	return ""
}
