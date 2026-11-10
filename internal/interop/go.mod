module github.com/hsdfat/go-diam/internal/interop

go 1.26.0

require (
	github.com/fiorix/go-diameter/v4 v4.6.0
	github.com/hsdfat/go-diam v0.0.0
	github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam v0.0.0
	google.golang.org/protobuf v1.36.12
)

require github.com/ishidawataru/sctp v0.0.0-20251114114122-19ddcbc6aae2 // indirect

replace (
	github.com/hsdfat/go-diam => ../..
	github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam => ../../cmd/protoc-gen-go-diam
)
