package diam

import "testing"

// FuzzNextAVP splits arbitrary bytes into AVPs: it must never panic, never
// consume more than it was given, and always make progress.
func FuzzNextAVP(f *testing.F) {
	f.Add([]byte{0, 0, 1, 8, 0x40, 0, 0, 12, 'h', 'o', 's', 't'})
	f.Add([]byte{0, 0, 5, 0x7f, 0xc0, 0, 0, 15, 0, 0, 0x28, 0xaf, 1, 2, 3, 0})
	f.Add([]byte{0, 0, 0, 1, 0, 0, 0, 9, 'A'})
	f.Add([]byte{0, 0, 0, 1, 0x80, 0, 0, 8})
	f.Add([]byte{0, 0, 0, 1, 0, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		rest := data
		for len(rest) > 0 {
			a, next, err := NextAVP(rest)
			if err != nil {
				return
			}
			if len(next) >= len(rest) {
				t.Fatalf("no progress: %d bytes left before, %d after", len(rest), len(next))
			}
			consumed := len(rest) - len(next)
			if hl := consumed - len(a.Data); hl < 8 {
				t.Fatalf("AVP of %d bytes has %d data bytes", consumed, len(a.Data))
			}
			// Re-encoding what was parsed gives the same AVP back.
			b, tail, err := NextAVP(AppendRaw(nil, a))
			if err != nil || len(tail) != 0 || b.Code != a.Code || b.Vendor != a.Vendor || b.Flags != a.Flags || string(b.Data) != string(a.Data) {
				t.Fatalf("AppendRaw round trip: %+v -> %+v (%v)", a, b, err)
			}
			rest = next
		}
	})
}
