package main

import "github.com/bihicheng/realtime-camera/api"


func Execute() {
	runServer()
}

func runServer() {
	api.StartHTTPServer()
}
