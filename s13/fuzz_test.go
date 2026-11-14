package s13

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: both S13 commands, so each
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

// One target per S13/S13' command.

func FuzzECR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(MeIdentityCheckRequest) }, seeds(f)...)
}

func FuzzECA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(MeIdentityCheckAnswer) }, seeds(f)...)
}
