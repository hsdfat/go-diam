package interop

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6t"
)

// s6tApp is the S6t application id (TS 29.336 §8.1.8).
const s6tApp = 16777345

// go-diameter v4.6.0 ships no S6t application: no 16777345 dictionary and
// no S6t commands. Of the S6t AVPs it defines Supported-Features with
// Feature-List-ID and Feature-List, MSISDN, Charged-Party, User-CSG-Information
// and its members (Ro/Rf, Rx, SWx), Terminal-Information, IMEI,
// Software-Version, 3GPP2-MEID, Visited-PLMN-Id, PDN-Type,
// Served-Party-IP-Address, Subscribed-Periodic-RAU-TAU-Timer, RAT-Type and
// Service-Selection (S6a, S13, SWx, Rx), and the re-used IETF AVPs (DRMP,
// OC-*, Load) under the base protocol and Rx. The S6t interop below is built
// on those, as the Cx and Sh ones are (see cx_test.go).
//
// The AVPs go-diameter has nowhere are listed in s6tRestated with code,
// vendor and M-bit restated from TS 29.336 tables 6.4.1/1, 8.4.1-1 and
// 8.4.1-2 (and the defining specifications the latter points to) here, in
// the test, not read from go-diam's schema. Their value encoding is still
// go-diameter's. TestFiorixS6tGaps keeps the list exact.
var s6tRestated = map[string]restated{
	"EPS-Location-Information":                 {1496, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"MME-Location-Information":                 {1600, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"SGSN-Location-Information":                {1601, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"E-UTRAN-Cell-Global-Identity":             {1602, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Tracking-Area-Identity":                   {1603, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Cell-Global-Identity":                     {1604, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Routing-Area-Identity":                    {1605, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Service-Area-Identity":                    {1607, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Current-Location-Retrieved":               {1610, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Age-Of-Location-Information":              {1611, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"DL-Buffering-Suggested-Packet-Count":      {1674, tgpp, false, "table 8.4.1-2 Must not set"},
	"IMSI-Group-Id":                            {1675, tgpp, false, "table 8.4.1-2 Must not set"},
	"Group-Service-Id":                         {1676, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Group-PLMN-Id":                            {1677, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Local-Group-Id":                           {1678, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Non-IP-PDN-Type-Indicator":                {1681, tgpp, false, "table 8.4.1-2 Must not set"},
	"Non-IP-Data-Delivery-Mechanism":           {1682, tgpp, false, "table 8.4.1-2 Must not set"},
	"eDRX-Cycle-Length":                        {1691, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"eDRX-Cycle-Length-Value":                  {1692, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"User-Identifier":                          {3102, tgpp, true, "table 6.4.1/1"},
	"External-Identifier":                      {3111, tgpp, true, "table 6.4.1/1"},
	"AESE-Communication-Pattern":               {3113, tgpp, true, "table 8.4.1-1"},
	"Communication-Pattern-Set":                {3114, tgpp, true, "table 8.4.1-1"},
	"Periodic-Communication-Indicator":         {3115, tgpp, true, "table 8.4.1-1"},
	"Communication-Duration-Time":              {3116, tgpp, true, "table 8.4.1-1"},
	"Periodic-Time":                            {3117, tgpp, true, "table 8.4.1-1"},
	"Scheduled-Communication-Time":             {3118, tgpp, true, "table 8.4.1-1"},
	"Stationary-Indication":                    {3119, tgpp, true, "table 8.4.1-1"},
	"AESE-Communication-Pattern-Config-Status": {3120, tgpp, true, "table 8.4.1-1"},
	"AESE-Error-Report":                        {3121, tgpp, true, "table 8.4.1-1"},
	"Monitoring-Event-Configuration":           {3122, tgpp, true, "table 8.4.1-1"},
	"Monitoring-Event-Report":                  {3123, tgpp, true, "table 8.4.1-1"},
	"SCEF-Reference-ID":                        {3124, tgpp, true, "table 8.4.1-1"},
	"SCEF-ID":                                  {3125, tgpp, true, "table 8.4.1-1"},
	"SCEF-Reference-ID-for-Deletion":           {3126, tgpp, true, "table 8.4.1-1"},
	"Monitoring-Type":                          {3127, tgpp, true, "table 8.4.1-1"},
	"Maximum-Number-of-Reports":                {3128, tgpp, true, "table 8.4.1-1"},
	"UE-Reachability-Configuration":            {3129, tgpp, true, "table 8.4.1-1"},
	"Monitoring-Duration":                      {3130, tgpp, true, "table 8.4.1-1"},
	"Maximum-Detection-Time":                   {3131, tgpp, true, "table 8.4.1-1"},
	"Reachability-Type":                        {3132, tgpp, true, "table 8.4.1-1"},
	"Maximum-Latency":                          {3133, tgpp, true, "table 8.4.1-1"},
	"Maximum-Response-Time":                    {3134, tgpp, true, "table 8.4.1-1"},
	"Location-Information-Configuration":       {3135, tgpp, true, "table 8.4.1-1"},
	"MONTE-Location-Type":                      {3136, tgpp, true, "table 8.4.1-1"},
	"Accuracy":                                 {3137, tgpp, true, "table 8.4.1-1"},
	"Association-Type":                         {3138, tgpp, true, "table 8.4.1-1"},
	"Roaming-Information":                      {3139, tgpp, true, "table 8.4.1-1"},
	"Reachability-Information":                 {3140, tgpp, true, "table 8.4.1-1"},
	"IMEI-Change":                              {3141, tgpp, true, "table 8.4.1-1"},
	"Monitoring-Event-Config-Status":           {3142, tgpp, true, "table 8.4.1-1"},
	"Supported-Services":                       {3143, tgpp, true, "table 8.4.1-1"},
	"Supported-Monitoring-Events":              {3144, tgpp, true, "table 8.4.1-1"},
	"CIR-Flags":                                {3145, tgpp, true, "table 8.4.1-1"},
	"Service-Result":                           {3146, tgpp, true, "table 8.4.1-1"},
	"Service-Result-Code":                      {3147, tgpp, true, "table 8.4.1-1"},
	"Reference-ID-Validity-Time":               {3148, tgpp, true, "table 8.4.1-1"},
	"Event-Handling":                           {3149, tgpp, true, "table 8.4.1-1"},
	"NIDD-Authorization-Request":               {3150, tgpp, true, "table 8.4.1-1"},
	"NIDD-Authorization-Response":              {3151, tgpp, true, "table 8.4.1-1"},
	"Service-Report":                           {3152, tgpp, true, "table 8.4.1-1"},
	"Node-Type":                                {3153, tgpp, true, "table 8.4.1-1"},
	"S6t-HSS-Cause":                            {3154, tgpp, true, "table 8.4.1-1"},
	"Enhanced-Coverage-Restriction":            {3155, tgpp, false, "table 8.4.1-1"},
	"Enhanced-Coverage-Restriction-Data":       {3156, tgpp, false, "table 8.4.1-1"},
	"Restricted-PLMN-List":                     {3157, tgpp, false, "table 8.4.1-1"},
	"Allowed-PLMN-List":                        {3158, tgpp, false, "table 8.4.1-1"},
	"Requested-Validity-Time":                  {3159, tgpp, false, "table 8.4.1-1"},
	"Granted-Validity-Time":                    {3160, tgpp, false, "table 8.4.1-1"},
	"NIDD-Authorization-Update":                {3161, tgpp, false, "table 8.4.1-1"},
	"Loss-Of-Connectivity-Reason":              {3162, tgpp, false, "table 8.4.1-1"},
	"Group-Reporting-Guard-Timer":              {3163, tgpp, false, "table 8.4.1-1"},
	"CIA-Flags":                                {3164, tgpp, false, "table 8.4.1-1"},
	"Group-Report":                             {3165, tgpp, false, "table 8.4.1-1"},
	"Group-Report-Item":                        {3166, tgpp, false, "table 8.4.1-1"},
	"RIR-Flags":                                {3167, tgpp, false, "table 8.4.1-1"},
	"Type-Of-External-Identifier":              {3168, tgpp, false, "table 8.4.1-1"},
	"APN-Validity-Time":                        {3169, tgpp, false, "table 8.4.1-1"},
	"Suggested-Network-Configuration":          {3170, tgpp, false, "table 8.4.1-1"},
	"Monitoring-Event-Report-Status":           {3171, tgpp, false, "table 8.4.1-1"},
	"PLMN-ID-Requested":                        {3172, tgpp, false, "table 8.4.1-1"},
	"AdditionalIdentifiers":                    {3173, tgpp, false, "table 8.4.1-1"},
	"NIR-Flags":                                {3174, tgpp, false, "table 8.4.1-1"},
	"Reporting-Time-Stamp":                     {3175, tgpp, false, "table 8.4.1-1"},
	"NIA-Flags":                                {3176, tgpp, false, "table 8.4.1-1"},
	"Group-User-Identifier":                    {3177, tgpp, false, "table 8.4.1-1"},
	"MTC-Provider-Info":                        {3178, tgpp, false, "table 8.4.1-1"},
	"MTC-Provider-ID":                          {3179, tgpp, false, "table 8.4.1-1"},
	"PDN-Connectivity-Status-Configuration":    {3180, tgpp, false, "table 8.4.1-1"},
	"PDN-Connectivity-Status-Report":           {3181, tgpp, false, "table 8.4.1-1"},
	"PDN-Connectivity-Status-Type":             {3182, tgpp, false, "table 8.4.1-1"},
	"Traffic-Profile":                          {3183, tgpp, false, "table 8.4.1-1"},
	"Updated-Network-Configuration":            {3184, tgpp, false, "table 8.4.1-1"},
	"Battery-Indicator":                        {3185, tgpp, false, "table 8.4.1-1"},
	"SCEF-Reference-ID-Ext":                    {3186, tgpp, false, "table 8.4.1-1"},
	"SCEF-Reference-ID-for-Deletion-Ext":       {3187, tgpp, false, "table 8.4.1-1"},
	"Exclude-Identifiers":                      {3188, tgpp, false, "table 8.4.1-1"},
	"Include-Identifiers":                      {3189, tgpp, false, "table 8.4.1-1 (code; §8.4.85 writes xxxx)"},
	"Maximum-UE-Availability-Time":             {3329, tgpp, false, "TS 29.338 table 5.3.3.1"},
	"eNodeB-ID":                                {4008, tgpp, true, "TS 29.217 table 5.3.1"},
	"Extended-eNodeB-ID":                       {4013, tgpp, false, "table 8.4.1-2 Must not set"},
	"Number-of-UEs":                            {4209, tgpp, false, "table 8.4.1-2 Must not set; code from TS 29.154 §5.3.10"},
	"Idle-Status-Indication":                   {4322, tgpp, false, "table 8.4.1-2 Must not set"},
	"Idle-Status-Timestamp":                    {4323, tgpp, false, "TS 29.128 table 6.4.1"},
	"Active-Time":                              {4324, tgpp, false, "table 8.4.1-2 Must not set"},
	"Reachability-Cause":                       {4325, tgpp, false, "table 8.4.1-2 Must not set"},
	"Time-Of-Day-Start":                        {561, 0, false, "RFC 5777 §4.2.5, no flag rule"},
	"Time-Of-Day-End":                          {562, 0, false, "RFC 5777 §4.2.6, no flag rule"},
	"Day-Of-Week-Mask":                         {563, 0, false, "RFC 5777 §4.2.7, no flag rule"},
}

// s6tRequestM: Supported-Features carries the M-bit in requests and not in
// answers (TS 29.336 §8.1.9, which applies TS 29.229 §7.2.1).
var s6tRequestM = map[[2]uint32]bool{{avp.SupportedFeatures, tgpp}: true}

// s6tM is the S6t M-bit of AVPs go-diameter defines with another one: table
// 8.4.1-2 says "Must not set" for Served-Party-IP-Address,
// Terminal-Information and PDN-Type, which go-diameter's S6a and SWx
// dictionaries mark M. Everything else about these AVPs is go-diameter's.
// TestFiorixS6tGaps keeps the list exact.
var s6tM = map[[2]uint32]bool{
	{avp.ServedPartyIPAddress, tgpp}: false,
	{avp.TerminalInformation, tgpp}:  false,
	{avp.PDNType, tgpp}:              false,
}

// s6tAVP is perAVP with the S6t M-bit of s6tM.
type s6tAVP struct{ perAVP }

func (c s6tAVP) a(code, vendor uint32, data datatype.Type) *fd.AVP {
	a := c.perAVP.a(code, vendor, data)
	if m, ok := s6tM[[2]uint32{code, vendor}]; ok {
		a.Flags &^= avp.Mbit
		if m {
			a.Flags |= avp.Mbit
		}
	}
	return a
}

func (c s6tAVP) g(code, vendor uint32, avps ...*fd.AVP) *fd.AVP {
	return c.a(code, vendor, &fd.GroupedAVP{AVP: avps})
}

// fs6t builds a go-diameter message of the S6t application.
func fs6t(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, s6tApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// s6tPairs is one message per S6t command, built with go-diam and with
// go-diameter, covering every grouped AVP a command can carry.
func s6tPairs() []pair {
	const (
		scef, hss, realm = "scef.example.org", "hss.example.org", "example.org"
		imsi             = "001010000000001"
		extID            = "device-1@iot.example.org"
		groupID          = "group-1@iot.example.org"
		apn              = "iot.apn"
		dra              = "dra.example.org"
	)
	expiry := time.Date(2026, 12, 13, 8, 0, 0, 0, time.UTC)
	stamp := time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
	msisdn := []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	plmn := []byte{0x00, 0xf1, 0x10}
	nsm := base.AuthSessionStateNoStateMaintained
	q := s6tAVP{perAVP{true, s6tRestated, s6tRequestM}}
	s := s6tAVP{perAVP{false, s6tRestated, s6tRequestM}}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s6tApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, s6tApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: dra, ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, dra), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, dra) }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }
	// DIAMETER_ERROR_UNAUTHORIZED_REQUESTING_ENTITY (§8.3.3.2).
	gExp := &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 5510}
	fExp := func() *fd.AVP {
		return group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 5510))
	}

	gFeatures := []s6t.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0x207}}
	fFeatures := func(b s6tAVP) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(0x207)))
	}
	gOcSF := &s6t.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	fOcSF := func() *fd.AVP {
		return s.g(avp.OCSupportedFeatures, 0, s.a(avp.OCFeatureVector, 0, datatype.Unsigned64(1)))
	}
	gOlr := &s6t.OcOlr{OcSequenceNumber: 9, OcReportType: s6t.OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	fOlr := func() *fd.AVP {
		return s.g(avp.OCOLR, 0, s.a(avp.OCSequenceNumber, 0, datatype.Unsigned64(9)), s.a(avp.OCReportType, 0, datatype.Enumerated(1)),
			s.a(avp.OCReductionPercentage, 0, datatype.Unsigned32(50)), s.a(avp.OCValidityDuration, 0, datatype.Unsigned32(30)))
	}
	// Load-Value is left out: go-diameter types it Unsigned32, RFC 8583
	// §7.3 says Unsigned64 (see dictcheck-allow.txt).
	gLoad := []s6t.Load{{LoadType: diam.Some(s6t.LoadTypeHost), Sourceid: diam.Some(hss)}}
	fLoad := func() *fd.AVP {
		return s.g(avp.Load, 0, s.a(avp.LoadType, 0, datatype.Enumerated(0)), s.a(avp.SourceID, 0, datatype.DiameterIdentity(hss)))
	}

	u := func(b s6tAVP, name string, v uint32) *fd.AVP { return b.r(name, datatype.Unsigned32(v)) }
	u64 := func(b s6tAVP, name string, v uint64) *fd.AVP { return b.r(name, datatype.Unsigned64(v)) }
	tm := func(b s6tAVP, name string, v time.Time) *fd.AVP { return b.r(name, datatype.Time(v)) }
	oct := func(b s6tAVP, name string, v []byte) *fd.AVP { return b.r(name, datatype.OctetString(v)) }
	ext := func(b s6tAVP, v string) *fd.AVP { return b.r("External-Identifier", datatype.UTF8String(v)) }
	scefID := func(b s6tAVP) *fd.AVP { return b.r("SCEF-ID", datatype.DiameterIdentity(scef)) }
	sel := func(b s6tAVP, v string) *fd.AVP { return b.a(avp.ServiceSelection, 0, datatype.UTF8String(v)) }

	gMtc := &s6t.MtcProviderInfo{MtcProviderId: diam.Some("provider-1")}
	fMtc := func(b s6tAVP) *fd.AVP {
		return b.rg("MTC-Provider-Info", b.r("MTC-Provider-ID", datatype.UTF8String("provider-1")))
	}

	gCsg := &s6t.UserCsgInformation{CsgId: 7, CsgAccessMode: s6t.CsgAccessModeHybridMode,
		CsgMembershipIndication: diam.Some(s6t.CsgMembershipIndicationCsgMember)}
	fCsg := func(b s6tAVP) *fd.AVP {
		return b.g(avp.UserCSGInformation, tgpp, b.a(avp.CSGID, tgpp, datatype.Unsigned32(7)),
			b.a(avp.CSGAccessMode, tgpp, datatype.Enumerated(1)), b.a(avp.CSGMembershipIndication, tgpp, datatype.Enumerated(1)))
	}
	ecgi, tai := []byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x02, 0x03}, []byte{0x00, 0xf1, 0x10, 0x00, 0x01}
	gMme := &s6t.EpsLocationInformation{MmeLocationInformation: &s6t.MmeLocationInformation{
		EUtranCellGlobalIdentity: diam.Some(ecgi), TrackingAreaIdentity: diam.Some(tai),
		CurrentLocationRetrieved: diam.Some(s6t.CurrentLocationRetrievedActiveLocationRetrieval),
		AgeOfLocationInformation: diam.Some(uint32(3)), UserCsgInformation: gCsg,
		EnodebId: diam.Some([]byte{0, 1, 2}), ExtendedEnodebId: diam.Some([]byte{0, 1, 2, 3}),
	}}
	fMme := func(b s6tAVP) *fd.AVP {
		return b.rg("EPS-Location-Information", b.rg("MME-Location-Information",
			oct(b, "E-UTRAN-Cell-Global-Identity", ecgi), oct(b, "Tracking-Area-Identity", tai),
			b.r("Current-Location-Retrieved", datatype.Enumerated(0)), u(b, "Age-Of-Location-Information", 3), fCsg(b),
			oct(b, "eNodeB-ID", []byte{0, 1, 2}), oct(b, "Extended-eNodeB-ID", []byte{0, 1, 2, 3})))
	}
	cgi, sai, rai := []byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x00, 0x02}, []byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x00, 0x03}, []byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x04}
	gSgsn := &s6t.EpsLocationInformation{SgsnLocationInformation: &s6t.SgsnLocationInformation{
		CellGlobalIdentity: diam.Some(cgi), ServiceAreaIdentity: diam.Some(sai), RoutingAreaIdentity: diam.Some(rai),
		CurrentLocationRetrieved: diam.Some(s6t.CurrentLocationRetrievedActiveLocationRetrieval),
		AgeOfLocationInformation: diam.Some(uint32(0)), UserCsgInformation: gCsg,
	}}
	fSgsn := func(b s6tAVP) *fd.AVP {
		return b.rg("EPS-Location-Information", b.rg("SGSN-Location-Information",
			oct(b, "Cell-Global-Identity", cgi), oct(b, "Service-Area-Identity", sai), oct(b, "Routing-Area-Identity", rai),
			b.r("Current-Location-Retrieved", datatype.Enumerated(0)), u(b, "Age-Of-Location-Information", 0), fCsg(b)))
	}
	gIdle := &s6t.IdleStatusIndication{
		IdleStatusTimestamp: diam.Some(stamp), ActiveTime: diam.Some(uint32(60)),
		SubscribedPeriodicRauTauTimer:   diam.Some(uint32(3240)),
		EdrxCycleLength:                 &s6t.EdrxCycleLength{RatType: s6t.RatTypeEutranNbIot, EdrxCycleLengthValue: []byte{0x05}},
		DlBufferingSuggestedPacketCount: diam.Some(int32(2)),
	}
	fIdle := func(b s6tAVP) *fd.AVP {
		return b.rg("Idle-Status-Indication", tm(b, "Idle-Status-Timestamp", stamp), u(b, "Active-Time", 60),
			b.a(avp.SubscribedPeriodicRAUTAUTimer, tgpp, datatype.Unsigned32(3240)),
			b.rg("eDRX-Cycle-Length", b.a(avp.RATType, tgpp, datatype.Enumerated(1005)), oct(b, "eDRX-Cycle-Length-Value", []byte{0x05})),
			b.r("DL-Buffering-Suggested-Packet-Count", datatype.Integer32(2)))
	}
	gPdn := []s6t.PdnConnectivityStatusReport{
		{ServiceSelection: apn, PdnConnectivityStatusType: 0, PdnType: diam.Some(s6t.PdnTypeIpv4v6),
			ServedPartyIpAddress: []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::")}},
		{ServiceSelection: "nidd.apn", PdnConnectivityStatusType: 1,
			NonIpPdnTypeIndicator: diam.Some(s6t.NonIpPdnTypeIndicatorTrue), NonIpDataDeliveryMechanism: diam.Some(uint32(1))},
	}
	fPdn := func(b s6tAVP) []*fd.AVP {
		return []*fd.AVP{
			b.rg("PDN-Connectivity-Status-Report", sel(b, apn), u(b, "PDN-Connectivity-Status-Type", 0),
				b.a(avp.PDNType, tgpp, datatype.Enumerated(2)),
				b.a(avp.ServedPartyIPAddress, tgpp, ip("192.0.2.10")), b.a(avp.ServedPartyIPAddress, tgpp, ip("2001:db8::"))),
			b.rg("PDN-Connectivity-Status-Report", sel(b, "nidd.apn"), u(b, "PDN-Connectivity-Status-Type", 1),
				b.r("Non-IP-PDN-Type-Indicator", datatype.Enumerated(1)), u(b, "Non-IP-Data-Delivery-Mechanism", 1)),
		}
	}
	gServiceReport := []s6t.ServiceReport{{
		ServiceResult: &s6t.ServiceResult{VendorId: diam.Some(uint32(tgpp)), ServiceResultCode: diam.Some(uint32(5512))},
		NodeType:      diam.Some(uint32(1)),
	}}
	fServiceReport := func(b s6tAVP) *fd.AVP {
		return b.rg("Service-Report", b.rg("Service-Result", b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			u(b, "Service-Result-Code", 5512)), u(b, "Node-Type", 1))
	}
	gUpdated := &s6t.UpdatedNetworkConfiguration{
		ScefId: scef, ScefReferenceId: diam.Some(uint32(41)), ScefReferenceIdExt: diam.Some(uint64(1) << 40),
		ScefReferenceIdForDeletion: []uint32{40}, ScefReferenceIdForDeletionExt: []uint64{1<<40 - 1},
		SubscribedPeriodicRauTauTimer: diam.Some(uint32(3240)), ActiveTime: diam.Some(uint32(60)),
		DlBufferingSuggestedPacketCount: diam.Some(int32(-1)),
	}
	fUpdated := func(b s6tAVP) *fd.AVP {
		return b.rg("Updated-Network-Configuration", scefID(b), u(b, "SCEF-Reference-ID", 41),
			u64(b, "SCEF-Reference-ID-Ext", 1<<40), u(b, "SCEF-Reference-ID-for-Deletion", 40),
			u64(b, "SCEF-Reference-ID-for-Deletion-Ext", 1<<40-1),
			b.a(avp.SubscribedPeriodicRAUTAUTimer, tgpp, datatype.Unsigned32(3240)), u(b, "Active-Time", 60),
			b.r("DL-Buffering-Suggested-Packet-Count", datatype.Integer32(-1)))
	}
	gSuggested := &s6t.SuggestedNetworkConfiguration{
		ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: scef,
		SubscribedPeriodicRauTauTimer: diam.Some(uint32(3240)), ActiveTime: diam.Some(uint32(60)),
		DlBufferingSuggestedPacketCount: diam.Some(int32(3)), MtcProviderInfo: gMtc,
		ScefReferenceIdForDeletion: []uint32{40}, ScefReferenceIdForDeletionExt: []uint64{1<<40 - 1},
	}
	fSuggested := func(b s6tAVP) *fd.AVP {
		return b.rg("Suggested-Network-Configuration", u(b, "SCEF-Reference-ID", 41), u64(b, "SCEF-Reference-ID-Ext", 1<<40),
			scefID(b), b.a(avp.SubscribedPeriodicRAUTAUTimer, tgpp, datatype.Unsigned32(3240)), u(b, "Active-Time", 60),
			b.r("DL-Buffering-Suggested-Packet-Count", datatype.Integer32(3)), fMtc(b),
			u(b, "SCEF-Reference-ID-for-Deletion", 40), u64(b, "SCEF-Reference-ID-for-Deletion-Ext", 1<<40-1))
	}
	gEcr := &s6t.EnhancedCoverageRestriction{
		RestrictedPlmnList: &s6t.RestrictedPlmnList{VisitedPlmnId: [][]byte{plmn}},
		AllowedPlmnList:    &s6t.AllowedPlmnList{VisitedPlmnId: [][]byte{{0x00, 0xf2, 0x10}}},
	}
	fEcr := func(b s6tAVP) *fd.AVP {
		return b.rg("Enhanced-Coverage-Restriction",
			b.rg("Restricted-PLMN-List", b.a(avp.VisitedPLMNID, tgpp, datatype.OctetString(plmn))),
			b.rg("Allowed-PLMN-List", b.a(avp.VisitedPLMNID, tgpp, datatype.OctetString([]byte{0x00, 0xf2, 0x10}))))
	}

	ciaAVPs := func() []*fd.AVP {
		mer := append([]*fd.AVP{u(s, "SCEF-Reference-ID", 41), u64(s, "SCEF-Reference-ID-Ext", 1<<40),
			scefID(s), u(s, "SCEF-Reference-ID-for-Deletion", 39), u64(s, "SCEF-Reference-ID-for-Deletion-Ext", 1<<40-1),
			s.a(avp.VisitedPLMNID, tgpp, datatype.OctetString(plmn)), u(s, "Roaming-Information", 1), u(s, "IMEI-Change", 1),
			s.g(avp.TerminalInformation, tgpp, s.a(avp.IMEI, tgpp, datatype.UTF8String("3534900698733")),
				s.a(avp.TGPP2MEID, tgpp, datatype.OctetString([]byte{0xa1, 0, 0, 1})),
				s.a(avp.SoftwareVersion, tgpp, datatype.UTF8String("01"))),
			u(s, "Reachability-Information", 1), u(s, "Reachability-Cause", 0), tm(s, "Maximum-UE-Availability-Time", expiry),
			fMme(s), u(s, "Monitoring-Type", 1), u(s, "Event-Handling", 2), fServiceReport(s),
			u(s, "Loss-Of-Connectivity-Reason", 4), fIdle(s)}, fPdn(s)...)
		return []*fd.AVP{session("scef;1;1"), s.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(),
			u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
			fOcSF(), fOlr(), fLoad(), fFeatures(s),
			s.rg("User-Identifier", utf8(avp.UserName, imsi), s.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)), ext(s, extID),
				u(s, "Type-Of-External-Identifier", 0)),
			u(s, "Number-of-UEs", 12), s.rg("Monitoring-Event-Report", mer...),
			s.rg("Monitoring-Event-Config-Status", fServiceReport(s), u(s, "SCEF-Reference-ID", 41),
				u64(s, "SCEF-Reference-ID-Ext", 1<<40), scefID(s)),
			s.rg("AESE-Communication-Pattern-Config-Status", u(s, "SCEF-Reference-ID", 42), u64(s, "SCEF-Reference-ID-Ext", 42<<32),
				scefID(s), s.rg("AESE-Error-Report", s.rg("Service-Result", u(s, "Service-Result-Code", 5004)))),
			s.rg("Supported-Services", u64(s, "Supported-Monitoring-Events", 0x1ff), u(s, "Node-Type", 0)),
			u(s, "S6t-HSS-Cause", 1),
			s.rg("Enhanced-Coverage-Restriction-Data", fEcr(s), s.a(avp.VisitedPLMNID, tgpp, datatype.OctetString(plmn))),
			u(s, "CIA-Flags", 1),
			s.rg("IMSI-Group-Id", u(s, "Group-Service-Id", 3), oct(s, "Group-PLMN-Id", plmn), oct(s, "Local-Group-Id", []byte{0x12, 0x34})),
			fFailed(), fProxy(), fRoute(), fSuggested(s)}
	}

	return []pair{
		{"CIR",
			&s6t.ConfigurationInformationRequest{
				Header: hdr, SessionId: "scef;1;1", Drmp: diam.Some(s6t.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				UserIdentifier:      s6t.UserIdentifier{ExternalIdentifier: diam.Some(groupID), TypeOfExternalIdentifier: diam.Some(uint32(1))},
				OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures,
				MonitoringEventConfiguration: []s6t.MonitoringEventConfiguration{{
					ScefReferenceId: diam.Some(uint32(41)), ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: scef, MonitoringType: 1,
					ScefReferenceIdForDeletion: []uint32{38}, ScefReferenceIdForDeletionExt: []uint64{1<<40 - 1},
					MaximumNumberOfReports: diam.Some(uint32(5)), MonitoringDuration: diam.Some(expiry),
					ChargedParty: diam.Some("sip:charged@example.org"), MaximumDetectionTime: diam.Some(uint32(3600)),
					UeReachabilityConfiguration: &s6t.UeReachabilityConfiguration{
						ReachabilityType: diam.Some(uint32(2)), MaximumLatency: diam.Some(uint32(120)),
						MaximumResponseTime: diam.Some(uint32(30)), DlBufferingSuggestedPacketCount: diam.Some(int32(4)),
					},
					LocationInformationConfiguration: &s6t.LocationInformationConfiguration{
						MonteLocationType: diam.Some(uint32(0)), Accuracy: diam.Some(uint32(1)), PeriodicTime: diam.Some(uint32(600)),
					},
					AssociationType: diam.Some(uint32(1)), DlBufferingSuggestedPacketCount: diam.Some(int32(1)),
					PlmnIdRequested: diam.Some(s6t.PlmnIdRequestedFalse), MtcProviderInfo: gMtc,
					PdnConnectivityStatusConfiguration: &s6t.PdnConnectivityStatusConfiguration{ServiceSelection: diam.Some(apn)},
					ExcludeIdentifiers:                 &s6t.ExcludeIdentifiers{ExternalIdentifier: []string{"device-2@iot.example.org"}, Msisdn: [][]byte{msisdn}},
					IncludeIdentifiers:                 &s6t.IncludeIdentifiers{ExternalIdentifier: []string{"device-3@iot.example.org"}},
				}},
				CirFlags: diam.Some(uint32(6)),
				AeseCommunicationPattern: []s6t.AeseCommunicationPattern{{
					ScefReferenceId: diam.Some(uint32(42)), ScefReferenceIdExt: diam.Some(uint64(42) << 32), ScefId: scef,
					ScefReferenceIdForDeletion: []uint32{36}, ScefReferenceIdForDeletionExt: []uint64{36 << 32},
					CommunicationPatternSet: []s6t.CommunicationPatternSet{{
						PeriodicCommunicationIndicator: diam.Some(uint32(0)), CommunicationDurationTime: diam.Some(uint32(60)),
						PeriodicTime: diam.Some(uint32(3600)),
						ScheduledCommunicationTime: []s6t.ScheduledCommunicationTime{
							{DayOfWeekMask: diam.Some(uint32(0x3e)), TimeOfDayStart: diam.Some(uint32(28800)), TimeOfDayEnd: diam.Some(uint32(64800))},
						},
						StationaryIndication: diam.Some(uint32(0)), ReferenceIdValidityTime: diam.Some(expiry),
						TrafficProfile: diam.Some(uint32(2)), BatteryIndicator: diam.Some(uint32(6)),
					}},
					MtcProviderInfo: gMtc,
				}},
				EnhancedCoverageRestriction: gEcr, GroupReportingGuardTimer: diam.Some(uint32(300)),
				AdditionalIdentifiers: &s6t.AdditionalIdentifiers{ExternalIdentifier: []string{"group-2@iot.example.org"}},
				ProxyInfo:             gProxy, RouteRecord: []string{dra}, SuggestedNetworkConfiguration: gSuggested,
			},
			fs6t(8388718, req|pxy, session("scef;1;1"), q.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), authState(),
				ident(avp.OriginHost, scef), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm),
				q.rg("User-Identifier", ext(q, groupID), u(q, "Type-Of-External-Identifier", 1)),
				fOcSF(), fFeatures(q),
				q.rg("Monitoring-Event-Configuration", u(q, "SCEF-Reference-ID", 41), u64(q, "SCEF-Reference-ID-Ext", 1<<40), scefID(q),
					u(q, "Monitoring-Type", 1), u(q, "SCEF-Reference-ID-for-Deletion", 38), u64(q, "SCEF-Reference-ID-for-Deletion-Ext", 1<<40-1),
					u(q, "Maximum-Number-of-Reports", 5), tm(q, "Monitoring-Duration", expiry),
					q.a(avp.ChargedParty, tgpp, datatype.UTF8String("sip:charged@example.org")), u(q, "Maximum-Detection-Time", 3600),
					q.rg("UE-Reachability-Configuration", u(q, "Reachability-Type", 2), u(q, "Maximum-Latency", 120),
						u(q, "Maximum-Response-Time", 30), q.r("DL-Buffering-Suggested-Packet-Count", datatype.Integer32(4))),
					q.rg("Location-Information-Configuration", u(q, "MONTE-Location-Type", 0), u(q, "Accuracy", 1), u(q, "Periodic-Time", 600)),
					u(q, "Association-Type", 1), q.r("DL-Buffering-Suggested-Packet-Count", datatype.Integer32(1)),
					q.r("PLMN-ID-Requested", datatype.Enumerated(1)), fMtc(q),
					q.rg("PDN-Connectivity-Status-Configuration", sel(q, apn)),
					q.rg("Exclude-Identifiers", ext(q, "device-2@iot.example.org"), q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn))),
					q.rg("Include-Identifiers", ext(q, "device-3@iot.example.org"))),
				u(q, "CIR-Flags", 6),
				q.rg("AESE-Communication-Pattern", u(q, "SCEF-Reference-ID", 42), u64(q, "SCEF-Reference-ID-Ext", 42<<32), scefID(q),
					u(q, "SCEF-Reference-ID-for-Deletion", 36), u64(q, "SCEF-Reference-ID-for-Deletion-Ext", 36<<32),
					q.rg("Communication-Pattern-Set", u(q, "Periodic-Communication-Indicator", 0), u(q, "Communication-Duration-Time", 60),
						u(q, "Periodic-Time", 3600),
						q.rg("Scheduled-Communication-Time", u(q, "Day-Of-Week-Mask", 0x3e), u(q, "Time-Of-Day-Start", 28800),
							u(q, "Time-Of-Day-End", 64800)),
						u(q, "Stationary-Indication", 0), tm(q, "Reference-ID-Validity-Time", expiry), u(q, "Traffic-Profile", 2),
						u(q, "Battery-Indicator", 6)),
					fMtc(q)),
				fEcr(q), u(q, "Group-Reporting-Guard-Timer", 300),
				q.rg("AdditionalIdentifiers", ext(q, "group-2@iot.example.org")),
				fProxy(), fRoute(), fSuggested(q)),
		},
		{"CIA",
			&s6t.ConfigurationInformationAnswer{
				Header: hdr, SessionId: "scef;1;1", Drmp: diam.Some(s6t.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, SupportedFeatures: gFeatures,
				UserIdentifier: &s6t.UserIdentifier{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), ExternalIdentifier: diam.Some(extID),
					TypeOfExternalIdentifier: diam.Some(uint32(0))},
				NumberOfUes: diam.Some(uint32(12)),
				MonitoringEventReport: []s6t.MonitoringEventReport{{
					ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: diam.Some(scef),
					ScefReferenceIdForDeletion: diam.Some(uint32(39)), ScefReferenceIdForDeletionExt: diam.Some(uint64(1<<40 - 1)),
					VisitedPlmnId: diam.Some(plmn), RoamingInformation: diam.Some(uint32(1)), ImeiChange: diam.Some(uint32(1)),
					TerminalInformation: &s6t.TerminalInformation{Imei: diam.Some("3534900698733"),
						Tgpp2Meid: diam.Some([]byte{0xa1, 0, 0, 1}), SoftwareVersion: diam.Some("01")},
					ReachabilityInformation: diam.Some(uint32(1)), ReachabilityCause: diam.Some(uint32(0)),
					MaximumUeAvailabilityTime: diam.Some(expiry), EpsLocationInformation: gMme,
					MonitoringType: diam.Some(uint32(1)), EventHandling: diam.Some(uint32(2)), ServiceReport: gServiceReport,
					LossOfConnectivityReason: diam.Some(uint32(4)), IdleStatusIndication: gIdle, PdnConnectivityStatusReport: gPdn,
				}},
				MonitoringEventConfigStatus: []s6t.MonitoringEventConfigStatus{{
					ServiceReport: gServiceReport, ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: diam.Some(scef),
				}},
				AeseCommunicationPatternConfigStatus: []s6t.AeseCommunicationPatternConfigStatus{{
					ScefReferenceId: 42, ScefReferenceIdExt: diam.Some(uint64(42) << 32), ScefId: diam.Some(scef),
					AeseErrorReport: &s6t.AeseErrorReport{ServiceResult: &s6t.ServiceResult{ServiceResultCode: diam.Some(uint32(5004))}},
				}},
				SupportedServices: []s6t.SupportedServices{{SupportedMonitoringEvents: diam.Some(uint64(0x1ff)), NodeType: diam.Some(uint32(0))}},
				S6THssCause:       diam.Some(uint32(1)),
				EnhancedCoverageRestrictionData: &s6t.EnhancedCoverageRestrictionData{EnhancedCoverageRestriction: *gEcr,
					VisitedPlmnId: diam.Some(plmn)},
				CiaFlags:    diam.Some(uint32(1)),
				ImsiGroupId: []s6t.ImsiGroupId{{GroupServiceId: 3, GroupPlmnId: plmn, LocalGroupId: []byte{0x12, 0x34}}},
				FailedAvp:   gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra}, SuggestedNetworkConfiguration: gSuggested,
			},
			fs6t(8388718, pxy, ciaAVPs()...),
		},
		{"RIR",
			&s6t.ReportingInformationRequest{
				Header: hdr, SessionId: "hss;1;2", Drmp: diam.Some(s6t.DrmpPriority3), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: scef, DestinationRealm: realm,
				SupportedFeatures: gFeatures, UserIdentifier: &s6t.UserIdentifier{Msisdn: diam.Some(msisdn)},
				MonitoringEventReport: []s6t.MonitoringEventReport{{
					ScefReferenceId: 43, ScefId: diam.Some(scef), EpsLocationInformation: gSgsn, MonitoringType: diam.Some(uint32(2)),
				}},
				GroupReport: []s6t.GroupReport{{
					ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: diam.Some(scef),
					GroupReportItem: []s6t.GroupReportItem{{
						UserIdentifier: s6t.UserIdentifier{ExternalIdentifier: diam.Some(extID)}, VisitedPlmnId: diam.Some(plmn),
						RoamingInformation: diam.Some(uint32(0)), ImeiChange: diam.Some(uint32(2)),
						ReachabilityInformation: diam.Some(uint32(0)), MaximumUeAvailabilityTime: diam.Some(expiry),
						EpsLocationInformation: gMme, MonitoringType: diam.Some(uint32(8)), ServiceReport: gServiceReport,
						S6THssCause: diam.Some(uint32(2)), IdleStatusIndication: gIdle, ReportingTimeStamp: diam.Some(stamp),
						UpdatedNetworkConfiguration: gUpdated, ScefReferenceIdForDeletion: []uint32{38},
						ScefReferenceIdForDeletionExt: []uint64{38 << 32}, EventHandling: diam.Some(uint32(0)),
						LossOfConnectivityReason: diam.Some(uint32(2)), PdnConnectivityStatusReport: gPdn[:1],
					}},
				}},
				UpdatedNetworkConfiguration: gUpdated, RirFlags: diam.Some(uint32(5)),
				SupportedServices: []s6t.SupportedServices{{SupportedMonitoringEvents: diam.Some(uint64(6)), NodeType: diam.Some(uint32(2))}},
				ProxyInfo:         gProxy, RouteRecord: []string{dra},
			},
			fs6t(8388719, req|pxy, session("hss;1;2"), q.a(avp.DRMP, 0, datatype.Enumerated(3)), fVSA(), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, scef), ident(avp.DestinationRealm, realm),
				fFeatures(q), q.rg("User-Identifier", q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn))),
				q.rg("Monitoring-Event-Report", u(q, "SCEF-Reference-ID", 43), scefID(q), fSgsn(q), u(q, "Monitoring-Type", 2)),
				q.rg("Group-Report", u(q, "SCEF-Reference-ID", 41), u64(q, "SCEF-Reference-ID-Ext", 1<<40), scefID(q),
					q.rg("Group-Report-Item", q.rg("User-Identifier", ext(q, extID)), q.a(avp.VisitedPLMNID, tgpp, datatype.OctetString(plmn)),
						u(q, "Roaming-Information", 0), u(q, "IMEI-Change", 2), u(q, "Reachability-Information", 0),
						tm(q, "Maximum-UE-Availability-Time", expiry), fMme(q), u(q, "Monitoring-Type", 8), fServiceReport(q),
						u(q, "S6t-HSS-Cause", 2), fIdle(q), tm(q, "Reporting-Time-Stamp", stamp), fUpdated(q),
						u(q, "SCEF-Reference-ID-for-Deletion", 38), u64(q, "SCEF-Reference-ID-for-Deletion-Ext", 38<<32),
						u(q, "Event-Handling", 0), u(q, "Loss-Of-Connectivity-Reason", 2), fPdn(q)[0])),
				fUpdated(q), u(q, "RIR-Flags", 5),
				q.rg("Supported-Services", u64(q, "Supported-Monitoring-Events", 6), u(q, "Node-Type", 2)),
				fProxy(), fRoute()),
		},
		{"RIA",
			&s6t.ReportingInformationAnswer{
				Header: hdr, SessionId: "hss;1;2", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm, SupportedFeatures: gFeatures,
				MonitoringEventReportStatus: []s6t.MonitoringEventReportStatus{
					{ScefReferenceId: 43, ScefReferenceIdExt: diam.Some(uint64(43) << 32), ScefId: scef, ResultCode: diam.Some(uint32(2001))},
					{ScefReferenceId: 44, ScefId: scef, ExperimentalResultCode: diam.Some(uint32(5514))},
				},
				FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6t(8388719, pxy, session("hss;1;2"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, scef),
				ident(avp.OriginRealm, realm), fFeatures(s),
				s.rg("Monitoring-Event-Report-Status", u(s, "SCEF-Reference-ID", 43), u64(s, "SCEF-Reference-ID-Ext", 43<<32), scefID(s),
					u32(avp.ResultCode, 2001)),
				s.rg("Monitoring-Event-Report-Status", u(s, "SCEF-Reference-ID", 44), scefID(s), u32(avp.ExperimentalResultCode, 5514)),
				fFailed(), fProxy(), fRoute()),
		},
		{"NIR",
			&s6t.NiddInformationRequest{
				Header: hdr, SessionId: "scef;1;3", Drmp: diam.Some(s6t.DrmpPriority2), AuthSessionState: nsm,
				OriginHost: scef, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				UserIdentifier: s6t.UserIdentifier{UserName: diam.Some(imsi)}, OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures,
				NiddAuthorizationRequest: &s6t.NiddAuthorizationRequest{ServiceSelection: diam.Some(apn), RequestedValidityTime: diam.Some(expiry)},
				NiddAuthorizationUpdate: &s6t.NiddAuthorizationUpdate{ApnValidityTime: []s6t.ApnValidityTime{
					{GrantedValidityTime: expiry, ServiceSelection: diam.Some(apn)},
				}},
				NirFlags: diam.Some(uint32(1)),
				GroupUserIdentifier: []s6t.GroupUserIdentifier{
					{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), ExternalIdentifier: diam.Some(extID)},
				},
				MtcProviderInfo: gMtc, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6t(8388726, req|pxy, session("scef;1;3"), q.a(avp.DRMP, 0, datatype.Enumerated(2)), authState(),
				ident(avp.OriginHost, scef), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm),
				q.rg("User-Identifier", utf8(avp.UserName, imsi)), fOcSF(), fFeatures(q),
				q.rg("NIDD-Authorization-Request", sel(q, apn), tm(q, "Requested-Validity-Time", expiry)),
				q.rg("NIDD-Authorization-Update", q.rg("APN-Validity-Time", tm(q, "Granted-Validity-Time", expiry), sel(q, apn))),
				u(q, "NIR-Flags", 1),
				q.rg("Group-User-Identifier", utf8(avp.UserName, imsi), q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)), ext(q, extID)),
				fMtc(q), fProxy(), fRoute()),
		},
		{"NIA",
			&s6t.NiddInformationAnswer{
				Header: hdr, SessionId: "scef;1;3", Drmp: diam.Some(s6t.DrmpPriority2), ExperimentalResult: gExp,
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad,
				SupportedFeatures: gFeatures,
				NiddAuthorizationResponse: &s6t.NiddAuthorizationResponse{Msisdn: diam.Some(msisdn), UserName: diam.Some(imsi),
					ExternalIdentifier: diam.Some(extID), GrantedValidityTime: diam.Some(expiry)},
				GroupUserIdentifier: []s6t.GroupUserIdentifier{{UserName: diam.Some(imsi), ExternalIdentifier: diam.Some(extID)}},
				NiaFlags:            diam.Some(uint32(1)), FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6t(8388726, pxy, session("scef;1;3"), s.a(avp.DRMP, 0, datatype.Enumerated(2)), fExp(), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), fOcSF(), fOlr(), fLoad(), fFeatures(s),
				s.rg("NIDD-Authorization-Response", s.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)), utf8(avp.UserName, imsi),
					ext(s, extID), tm(s, "Granted-Validity-Time", expiry)),
				s.rg("Group-User-Identifier", utf8(avp.UserName, imsi), ext(s, extID)),
				u(s, "NIA-Flags", 1), fFailed(), fProxy(), fRoute()),
		},
	}
}

