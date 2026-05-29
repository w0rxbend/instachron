module github.com/w0rxbend/instachron/services/camera-recorder

go 1.22.5

require (
	github.com/w0rxbend/instachron/shared/cameras v0.0.0
	github.com/w0rxbend/instachron/shared/restream v0.0.0
	github.com/w0rxbend/instachron/shared/streamproto v0.0.0
)

replace (
	github.com/w0rxbend/instachron/shared/cameras => ../../shared/cameras
	github.com/w0rxbend/instachron/shared/restream => ../../shared/restream
	github.com/w0rxbend/instachron/shared/streamproto => ../../shared/streamproto
)
