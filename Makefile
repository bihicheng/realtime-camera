export PATH := $(GOPATH)/bin:$(PATH)

all: fmt build

build: rtcamera

fmt:
	go fmt ./...
	
rtcamera:
	go build -o bin/rtcamera ./cmd/rtcamera

test: gotest

clean:
	rm -f ./bin/rtcamera

run:
	./bin/rtcamera

dep:
	supervisorctl stop ipcamera-webrtc:*
	cp -r ./bin /data/w/ais/app/ipcamera-webrtc
	supervisorctl start ipcamera-webrtc:*