// TestFiorixS6tGaps states what go-diameter v4.6.0 lacks for S6t and keeps
// s6tRestated and s6tM exact: an entry go-diameter gains must move to the
// independent side, and an M-bit override must still be needed.
func TestFiorixS6tGaps(t *testing.T) {
	if _, err := dict.Default.App(s6tApp); err == nil {
		t.Errorf("go-diameter now has an S6t (%d) dictionary: build the S6t interop on it and drop the per-AVP lookup", s6tApp)
	}
	for name, d := range s6tRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from s6tRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
	for k, m := range s6tM {
		_, def := fiorixHome(k[0], k[1])
		if def == nil {
			t.Errorf("AVP %d vendor %d: go-diameter no longer defines it: move it to s6tRestated", k[0], k[1])
			continue
		}
		if strings.Contains(def.Must, "M") == m {
			t.Errorf("AVP %d vendor %d: go-diameter now agrees on the M-bit: remove it from s6tM", k[0], k[1])
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestS6tFiorixToGoDiam(t *testing.T) {
	for _, p := range s6tPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// go-diam encodes; go-diameter reads the message (it has no S6t dictionary)
// and decodes every AVP in the application that defines it
// (checkGoDiamToFiorixByAVP in cx_test.go).
func TestS6tGoDiamToFiorix(t *testing.T) {
	for _, p := range s6tPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, s6tApp, s6tRestated) })
	}
}
