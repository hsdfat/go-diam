package swx

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: every SWx command, so each
// target also sees well-formed messages of the other commands.
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

// One target per SWx command.

func FuzzMAR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(MultimediaAuthRequest) }, seeds(f)...)
}

func FuzzMAA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(MultimediaAuthAnswer) }, seeds(f)...)
}

func FuzzPPR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(PushProfileRequest) }, seeds(f)...)
}

func FuzzPPA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(PushProfileAnswer) }, seeds(f)...)
}

func FuzzSAR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ServerAssignmentRequest) }, seeds(f)...)
}

func FuzzSAA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ServerAssignmentAnswer) }, seeds(f)...)
}

func FuzzRTR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(RegistrationTerminationRequest) }, seeds(f)...)
}

func FuzzRTA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(RegistrationTerminationAnswer) }, seeds(f)...)
}
