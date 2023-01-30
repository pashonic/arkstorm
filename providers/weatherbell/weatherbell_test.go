package weatherbell

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/pashonic/arkstorm/utils/mockclient"
	"github.com/pashonic/arkstorm/utils/restclient"
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
	sessionId, _ := getSessionId()
	assert.EqualValues(t, "a3fd3b61d7db6d652d2c588bcd0b57a3", sessionId)

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
	sessionId, err := getSessionId()
	assert.NotNil(t, err)

}
