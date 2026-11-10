module github.com/hsdfat/go-diam/internal/interop

go 1.23

require (
	github.com/fiorix/go-diameter/v4 v4.6.0
	github.com/hsdfat/go-diam v0.0.0
	github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam v0.0.0
	google.golang.org/protobuf v1.36.12
)

replace (
	github.com/hsdfat/go-diam => ../..
	github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam => ../../cmd/protoc-gen-go-diam
)
