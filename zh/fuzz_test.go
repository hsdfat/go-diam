package zh

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: both Zh commands, so each
// target also sees a well-formed message of the other command.
func seeds(f *testing.F) [][]byte {
	f.Helper()
	var out [][]byte
	for _, m := range samples() {
		b, err := m.AppendMarshal(nil)
		if err != nil {
			f.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// One target per Zh command.

func FuzzMAR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(MultimediaAuthRequest) }, seeds(f)...)
}

func FuzzMAA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(MultimediaAuthAnswer) }, seeds(f)...)
}
