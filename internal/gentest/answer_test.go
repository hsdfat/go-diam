package gentest

import (
	"errors"
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
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

// errorAnswer encodes a Test-Answer with the given header flags and AVPs.
func errorAnswer(t *testing.T, flags uint8, avps ...diam.RawAVP) []byte {
	t.Helper()
	b := diam.AppendHeader(nil, &diam.Header{Version: 1, Flags: flags, CommandCode: 1, AppID: vendor, HopByHop: 1, EndToEnd: 1})
	for _, a := range avps {
		b = diam.AppendRaw(b, a)
	}
	b, err := diam.FinishMessage(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// An answer with the E bit follows the answer-message grammar of RFC 6733
// §7.2 instead of its command's: Origin-Host, Origin-Realm and Result-Code
// are required there, and the AVPs it names are not unknown even when the
// command does not list them.
func TestErrorAnswerGrammar(t *testing.T) {
	m := diam.AVPFlagMandatory
	host := diam.RawAVP{Code: 264, Flags: m, Data: []byte("hss.test")}
	realm := diam.RawAVP{Code: 296, Flags: m, Data: []byte("test")}
	result := diam.RawAVP{Code: 268, Flags: m, Data: diam.AppendUnsigned32(nil, 3001)}
	message := diam.RawAVP{Code: 281, Data: []byte("DIAMETER_COMMAND_UNSUPPORTED")}
	stateID := diam.RawAVP{Code: 278, Flags: m, Data: diam.AppendUnsigned32(nil, 7)}
	sid := diam.RawAVP{Code: 40, Vendor: vendor, Flags: diam.AVPFlagVendor, Data: []byte("s")}

	var a TestAnswer
	if err := a.Unmarshal(errorAnswer(t, diam.FlagProxiable|diam.FlagError, host, realm, result, message, stateID)); err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("error answer: %v", err)
	}

	// Without the E bit the same AVPs are unknown to the command's grammar.
	if err := a.Unmarshal(errorAnswer(t, diam.FlagProxiable, sid, host, realm, result, message, stateID)); err != nil {
		t.Fatal(err)
	}
	var ae *diam.AVPError
	if err := a.Validate(); !errors.As(err, &ae) || ae.ResultCode != diam.ResultAVPUnsupported || ae.AVP.Code != 264 {
		t.Fatalf("answer without the E bit: %v, want 5001 for Origin-Host", err)
	}

	// The answer-message grammar's own required AVPs are checked.
	for _, tc := range []struct {
		avps []diam.RawAVP
		code uint32
	}{
		{[]diam.RawAVP{realm, result}, 264},
		{[]diam.RawAVP{host, result}, 296},
		{[]diam.RawAVP{host, realm, message}, 268},
	} {
		if err := a.Unmarshal(errorAnswer(t, diam.FlagError, tc.avps...)); err != nil {
			t.Fatal(err)
		}
		if err := a.Validate(); !errors.As(err, &ae) || ae.ResultCode != diam.ResultMissingAVP || ae.AVP.Code != tc.code {
			t.Fatalf("error answer without AVP %d: %v", tc.code, err)
		}
	}

	// An AVP under *[ AVP ] still follows the M-bit rule.
	other := diam.RawAVP{Code: 9999, Flags: m}
	if err := a.Unmarshal(errorAnswer(t, diam.FlagError, host, realm, result, other)); err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(); !errors.As(err, &ae) || ae.ResultCode != diam.ResultAVPUnsupported || ae.AVP.Code != 9999 {
		t.Fatalf("unknown M-bit AVP in an error answer: %v", err)
	}
}

// Re-encoding a decoded error answer writes only the AVPs it had: the
// command's required AVPs are not part of the answer-message grammar, so a
// missing one is not filled in with its zero value. Without the E bit a
// required AVP is always written.
func TestErrorAnswerRoundTrip(t *testing.T) {
	m := diam.AVPFlagMandatory
	in := errorAnswer(t, diam.FlagProxiable|diam.FlagError,
		diam.RawAVP{Code: 264, Flags: m, Data: []byte("hss.test")},
		diam.RawAVP{Code: 296, Flags: m, Data: []byte("test")},
		diam.RawAVP{Code: 268, Flags: m, Data: diam.AppendUnsigned32(nil, 3001)})
	var a TestAnswer
	if err := a.Unmarshal(in); err != nil {
		t.Fatal(err)
	}
	out, err := a.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := diamtest.SameAVPs(in, out); err != nil {
		t.Fatalf("error answer re-encoded with other AVPs: %v", err)
	}

	a.Header.Flags &^= diam.FlagError
	out, err = a.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, avps, _ := diam.ParseHeader(out); len(avps) == 0 {
		t.Fatal("no AVPs")
	} else if first, _, _ := diam.NextAVP(avps); first.Code != 40 || first.Vendor != vendor {
		t.Fatalf("answer without the E bit: first AVP %d/%d, want the required Session-Id", first.Code, first.Vendor)
	}
}
