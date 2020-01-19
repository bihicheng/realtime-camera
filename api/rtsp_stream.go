package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v2"
	"github.com/vaughan0/go-ini"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"
)

var (
	m                            webrtc.MediaEngine
	api                          *webrtc.API
	channelUserLock              = sync.RWMutex{}
	channelRtspLock              = sync.RWMutex{}
	channelClientLock            = sync.RWMutex{}
	channelRtspPeerConnections   = make(map[string]*webrtc.PeerConnection)
	channelClientPeerConnections = make(map[string][]*webrtc.PeerConnection)
	channelVideoTracks           = make(map[string]*webrtc.Track)
	channelAudioTracks           = make(map[string]*webrtc.Track)
)

const (
	rtcpPLIInterval = time.Second * 3
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func SendOffer(sdp, camId, nvrUrl, nvrHost string) ([]byte, error) {
	val := url.Values{}
	val.Set("data", base64.StdEncoding.EncodeToString([]byte(sdp)))
	val.Set("camId", camId)
	body := strings.NewReader(val.Encode())
	log.Printf("Request Body %v", body)

	req, err := http.NewRequest("POST", nvrUrl, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = nvrHost
	client := http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		log.Println("response body, ", string(respBody))
		return nil, errors.New(string(respBody))
	}
	decodedBody, err := base64.StdEncoding.DecodeString(string(respBody))
	log.Printf("decoded body %v %v", decodedBody, err)
	if err != nil {
		return nil, err
	}
	return decodedBody, nil
}

func RequestRtspStream(camId, url, nvrHost string, sdp []byte, c *websocket.Conn) ([]byte, error) {
	var (
		videoTrackLock = sync.RWMutex{}
		audioTrackLock = sync.RWMutex{}
		videoTrack     *webrtc.Track
		audioTrack     *webrtc.Track
		SendError      error
	)

	file, err := ini.LoadFile("conf.ini")
	checkError(err)
	ip, ok := file.Get("stun", "ip")
	if !ok {
		panic("[stun] ip is missing")
	}
	port, ok := file.Get("stun", "port")
	if !ok {
		panic("[stun] port is missing")
	}
	username, ok := file.Get("stun", "username")
	if !ok {
		panic("[stun] username is missing")
	}
	credential, ok := file.Get("stun", "credential")
	if !ok {
		panic("[stun] credential is missing")
	}

	peerConnectionConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{

				URLs:       []string{fmt.Sprintf("stun:%s:%s", ip, port)},
				Username:   username,
				Credential: credential,
			},
		},
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlanWithFallback,
	}

	channelKey := fmt.Sprintf("%s-%s", nvrHost, camId)

	channelRtspLock.RLock()
	rtspPeer, publishExists := channelRtspPeerConnections[channelKey]
	channelRtspLock.RUnlock()

	if !publishExists || rtspPeer.ConnectionState() == webrtc.PeerConnectionStateDisconnected || rtspPeer.ConnectionState() == webrtc.PeerConnectionStateClosed || rtspPeer.ConnectionState() == webrtc.PeerConnectionStateFailed {
		log.Printf("[NVR]新建当前%s频道连接", channelKey)
		pubReceiver, err := api.NewPeerConnection(peerConnectionConfig)
		if err != nil {
			return nil, err
		}
		_, err = pubReceiver.AddTransceiver(webrtc.RTPCodecTypeAudio)
		if err != nil {
			return nil, err
		}
		_, err = pubReceiver.AddTransceiver(webrtc.RTPCodecTypeVideo)
		if err != nil {
			return nil, err
		}

		pubReceiver.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
			log.Printf("NVR连接状态改变 %s\n", connectionState.String())
			if connectionState == webrtc.ICEConnectionStateClosed || connectionState == webrtc.ICEConnectionStateDisconnected || connectionState == webrtc.ICEConnectionStateFailed {
				log.Printf("NVR失联, 正在删除连接对象\n")
				channelRtspLock.RLock()
				rtspPeer, ok := channelRtspPeerConnections[channelKey]
				channelRtspLock.RUnlock()
				if ok {
					rtspPeer = nil
					log.Println("[NVR] closed", rtspPeer)
					channelRtspLock.Lock()
					delete(channelRtspPeerConnections, channelKey)
					channelRtspLock.Unlock()
				}
				// NVR连接删除后客户端的连接也要删除
				channelClientLock.RLock()
				for i, client := range channelClientPeerConnections[channelKey] {
					log.Println("[CLIENT] %d be closing", i, client)
					client.Close()
				}
				channelClientLock.RUnlock()
				channelClientLock.Lock()
				channelClientPeerConnections[channelKey] = channelClientPeerConnections[channelKey][:0]
				channelClientLock.Unlock()
			}
		})

		pubReceiver.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			log.Printf("[NVR]候选者 %s\n", candidate)
			if candidate == nil {
				answer, err := SendOffer(pubReceiver.LocalDescription().SDP, camId, url, nvrHost)
				if err != nil {
					log.Println("[NVR]", err)
					SendError = err
				} else {
					err = pubReceiver.SetRemoteDescription(
						webrtc.SessionDescription{
							Type: webrtc.SDPTypeAnswer,
							SDP:  string(answer),
						},
					)
					if err != nil {
						log.Println("[NVR]", err)
					} else {
						channelRtspLock.Lock()
						channelRtspPeerConnections[channelKey] = pubReceiver
						channelRtspLock.Unlock()
					}
				}
			}
		})

		// 音频视频
		pubReceiver.OnTrack(func(remoteTrack *webrtc.Track, receiver *webrtc.RTPReceiver) {
			log.Printf("[NVR]incoming track type %s", remoteTrack.PayloadType())

			if remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeVP8 || remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeVP9 || remoteTrack.PayloadType() == webrtc.DefaultPayloadTypeH264 {
				videoTrackLock.Lock()
				videoTrack, err = pubReceiver.NewTrack(remoteTrack.PayloadType(), remoteTrack.SSRC(), "video", "B")
				channelVideoTracks[channelKey] = videoTrack
				videoTrackLock.Unlock()
				if err != nil {
					log.Println("[NVR]", err)
				} else {
					// 发送关键帧
					go func() {
						ticker := time.NewTicker(rtcpPLIInterval)
						defer ticker.Stop()
						for range ticker.C {
							err = pubReceiver.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: videoTrack.SSRC()}})
							if err != nil {
								log.Println("[NVR]ticker so stop", err)
								ticker.Stop()
								return
							}
						}
					}()

					rtpBuf := make([]byte, 1600)
					for {
						i, err := remoteTrack.Read(rtpBuf)
						if err != nil {
							log.Println("[NVR] read video rtp buf ", err)
							break
						}
						videoTrackLock.RLock()
						videoTrack.Write(rtpBuf[:i])
						videoTrackLock.RUnlock()
					}
				}
			} else {
				var err error
				audioTrackLock.Lock()
				audioTrack, err = pubReceiver.NewTrack(remoteTrack.PayloadType(), remoteTrack.SSRC(), "audio", "B")
				channelAudioTracks[channelKey] = audioTrack
				log.Printf("[NVR]audio, payloadtype, %s ssrc, %s", remoteTrack.PayloadType(), remoteTrack.SSRC())
				audioTrackLock.Unlock()
				if err != nil {
					log.Println("[NVR]", err)
				} else {
					rtpBuf := make([]byte, 1600)
					for {
						i, err := remoteTrack.Read(rtpBuf)
						if err != nil {
							log.Println("[NVR] read audio rtp buf ", err)
							break
						}
						audioTrackLock.RLock()
						audioTrack.Write(rtpBuf[:i])
					}
				}
			}
		})

		// 发起请求rtsp流
		offer, err := pubReceiver.CreateOffer(nil)
		if err != nil {
			return nil, err
		}
		err = pubReceiver.SetLocalDescription(offer)
		if err != nil {
			return nil, err
		}
	} else {
		channelClientLock.RLock()
		clients, _ := channelClientPeerConnections[channelKey]
		channelClientLock.RUnlock()
		log.Printf("[CLIENT]订阅当前%s频道人数 %d", channelKey, len(clients))
		videoTrack = channelVideoTracks[channelKey]
		audioTrack = channelAudioTracks[channelKey]
	}

	log.Println("[CLIENT]广播流")
	sender, err := api.NewPeerConnection(peerConnectionConfig)
	if err != nil {
		return nil, err
	}

	sender.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("[CLIENT]连接状态改变 %s\n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateDisconnected {
			log.Printf("[CLIENT]失联, 减少客户端\n")
			channelClientLock.RLock()
			clients, ok := channelClientPeerConnections[channelKey]
			channelClientLock.RUnlock()
			if ok {
				log.Printf("[CLIENT] len=[%d]", len(clients))
				for i, client := range clients {
					log.Println("[CLIENT] ", i)
					if reflect.DeepEqual(client, sender) {
						channelClientLock.Lock()
						channelClientPeerConnections[channelKey] = append(channelClientPeerConnections[channelKey][:i], channelClientPeerConnections[channelKey][i+1:]...)
						channelClientLock.Unlock()
					}
				}
				channelClientLock.RLock()
				clients, _ = channelClientPeerConnections[channelKey]
				channelClientLock.RUnlock()
				log.Printf("[CLIENT]当前%s频道连接数%d", channelKey, len(clients))
				if len(clients) == 0 {
					channelRtspLock.RLock()
					rtspPeer, ok := channelRtspPeerConnections[channelKey]
					channelRtspLock.RUnlock()
					if ok {
						log.Printf("[CLIENT]关闭NVR连接 %s", channelKey)
						rtspPeer.Close()
					}
					log.Println("[CLIENT]清除video track, audio track")
					videoT, ok := channelVideoTracks[channelKey]
					if ok {
						log.Printf("videoT is %v", videoT)
						delete(channelVideoTracks, channelKey)
						videoT = nil
					}
					audioT, ok := channelAudioTracks[channelKey]
					if ok {
						log.Printf("audioT is %v", audioT)
						delete(channelAudioTracks, channelKey)
						audioT = nil
					}
				}
			}
		}
	})

	sender.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		log.Printf("[CLIENT]广播候选者 %s\n", candidate)
		if candidate == nil {
			channelClientLock.Lock()
			channelClientPeerConnections[channelKey] = append(channelClientPeerConnections[channelKey], sender)
			channelClientLock.Unlock()

			channelClientLock.RLock()
			clients, _ := channelClientPeerConnections[channelKey]
			channelClientLock.RUnlock()
			log.Printf("[CLIENT]当前%s频道连接数%d", channelKey, len(clients))
		}
	})

	log.Println("[CLIENT]开始推流", videoTrack)
	for {
		videoTrackLock.RLock()
		if videoTrack == nil {
			videoTrackLock.RUnlock()
			if SendError != nil {
				return nil, SendError
			}
			time.Sleep(100 * time.Millisecond)
		} else {
			videoTrackLock.RUnlock()
			break
		}
	}

	videoTrackLock.RLock()
	if videoTrack != nil {
		_, err = sender.AddTrack(videoTrack)
		if err != nil {
			return nil, err
		}
	}
	videoTrackLock.RUnlock()

	if audioTrack != nil {
		audioTrackLock.RLock()
		_, err = sender.AddTrack(audioTrack)
		audioTrackLock.RUnlock()
		if err != nil {
			return nil, err
		}
	}

	err = sender.SetRemoteDescription(
		webrtc.SessionDescription{
			SDP:  string(sdp),
			Type: webrtc.SDPTypeOffer,
		},
	)
	if err != nil {
		return nil, err
	}

	answer, err := sender.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	err = sender.SetLocalDescription(answer)
	if err != nil {
		return nil, err
	}

	return []byte(answer.SDP), err
}
