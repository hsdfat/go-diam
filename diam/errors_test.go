package diam

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestHelperResultCodes(t *testing.T) {
	a := RawAVP{Code: 264, Flags: AVPFlagMandatory, Data: []byte("h")}

	ae := wantResult(t, Missing(296, 0, "ORIGIN_REALM"), ResultMissingAVP)
	if ae.AVP.Code != 296 || ae.AVP.Vendor != 0 || !strings.Contains(ae.Msg, "ORIGIN_REALM") {
		t.Fatalf("Missing = %+v", ae)
	}
	ae = wantResult(t, Missing(1407, 10415, "VISITED_PLMN_ID"), ResultMissingAVP)
	if ae.AVP.Vendor != 10415 || ae.AVP.Flags&AVPFlagVendor == 0 {
		t.Fatalf("Missing with vendor must carry the vendor and V-bit: %+v", ae.AVP)
	}

	ae = wantResult(t, TooMany(a, "ORIGIN_HOST"), ResultAVPOccursTooMany)
	if ae.AVP.Code != 264 || string(ae.AVP.Data) != "h" {
		t.Fatalf("TooMany = %+v", ae)
	}

	ae = wantResult(t, TooFew(257, 0, "HOST_IP_ADDRESS", 1), ResultMissingAVP)
	if ae.AVP.Code != 257 || !strings.Contains(ae.Msg, "at least 1") {
		t.Fatalf("TooFew = %+v", ae)
	}

	ae = wantResult(t, Misplaced(a, "SESSION_ID"), ResultInvalidAVPValue)
	if ae.AVP.Code != 264 {
		t.Fatalf("Misplaced = %+v", ae)
	}
}

func TestInGroupNesting(t *testing.T) {
	a := RawAVP{Code: 1402, Vendor: 10415}
	err := InGroup(InGroup(TooMany(a, "x"), 1401), 1400)
	ae := wantResult(t, err, ResultAVPOccursTooMany)
	if !reflect.DeepEqual(ae.Path, []uint32{1400, 1401}) {
		t.Fatalf("Path = %v, want [1400 1401]", ae.Path)
	}
	if ae.AVP.Code != 1402 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	if msg := ae.Error(); !strings.Contains(msg, "5009") || !strings.Contains(msg, "1402") || !strings.Contains(msg, "1400") || !strings.Contains(msg, "1401") {
		t.Fatalf("Error() = %q, want result code, AVP code and path", msg)
	}

	// InGroup must not modify the error it wraps.
	inner := TooMany(a, "x")
	_ = InGroup(inner, 7)
	if p := inner.(*AVPError).Path; len(p) != 0 {
		t.Fatalf("inner error path changed to %v", p)
	}

	plain := errors.New("boom")
	if got := InGroup(plain, 1400); got != plain {
		t.Fatalf("non-AVPError = %v, want it unchanged", got)
	}
	if got := InGroup(nil, 1400); got != nil {
		t.Fatalf("nil = %v", got)
	}
}

func TestCheckUnknown(t *testing.T) {
	if err := CheckUnknown(nil); err != nil {
		t.Fatal(err)
	}
	if err := CheckUnknown([]RawAVP{{Code: 9999, Vendor: 12345}}); err != nil {
		t.Fatalf("M-clear unknown AVP: %v", err)
	}
	err := CheckUnknown([]RawAVP{
		{Code: 9999},
		{Code: 9998, Flags: AVPFlagMandatory, Data: []byte{1}},
		{Code: 9997, Flags: AVPFlagMandatory},
	})
	ae := wantResult(t, err, ResultAVPUnsupported)
	if ae.AVP.Code != 9998 || len(ae.AVP.Data) != 1 {
		t.Fatalf("AVP = %+v, want the first M-set unknown AVP", ae.AVP)
	}
}
