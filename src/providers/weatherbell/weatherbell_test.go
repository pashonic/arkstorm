package weatherbell

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/pashonic/arkstorm/src/utils/mockclient"
	"github.com/pashonic/arkstorm/src/utils/restclient"
	"github.com/stretchr/testify/assert"
)

func init() {
	restclient.Client = &mockclient.MockClient{}
}

func TestGetSessionId(t *testing.T) {

	// Test Valid
	mockclient.GetDoFunc = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte("{}"))),
			Header: http.Header{
				"Set-Cookie": {
					"PHPSESSID=a3fd3b61d7db6d652d2c588bcd0b57a3; expires=Mon, 06-Feb-2023 06:02:36 GMT; Max-Age=604800; path=/; HTTPOnly; Secure; domain=.weatherbell.com; HttpOnly",
					"PHPSESSID=a3fd3b61d7db6d652d2c588bcd0b57a3; expires=Mon, 06-Feb-2023 06:02:36 GMT; Max-Age=604800; path=/; HTTPOnly; Secure; domain=.weatherbell.com; HttpOnly",
					"cookie=d1c9161b03480652bed0b3631515246b29e1aba29f2837d0b3d8eb9093822293; expires=Tue, 19-Jan-2038 03:14:07 GMT; Max-Age=472425091; path=/; HTTPOnly; Secure; domain=.weatherbell.com",
					"userid=lZ2cmD%2FLVNlB1ZcGVeDq3cVaQdI%2BsBHYtd00RFaf800%3D; expires=Tue, 30-Jan-2024 06:02:36 GMT; Max-Age=31536000; path=/; HTTPOnly; Secure; domain=.weatherbell.com; HttpOnly",
				},
			},
		}, nil
	}
	sessionId, err := getSessionId("username", "password")
	assert.EqualValues(t, "a3fd3b61d7db6d652d2c588bcd0b57a3", sessionId)
	assert.Nil(t, err)

	// Test Invalid
	mockclient.GetDoFunc = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte("{}"))),
			Header: http.Header{
				"Set-Cookie": {
					"PHPSESSID=a3fd3b61d7db6d652d2c588bcd0b57a3; expires=Mon, 06-Feb-2023 06:02:36 GMT; Max-Age=604800; path=/; HTTPOnly; Secure; domain=.weatherbell.com; HttpOnly",
				},
			},
		}, nil
	}
	sessionId, err = getSessionId("username", "password")
	assert.NotNil(t, err)
	assert.EqualValues(t, "", sessionId)

}

func TestGetCycleList(t *testing.T) {

	// Test valid data
	mockclient.GetDoFunc = func(*http.Request) (*http.Response, error) {
		data := "[\"1675188000\",\"1675166400\",\"1675144800\",\"1675123200\",\"1675101600\"]"
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(data))),
			Header:     http.Header{},
		}, nil
	}
	view := WeatherBellView{}
	cycleList, err := view.getCycleList("")
	expectedList := []string{"1675188000", "1675166400", "1675144800", "1675123200", "1675101600"}
	if !reflect.DeepEqual(expectedList, cycleList) {
		t.Fatalf("CycleList didn't match")
	}
	assert.Nil(t, err)
}

