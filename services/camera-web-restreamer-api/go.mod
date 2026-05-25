module github.com/w0rxbend/instachron/services/camera-web-restreamer-api

go 1.22.5

require (
	github.com/w0rxbend/instachron/shared/mjpeg v0.0.0
	github.com/w0rxbend/instachron/shared/restream v0.0.0
)

replace (
	github.com/w0rxbend/instachron/shared/cameras  => ../../shared/cameras
	github.com/w0rxbend/instachron/shared/mjpeg    => ../../shared/mjpeg
	github.com/w0rxbend/instachron/shared/restream => ../../shared/restream
)
