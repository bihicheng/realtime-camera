package api

import (
	"github.com/pion/webrtc/v2"
)

func init() {
	m = webrtc.MediaEngine{}
	m.RegisterDefaultCodecs()
	api = webrtc.NewAPI(webrtc.WithMediaEngine(m))
}