func TestGetFrameList(t *testing.T) {

	// Setup full data monk
	fullList := `["1675447200-6BSj9Y0w2Ao","1675450800-GWr3z89zNEI","1675454400-mHFUY0aM3Yo","1675458000-vQq3LAkHT0s","1675461600-qVs2eyClgvk","1675465200-DyQjhuaUrj8","1675468800-Y8yMO8NgP1o","1675472400-gahM0WGU5rA","1675476000-iwu6QO63hZc","1675479600-t2OrMHWuUrY","1675483200-XnVidmm2fJg","1675486800-ECr9zKUHSkQ","1675490400-EviQq3ByYkY","1675494000-TLZaaeqfoYM","1675497600-8I9EM56y5I4","1675501200-C0G8ZTzhr88","1675504800-reB7Bo9goZg","1675508400-79tbmtSHrhU","1675512000-uJyDi9Lh5Z4","1675515600-TiauRttwMCw","1675519200-KibQy45jI2M","1675522800-Ba14KZIum8M","1675526400-6k7iFg4XffI","1675530000-5LpKJ5p8hPM","1675533600-hdZeksxlaFU","1675537200-tBTafzZLEl8","1675540800-SQOt43Stn30","1675544400-uMjputS7N04","1675548000-4M9kk4hYigo","1675551600-qXmCfgjCbgA","1675555200-cgOrQLVFkCY","1675558800-TmMxK0zC4kg","1675562400-SoqtrfVl02U","1675566000-dD4gdP6GUXE","1675569600-diAbjSRdt00","1675573200-anC7XPPOeNE","1675576800-4YnUCTjJNnc","1675580400-7tJ9sUTLuVc","1675584000-eObbCFhPmBY","1675587600-B8mk6l0zWYE","1675591200-BbaHQUzoc7w","1675594800-gPqcs5gTyQc","1675598400-NZrPTiPmrVU","1675602000-cUYmgxPjpq0","1675605600-YiEHGxJg43M","1675609200-uHCM9YIhJTI","1675612800-nCVN29uZs1Q","1675616400-ohoVBLURasc","1675620000-scsHzkupERs","1675623600-bOb7O9htFEs","1675627200-P5dS3Z0dyZA","1675630800-GFcSVBvXovw","1675634400-hXjY0FHApKQ","1675638000-Bb9e79UrQYg","1675641600-780fbdL98eM","1675645200-nEXjSKWuG8M","1675648800-dNMRVx9vR3Q","1675652400-R1nE8d126r8","1675656000-Q3ayGzBsBt8","1675659600-1iInldKzRUU","1675663200-kb4yq32aCZk","1675666800-miEtelm7pog","1675670400-NVVMiiBEAWM","1675674000-XupejHmUCFE","1675677600-DryP6XOW0OE","1675681200-YgY2w4xQcl0","1675684800-tAlXqxd50Wo","1675688400-fnn2KjKnhEA","1675692000-iSXGq2dg1ss","1675695600-djVJopaDQT4","1675699200-cGiL5g7ncuE","1675702800-d92jIWebqk8","1675706400-DzNCNa1ET2E","1675710000-YYu5XVBadzo","1675713600-RHI9pTxrFdw","1675717200-zZeUtVLhZGY","1675720800-olIEBnr0KdI","1675724400-yQHQA3KyCio","1675728000-mBPe3iUE1LQ","1675731600-mn8smH17G0g","1675735200-H5GyBkmW3zs","1675738800-ihDP4sSOXxc","1675742400-Qqe7JGCEowE","1675746000-htRiKIwVEG8","1675749600-7BGW6QjRe3s","1675753200-DnhWWqESbEM","1675756800-OGMLiTfI85o","1675760400-oIgR4q5PyAM","1675764000-tZ07IhbuZc8","1675767600-Sg8NpTGCQ5E","1675771200-H8tR1BPddtY"]`
	mockclient.GetDoFunc = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(fullList))),
			Header:     http.Header{},
		}, nil
	}
	view := WeatherBellView{
		Viewtype:  "dummy1",
		Product:   "dummy2",
		Region:    "dummy3",
		Parameter: "dummy4",
	}

	// Test all frames minus 1
	cycleList, err := view.getFrameList("", "1675447200", 89)
	assert.EqualValues(t, 90, len(cycleList))
	assert.EqualValues(t, fmt.Sprintf("%s/dummy1/dummy2/dummy3/dummy4/1675447200/1675447200-6BSj9Y0w2Ao.png", image_stroage_url), cycleList[0].url)
	assert.Nil(t, err)

	// Test all frames
	cycleList, err = view.getFrameList("", "1675447200", 90)
	assert.EqualValues(t, fmt.Sprintf("%s/dummy1/dummy2/dummy3/dummy4/1675447200/1675483200-XnVidmm2fJg.png", image_stroage_url), cycleList[10].url)
	assert.EqualValues(t, 91, len(cycleList))

	// Test frames returned with 0 timespan value
	cycleList, err = view.getFrameList("", "1675447200", 0)
	assert.Nil(t, err)
	assert.EqualValues(t, 2, len(cycleList))

	// Setup full data monk
	emptyList := `[]`
	mockclient.GetDoFunc = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(emptyList))),
			Header:     http.Header{},
		}, nil
	}
	cycleList, err = view.getFrameList("", "1675447200", 89)
	assert.EqualValues(t, 0, len(cycleList))
	assert.Nil(t, err)
}

