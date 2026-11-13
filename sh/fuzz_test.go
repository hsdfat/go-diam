package sh

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: every Sh command, so each
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

// One target per Sh/Dh command.

func FuzzUDR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(UserDataRequest) }, seeds(f)...)
}

func FuzzUDA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(UserDataAnswer) }, seeds(f)...)
}

func FuzzPUR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ProfileUpdateRequest) }, seeds(f)...)
}

func FuzzPUA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ProfileUpdateAnswer) }, seeds(f)...)
}

func FuzzSNR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SubscribeNotificationsRequest) }, seeds(f)...)
}

func FuzzSNA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SubscribeNotificationsAnswer) }, seeds(f)...)
}

func FuzzPNR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(PushNotificationRequest) }, seeds(f)...)
}

func FuzzPNA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(PushNotificationAnswer) }, seeds(f)...)
}
