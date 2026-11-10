package interop

import (
	"os"
	"path/filepath"
	"testing"

	_ "github.com/hsdfat/go-diam/base" // registers the base commands
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// capturesDir holds frames received from real peers: one raw Diameter
// message per .bin file, one directory per application, each with a SOURCE
// file that says where the frames come from.
var capturesDir = filepath.Join("..", "..", "testdata", "captures")

// TestCaptures replays every committed capture: it must decode, pass
// Validate and re-encode to the same AVPs. Equality is semantic (multiset of
// AVPs), because a peer need not send grammar order.
func TestCaptures(t *testing.T) {
	frames, err := filepath.Glob(filepath.Join(capturesDir, "*", "*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		t.Log("no captures committed yet; run the peer workflow and commit its artifact")
		return
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
				t.Fatalf("re-encoded message differs from the capture: %v", err)
			}
			orig, _, _ := diam.ParseHeader(wire)
			if got, _, _ := diam.ParseHeader(out); got != orig {
				t.Fatalf("header = %+v, capture has %+v", got, orig)
			}
		})
	}
	for dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "SOURCE")); err != nil {
			t.Errorf("%s: captures need a SOURCE file naming the peer software, version, date and command", dir)
		}
	}
}