func TestSelectLatestCycleTime(t *testing.T) {

	// Test varies valid cycle hours
	view := WeatherBellView{
		Cyclehours: []int{0, 12},
	}
	cycleList := []string{"1675447200", "1675425600", "1675404000", "1675382400"}
	cycleTime, err := view.selectLatestCycleTime(cycleList)
	assert.EqualValues(t, "1675425600", cycleTime)
	assert.Nil(t, err)
	view = WeatherBellView{
		Cyclehours: []int{6},
	}
	cycleTime, err = view.selectLatestCycleTime(cycleList)
	assert.EqualValues(t, "1675404000", cycleTime)
	assert.Nil(t, err)
	view = WeatherBellView{
		Cyclehours: []int{18},
	}
	cycleTime, err = view.selectLatestCycleTime(cycleList)
	assert.EqualValues(t, "1675447200", cycleTime)
	assert.Nil(t, err)

	// Test invalid cycle hour
	view = WeatherBellView{
		Cyclehours: []int{10},
	}
	cycleTime, err = view.selectLatestCycleTime(cycleList)
	assert.EqualValues(t, "", cycleTime)
	assert.NotNil(t, err)

	// Test emtpy cycle list
	cycleList = []string{}
	view = WeatherBellView{
		Cyclehours: []int{18},
	}
	cycleTime, err = view.selectLatestCycleTime(cycleList)
	assert.EqualValues(t, "", cycleTime)
	assert.NotNil(t, err)

	// Test garbage cycle list data
	cycleList = []string{"acv", "asdf", "gsdf0", "24fs"}
	view = WeatherBellView{
		Cyclehours: []int{18},
	}
	cycleTime, err = view.selectLatestCycleTime(cycleList)
	assert.EqualValues(t, "", cycleTime)
	assert.NotNil(t, err)
}

func TestDownloadFrameSet(t *testing.T) {

	// Prep for tests
	inputFileData, err := os.ReadFile("testdata/input.png")
	assert.Nil(t, err)
	mockclient.GetDoFunc = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte(inputFileData))),
			Header:     http.Header{},
		}, nil
	}
	frame := frame{
		url:       "",
		timeStamp: time.Unix(1675574734, 0),
	}

	// Test frame download and adding label
	view := WeatherBellView{
		Time_label_timezone: "America/Los_Angeles",
		Time_label_cords:    Time_label_cords{X: 200, Y: 200},
	}
	err = downloadFrame(0, frame, view, "testdata/")
	assert.Nil(t, err)
	expectedFileData, err := os.ReadFile("testdata/000_expected.png")
	assert.Nil(t, err)
	actualFileData, err := os.ReadFile("testdata/000.png")
	assert.Nil(t, err)
	assert.Nil(t, os.Remove("testdata/000.png"))
	if bytes.Compare(expectedFileData, actualFileData) != 0 {
		t.Fatalf("Actual label image didn't match expected")
	}

	// Test frame download without adding label
	view = WeatherBellView{}
	err = downloadFrame(1, frame, view, "testdata/")
	assert.Nil(t, err)
	expectedFileData, err = os.ReadFile("testdata/001_expected.png")
	assert.Nil(t, err)
	actualFileData, err = os.ReadFile("testdata/001.png")
	assert.Nil(t, err)
	assert.Nil(t, os.Remove("testdata/001.png"))
	if bytes.Compare(expectedFileData, actualFileData) != 0 {
		t.Fatalf("Actual non-label image didn't match expected")
	}

}
