module github.com/w0rxbend/instachron/services/camera-web-restream-enhancer-api

go 1.22.5

require (
	github.com/disintegration/imaging v1.6.2
	github.com/w0rxbend/instachron/shared/mjpeg v0.0.0
	github.com/w0rxbend/instachron/shared/restream v0.0.0
	github.com/w0rxbend/instachron/shared/streamproto v0.0.0
	github.com/w0rxbend/instachron/shared/webui v0.0.0
)

require (
	github.com/w0rxbend/instachron/shared/cameras v0.0.0 // indirect
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
)

replace (
	github.com/w0rxbend/instachron/shared/cameras => ../../shared/cameras
	github.com/w0rxbend/instachron/shared/mjpeg => ../../shared/mjpeg
	github.com/w0rxbend/instachron/shared/restream => ../../shared/restream
	github.com/w0rxbend/instachron/shared/streamproto => ../../shared/streamproto
	github.com/w0rxbend/instachron/shared/webui => ../../shared/webui
)
