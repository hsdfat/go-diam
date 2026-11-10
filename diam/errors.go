package diam

import "fmt"

// Result-Code values reported by Validate (RFC 6733 §7.1.5).
const (
	ResultAVPUnsupported   uint32 = 5001
	ResultInvalidAVPValue  uint32 = 5004
	ResultInvalidAVPLength uint32 = 5014
)

// AVPError describes a protocol violation with the Result-Code to answer
// with. AVP is the offending AVP, ready for a Failed-AVP.
type AVPError struct {
	ResultCode uint32
	AVP        RawAVP
	Msg        string
}

func (e *AVPError) Error() string {
	return fmt.Sprintf("diam: %s (result-code %d, avp %d vendor %d)", e.Msg, e.ResultCode, e.AVP.Code, e.AVP.Vendor)
}
