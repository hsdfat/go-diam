package gentest

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
)

func TestNewAnswerFor(t *testing.T) {
	req := sampleRequest()
	req.Header = diam.Header{
		Version:     1,
		Flags:       diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit,
		CommandCode: 1,
		AppID:       vendor,
		HopByHop:    7,
		EndToEnd:    9,
	}
	req.SessionId = "s"

	ans := NewTestAnswerFor(req)
	want := diam.Header{Version: 1, Flags: diam.FlagProxiable, CommandCode: 1, AppID: vendor, HopByHop: 7, EndToEnd: 9}
	if ans.Header != want {
		t.Fatalf("answer header = %+v, want %+v", ans.Header, want)
	}
	if ans.SessionId != "s" {
		t.Fatalf("SessionId = %q, want it copied from the request", ans.SessionId)
	}
	if ans.OptU32.Set || ans.PlainU32.Set || len(ans.Unknown) != 0 {
		t.Fatalf("answer has more than the header and Session-Id: %+v", ans)
	}

	// The answer is a fresh message: encoding and decoding it works.
	b, err := ans.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := diam.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := m.(*TestAnswer)
	if !ok || got.Header.HopByHop != 7 || got.Header.EndToEnd != 9 || got.SessionId != "s" {
		t.Fatalf("decoded answer = %T %+v", m, m)
	}
}
