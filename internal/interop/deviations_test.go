package interop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// deviationsDir holds frames received from real peers that deviate from the
// specification in a way go-diam does not follow, one directory per
// application with a SOURCE file, like capturesDir. docs/design.md §14
// records each deviation with the clause that settles it.
var deviationsDir = filepath.Join("..", "..", "testdata", "deviations")

// TestDeviations replays the frames of reviewed peer deviations. Each is an
// answer with the E bit whose P bit the peer cleared although the request
// had it set: RFC 6733 §6.2 and §7.2 give an answer the P bit of its
// request, and go-diam encodes the grammar's PXY. Everything else must hold
// as for a capture: the frame decodes, passes Validate (against the
// answer-message grammar of RFC 6733 §7.2) and re-encodes to the same AVPs.
// The test fails if the frame stops deviating, so the record cannot rot.
func TestDeviations(t *testing.T) {
	frames, err := filepath.Glob(filepath.Join(deviationsDir, "*", "*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Fatal("no deviation frames found")
	}
	dirs := map[string]bool{}
	for _, path := range frames {
		dirs[filepath.Dir(path)] = true
		t.Run(filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)), func(t *testing.T) {
			wire, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			m, err := diam.Decode(wire)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			out, err := m.AppendMarshal(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := diamtest.SameAVPs(wire, out); err != nil {
				t.Fatalf("re-encoded message differs from the frame: %v", err)
			}
			got, _, _ := diam.ParseHeader(out)
			orig, _, _ := diam.ParseHeader(wire)
			if orig.Flags&diam.FlagError == 0 || orig.Flags&diam.FlagProxiable != 0 || got.Flags&diam.FlagProxiable == 0 {
				t.Fatalf("frame flags %#x, re-encoded %#x: not an E-bit answer with the P bit cleared against a PXY grammar", orig.Flags, got.Flags)
			}
			got.Flags &^= diam.FlagProxiable
			if got != orig {
				t.Fatalf("header = %+v, frame has %+v: differs beyond the P bit", got, orig)
			}
		})
	}
	for dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "SOURCE")); err != nil {
			t.Errorf("%s: deviation frames need a SOURCE file naming the peer software, version, date and command", dir)
		}
	}
}
