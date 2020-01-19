#### 背景
rtmp方案的直播延迟在2-5秒间，需要较低延迟，希望能做到300毫秒。

#### realtime-camera原理
 
* [详解P2P技术中的NAT穿透原理](https://www.jianshu.com/p/f71707892eb2)

* [webrtc原理图](https://www.processon.com/view/link/5d909667e4b021bb66511840)



#### WEBRTC-STUN
[STUN/TURN/ICE协议在P2P SIP中的应用（一）](https://www.cnblogs.com/ishang/p/3810382.html)

[STUN/TURN/ICE协议在P2P SIP中的应用（一）](https://www.cnblogs.com/ishangs/p/3816689.html)

[NAT 简介分类作用](https://blog.csdn.net/qq_16095853/article/details/77743320)

#### 部署方式
```
mkdir $GOPATH/src/github.com/bihicheng/
cd $GOPATH/src/github.com/bihicheng/
git clone git@github.com:bihicheng/realtime-camera.git
cd realtime-camera && make  
supervisorctl restart rtcamera
```

