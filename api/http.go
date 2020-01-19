package api

import (
	"encoding/base64"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/vaughan0/go-ini"
	"log"
	"net/http"
	"time"
	// "net/http/pprof"
)

var (
	upgrader = websocket.Upgrader{}
)

func StartHTTPServer() {
	r := mux.NewRouter()
	r.HandleFunc("/rtcam", HttpRTCamera)
	r.HandleFunc("/ws", WsRTCamera)
	/*
		r.HandleFunc("/debug/pprof/", pprof.Index)
		r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		r.HandleFunc("/debug/pprof/profile", pprof.Profile)
		r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		r.HandleFunc("/debug/pprof/trace", pprof.Trace)
	*/
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("static"))))

	go func() {
		fmt.Println("Runing on 9090")
		srv := &http.Server{
			Addr:         ":9090",
			Handler:      r,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 20 * time.Second,
		}
		err := srv.ListenAndServe()
		if err != nil {
			log.Panicf("%v", err)
		}
	}()
	select {}
}

func HttpRTCamera(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	data := r.FormValue("data")
	camId := r.FormValue("camId")
	nvrId := r.FormValue("nvrId")
	sdp, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("[REQUEST]NVRID ", nvrId)
	log.Println("[REQUEST]CAMID ", camId)

	file, err := ini.LoadFile("conf.ini")
	checkError(err)
	nvrApi, ok := file.Get("nvr", "api")
	if !ok {
		panic("[REQUEST] api is missing")
	}
	subdomain, ok := file.Get("nvr", "subdomain")
	if !ok {
		panic("[REQUEST] subdomain is missing")
	}
	var (
		url     = nvrApi
		nvrHost = fmt.Sprintf("%s.%s", nvrId, subdomain)
	)
	answer, err := RequestRtspStream(camId, url, nvrHost, sdp, nil)
	if err != nil {
		w.Write([]byte(err.Error()))
	} else {
		w.Write([]byte(base64.StdEncoding.EncodeToString(answer)))
	}
}

func WsRTCamera(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	checkError(err)

	defer func() {
		checkError(c.Close())
	}()

	// mt, msg, err := c.ReadMessage()
	//checkError(err)
	// support websocket
	// answer := RequestRtspStream(camId, url, nvrHost, sdp, c)
	// checkError(c.WriteMessage(mt, answer))
}
