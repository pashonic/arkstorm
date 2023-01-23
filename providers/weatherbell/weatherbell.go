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

type frame struct {
	url       string
	timeStamp time.Time
}

type Weatherbell struct {
	Views map[string]WeatherBellView
}

type WeatherBellView struct {
	Viewtype            string
	Product             string
	Region              string
	Parameter           string
	Time_label_timezone string
	Time_label_cords    struct {
		X int
		Y int
	}
	Timespanhours int
	Cyclehours    []int
}

func Download(weatherbell *Weatherbell, targetDir string) {

	// Return if empty views
	if len(weatherbell.Views) < 1 {
		return
	}

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

		// Get frame list
		imageList := view.getFrameList(sessionId, selectedCycleTime, view.Timespanhours)

		// Download frames
		targetDir := filepath.Join(targetDir, viewName)
		downloadFrameSet(imageList, view, targetDir)
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

func downloadFrameSet(frameList []frame, view WeatherBellView, targetDir string) {
	for index, frame := range frameList {

		// Create and verify directory path
		err := os.MkdirAll(targetDir, os.ModePerm)
		if err != nil {
			log.Fatalln(err)
		}

		// Send request
		client := http.Client{}
		res, err := client.Get(frame.url)
		if err != nil {
			log.Fatalln(err)
		}

		// Read frame from body.
		body, err := ioutil.ReadAll(res.Body)
		img, _, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			log.Fatalln(err)
		}
		bounds := img.Bounds()
		imgRGBA := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(imgRGBA, imgRGBA.Bounds(), img, bounds.Min, draw.Src)

		// Draw date/time label to frame if specified
		if view.Time_label_cords.X > 0 && view.Time_label_cords.Y > 0 {
			location, err := time.LoadLocation(view.Time_label_timezone)
			if err != nil {
				log.Fatalln(err)
			}
			viewTime := frame.timeStamp.In(location)
			dateTimeString := viewTime.Format("Mon, 2 Jan 3:04 PM MST")
			addLabel(imgRGBA, view.Time_label_cords.X, view.Time_label_cords.Y, dateTimeString)
		}

		// Write final frame to file
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

func (view *WeatherBellView) getFrameList(sessionId string, cycleTimeString string, timeSpanHours int) []frame {

	// Prepare request
	payload := strings.NewReader(fmt.Sprintf(`{"action":"forecast","type":"%s","product":"%s","domain":"%s","param":"%s","init":"%s"}`, view.Viewtype, view.Product, view.Region, view.Parameter, cycleTimeString))
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
	var frameList []string
	json.Unmarshal([]byte(body), &frameList)
	if err != nil {
		log.Fatal(err)
	}
	res.Body.Close()

	// Caclulate max time span
	intVar, err := strconv.ParseInt(cycleTimeString, 10, 64)
	if err != nil {
		log.Fatalln(err)
	}
	cycleTime := time.Unix(intVar, 0)
	maxTimeSpan := cycleTime.Add(time.Duration(timeSpanHours) * time.Hour)

	// Convert to URL list and return
	var frameListReturn []frame
	for _, frameName := range frameList {

		// Create frame url
		url := fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s.png", image_stroage_url, view.Viewtype, view.Product, view.Region, view.Parameter, cycleTimeString, frameName)

		// Get and convert timestamp
		timeStampRegex := regexp.MustCompile(`(\d+)-\w+`)
		timestampString := timeStampRegex.FindStringSubmatch(frameName)[1]
		intVar, err := strconv.ParseInt(timestampString, 10, 64)
		if err != nil {
			log.Fatalln(err)
		}
		viewTime := time.Unix(intVar, 0)

		// If max time/data is less than view, stop adding to list
		if timeSpanHours > 0 && maxTimeSpan.Before(viewTime) {
			break
		}

		// Store object
		frameItem := frame{url: url, timeStamp: viewTime}
		frameListReturn = append(frameListReturn, frameItem)
	}
	return frameListReturn
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
