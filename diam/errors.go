package diam

import (
	"fmt"
	"strings"
)

// Result-Code values reported by Validate (RFC 6733 §7.1.5).
const (
	ResultAVPUnsupported       uint32 = 5001
	ResultInvalidAVPValue      uint32 = 5004
	ResultMissingAVP           uint32 = 5005
	ResultAVPOccursTooMany     uint32 = 5009
	ResultInvalidAVPLength     uint32 = 5014
	ResultInvalidMessageLength uint32 = 5015
)

// AVPError describes a protocol violation with the Result-Code to answer
// with.
//
// AVP is the offending AVP, ready for a Failed-AVP. For a missing AVP it holds
// the code and vendor with no data. Path lists the codes of the grouped AVPs
// that enclose it, outermost first, so a caller can build a nested Failed-AVP.
type AVPError struct {
	ResultCode uint32
	AVP        RawAVP
	Path       []uint32
	Msg        string
}

func (e *AVPError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diam: %s (result-code %d, avp %d", e.Msg, e.ResultCode, e.AVP.Code)
	if e.AVP.Vendor != 0 {
		fmt.Fprintf(&sb, " vendor %d", e.AVP.Vendor)
	}
	if len(e.Path) > 0 {
		sb.WriteString(" in group")
		for _, c := range e.Path {
			fmt.Fprintf(&sb, " %d", c)
		}
	}
	sb.WriteByte(')')
	return sb.String()
}

// absent builds the placeholder AVP carried by errors about an AVP that is
// not in the message.
func absent(code, vendor uint32) RawAVP {
	a := RawAVP{Code: code, Vendor: vendor}
	if vendor != 0 {
		a.Flags = AVPFlagVendor
	}
	return a
}

// Missing reports a required AVP that is absent (5005).
func Missing(code, vendor uint32, name string) error {
	return &AVPError{ResultCode: ResultMissingAVP, AVP: absent(code, vendor), Msg: "missing required AVP " + name}
}

// TooMany reports an AVP that occurs more often than its grammar allows (5009).
func TooMany(a RawAVP, name string) error {
	return &AVPError{ResultCode: ResultAVPOccursTooMany, AVP: a, Msg: "AVP occurs too many times: " + name}
}

// TooFew reports a repeated AVP under its minimum occurrence (5005).
func TooFew(code, vendor uint32, name string, min int) error {
	return &AVPError{ResultCode: ResultMissingAVP, AVP: absent(code, vendor), Msg: fmt.Sprintf("AVP %s needs at least %d occurrences", name, min)}
}

// Misplaced reports a fixed-position AVP found at another position (5004).
func Misplaced(a RawAVP, name string) error {
	return &AVPError{ResultCode: ResultInvalidAVPValue, AVP: a, Msg: "AVP is not at its fixed position: " + name}
}

// CheckUnknown returns a 5001 error for the first unknown AVP that has the
// M-bit set. Unknown AVPs without the M-bit are ignored (RFC 6733 §4.1).
func CheckUnknown(unknown []RawAVP) error {
	for _, a := range unknown {
		if a.Mandatory() {
			return &AVPError{ResultCode: ResultAVPUnsupported, AVP: a, Msg: "unsupported mandatory AVP"}
		}
	}
	return nil
}

// CheckUnknownError is CheckUnknown for an answer with the E bit. RFC 6733
// §7.2 says such an answer follows the answer-message grammar, not the
// command's, so the AVPs that grammar names (Session-Id, Origin-Host,
// Origin-Realm, Result-Code, Origin-State-Id, Error-Message,
// Error-Reporting-Host, Failed-AVP, Experimental-Result, Proxy-Info) are
// never unknown in it, even when the command's grammar does not list them.
func CheckUnknownError(unknown []RawAVP) error {
	for _, a := range unknown {
		if a.Vendor == 0 && answerMessageAVP(a.Code) {
			continue
		}
		if a.Mandatory() {
			return &AVPError{ResultCode: ResultAVPUnsupported, AVP: a, Msg: "unsupported mandatory AVP"}
		}
	}
	return nil
}

// answerMessageAVP reports whether the base protocol AVP with this code is
// named by the answer-message grammar of RFC 6733 §7.2.
func answerMessageAVP(code uint32) bool {
	switch code {
	case 263, 264, 296, 268, 278, 281, 294, 279, 297, 284:
		return true
	}
	return false
}

// HasAVP reports whether avps holds an AVP with this code and vendor.
func HasAVP(avps []RawAVP, code, vendor uint32) bool {
	for _, a := range avps {
		if a.Code == code && a.Vendor == vendor {
			return true
		}
	}
	return false
}

// InGroup records that err was found inside the grouped AVP with the given
// code. For an *AVPError it returns a copy with code prepended to Path; any
// other error is returned unchanged.
func InGroup(err error, code uint32) error {
	ae, ok := err.(*AVPError)
	if !ok {
		return err
	}
	out := *ae
	out.Path = make([]uint32, 0, len(ae.Path)+1)
	out.Path = append(out.Path, code)
	out.Path = append(out.Path, ae.Path...)
	return &out
}
