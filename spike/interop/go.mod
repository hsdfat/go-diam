module github.com/hsdfat/go-diam/spike/interop

go 1.25.0

require (
	github.com/fiorix/go-diameter/v4 v4.0.0
	github.com/hsdfat/go-diam v0.0.0
)

require github.com/ishidawataru/sctp v0.0.0-20251114114122-19ddcbc6aae2 // indirect

replace (
	github.com/fiorix/go-diameter/v4 => ../../../go-diameter-169
	github.com/hsdfat/go-diam => ../..
)
