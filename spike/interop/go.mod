module github.com/hsdfat/go-diam/spike/interop

go 1.26.0

require (
	github.com/fiorix/go-diameter/v4 v4.6.1-0.20261108172406-f95c2eb4ff52
	github.com/hsdfat/go-diam v0.0.0
)

require github.com/ishidawataru/sctp v0.0.0-20251114114122-19ddcbc6aae2 // indirect

replace github.com/hsdfat/go-diam => ../..
