# go-diam

Typed, code-generated Diameter codecs for Go.

You describe Diameter commands and AVPs in **standard proto3 files**. The
`protoc-gen-go-diam` plugin turns them into **plain Go structs** with
`Marshal`, `Unmarshal` and `Validate`. There is no runtime dictionary and no
reflection, and the runtime uses only the standard library.

```go
ulr := &s6a.UpdateLocationRequest{
    SessionId:        "mme.example.org;1;42",
    AuthSessionState: base.AuthSessionStateNoStateMaintained,
    OriginHost:       "mme.example.org",
    OriginRealm:      "example.org",
    DestinationRealm: "example.org",
    UserName:         "001010000000001",
    RatType:          s6a.RatTypeEutran,
    UlrFlags:         0x22,
    VisitedPlmnId:    []byte{0x00, 0xf1, 0x10},
}
b, err := ulr.Marshal()
```

> **Status: pre-release, wave 1 in progress.** What is in the repository
> today: the runtime (`diam`), the generator (`cmd/protoc-gen-go-diam`), the
> RFC 6733 base protocol (`base`), S6a/S6d and S13/S13' from TS 29.272
> (`s6a`, `s13`), Cx/Dx from TS 29.229 (`cx`), Sh/Dh from TS 29.329
> (`sh`), SLh from TS 29.173 (`slh`), Zh from TS 29.109 (`zh`) and SWx from
> TS 29.273 (`swx`), each tested in both directions against
> fiorix/go-diameter (for Cx, Sh, SLh and Zh, which go-diameter has no
> dictionary for, AVP by AVP; see
> [Coverage](#coverage)). Captures from freeDiameter are committed for three
> of the six base command pairs (CEA, DWA, DPA) and none for the 3GPP
> applications, so all eight are still *spec-only*. The API may still change before `v1.0.0`.

The same with the base protocol:

```go
import (
    "net/netip"

    "github.com/hsdfat/go-diam/base"
    "github.com/hsdfat/go-diam/diam"
)

cer := &base.CapabilitiesExchangeRequest{
    Header:            diam.Header{HopByHop: 1, EndToEnd: 1},
    OriginHost:        "mme.example.org",
    OriginRealm:       "example.org",
    HostIpAddress:     []netip.Addr{netip.MustParseAddr("192.0.2.1")},
    VendorId:          10415,
    ProductName:       "my-node",
    AuthApplicationId: []uint32{16777251},
}
b, err := cer.Marshal()

m, err := diam.Decode(answer) // *base.CapabilitiesExchangeAnswer
if cea, ok := m.(*base.CapabilitiesExchangeAnswer); ok && cea.Validate() == nil {
    // cea.ResultCode, cea.OriginHost, cea.SupportedVendorId, ...
}
```

---

## Contents

- [Why](#why)
- [Install](#install)
- [Usage](#usage)
- [Comparison with fiorix/go-diameter](#comparison-with-fiorixgo-diameter)
- [Benchmarks](#benchmarks)
- [Coverage](#coverage)
- [How correctness is checked](#how-correctness-is-checked)
- [Defining your own application](#defining-your-own-application)
- [Roadmap](#roadmap)
- [License](#license)

## Why

Diameter grammars are static. TS 29.272 defines exactly which AVPs a ULR
carries, their codes, vendor IDs, flags and types. go-diam moves that
knowledge to compile time:

- **Typed fields.** `ulr.UserName` is a `string` and `ulr.RatType` is a
  `s6a.RatType`. There are no `FindAVP` calls or type assertions.
- **Grammar in the type.** Required AVPs are plain fields, optional scalars
  are `diam.Opt[T]`, optional groups are `*T`, and repeated AVPs are `[]T`.
- **Errors you can answer with.** `Validate` returns an `*diam.AVPError`
  carrying the Result-Code (5001, 5004, 5005, 5009, 5014) and the offending
  AVP for `Failed-AVP`.
- **Unknown AVPs survive.** AVPs the schema does not describe are kept in
  `Unknown` and re-encoded verbatim. Known AVPs are re-encoded from the typed
  fields, in grammar order: an AVP whose value could not be decoded or that
  occurs too often is reported by `Validate` and not re-encoded, so check
  `Validate` before forwarding a message.
- **Fast.** Encoding allocates nothing when you reuse a buffer
  (`AppendMarshal`). See [Benchmarks](#benchmarks).
- **Codec only.** go-diam does not open sockets. Use your own TCP/SCTP/TLS
  code, or go-diameter's transport, and hand go-diam the bytes.

## Install

```sh
go get github.com/hsdfat/go-diam
```

Packages: `github.com/hsdfat/go-diam/diam` (runtime) and one generated
package per application, today `github.com/hsdfat/go-diam/base`,
`github.com/hsdfat/go-diam/s6a`, `github.com/hsdfat/go-diam/s13`,
`github.com/hsdfat/go-diam/cx`, `github.com/hsdfat/go-diam/sh`,
`github.com/hsdfat/go-diam/slh`, `github.com/hsdfat/go-diam/zh` and
`github.com/hsdfat/go-diam/swx`. Generated
code is committed in the repository. You only need the generator if you
[define your own application](#defining-your-own-application).

Minimum Go version: 1.23. The library depends on the standard library only.

## Usage

### Encode

```go
import (
    "github.com/hsdfat/go-diam/base"
    "github.com/hsdfat/go-diam/diam"
    "github.com/hsdfat/go-diam/s6a"
)

ulr := &s6a.UpdateLocationRequest{
    Header:           diam.Header{HopByHop: hbh, EndToEnd: e2e},
    SessionId:        "mme.example.org;1;42",
    AuthSessionState: base.AuthSessionStateNoStateMaintained,
    OriginHost:       "mme.example.org",
    OriginRealm:      "example.org",
    DestinationHost:  diam.Some("hss.example.org"), // optional AVP
    DestinationRealm: "example.org",
    UserName:         "001010000000001",
    SupportedFeatures: []s6a.SupportedFeatures{     // *[ Supported-Features ]
        {VendorId: 10415, FeatureListId: 1, FeatureList: 0x0bff},
    },
    TerminalInformation: &s6a.TerminalInformation{  // optional grouped AVP
        Imei: diam.Some("3534900698733"),
    },
    RatType:       s6a.RatTypeEutran,
    UlrFlags:      0x22,
    VisitedPlmnId: []byte{0x00, 0xf1, 0x10},
}

b, err := ulr.Marshal()

// Hot path: reuse a buffer, zero allocations.
buf, err = ulr.AppendMarshal(buf[:0])
```

The command code, application ID and the R/P header bits come from the
schema. Hop-by-Hop and End-to-End IDs are yours to set.

### Decode

Use the concrete type when you know what to expect:

```go
var ulr s6a.UpdateLocationRequest
if err := ulr.Unmarshal(b); err != nil {
    return err // framing error only: cannot even split the AVPs
}
```

Or let go-diam pick the type from (application ID, command code, R-bit):

```go
m, err := diam.Decode(b)
if err != nil {
    return err
}
switch req := m.(type) {
case *s6a.UpdateLocationRequest:
    handleULR(req)
case *s6a.AuthenticationInformationRequest:
    handleAIR(req)
}
```

### Validate and answer with the right Result-Code

Decoding is lenient and `Validate` is strict. `Unmarshal` fills in
everything it can. `Validate` reports the first grammar violation as an
`*diam.AVPError`:

```go
if err := req.Validate(); err != nil {
    var ae *diam.AVPError
    if errors.As(err, &ae) {
        ula := s6a.NewUpdateLocationAnswerFor(req) // copies Session-Id, Hop-by-Hop, End-to-End
        ula.ResultCode = diam.Some(ae.ResultCode)  // e.g. 5005 DIAMETER_MISSING_AVP
        ula.FailedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{ae.AVP}}
        // ... fill Origin-Host / Origin-Realm and send
    }
}
```

| Violation | Result-Code |
|---|---|
| Unknown AVP with the M-bit set | 5001 `DIAMETER_AVP_UNSUPPORTED` |
| Invalid value (e.g. bad address family) | 5004 `DIAMETER_INVALID_AVP_VALUE` |
| Required AVP missing | 5005 `DIAMETER_MISSING_AVP` |
| AVP occurs too many times | 5009 `DIAMETER_AVP_OCCURS_TOO_MANY_TIMES` |
| Wrong data length for the AVP type | 5014 `DIAMETER_INVALID_AVP_LENGTH` |

Unknown AVPs **without** the M-bit are kept in `Unknown` and are not errors
(RFC 6733 §4.1).

### Read messages from a stream

```go
for {
    frame, err := diam.ReadFrame(conn) // exactly one Diameter message
    if err != nil {
        return err
    }
    m, err := diam.Decode(frame)
    // ...
}
```

### Use with go-diameter's transport

If you already run go-diameter for connections, CER/CEA and watchdogs, keep
it and decode the bytes with go-diam inside a handler:

```go
mux.HandleFunc("ULR", func(c fdiam.Conn, m *fdiam.Message) {
    raw, _ := m.Serialize()
    var ulr s6a.UpdateLocationRequest
    if err := ulr.Unmarshal(raw); err != nil { /* ... */ }
    // typed access from here on
    out, _ := buildULA(&ulr).Marshal()
    c.Write(out)
})
```

### Optional values

```go
if host, ok := ulr.DestinationHost.Get(); ok {
    route(host)
}
ulr.UeSrvccCapability = diam.Some(s6a.UeSrvccCapabilityUeSrvccSupported)
```

`diam.Opt[T]` is a value plus a presence flag. Unlike `*T`, it does not
allocate.

## Comparison with fiorix/go-diameter

[fiorix/go-diameter](https://github.com/fiorix/go-diameter) is the
established Go Diameter library and has been used in production for years.
The two libraries make different trade-offs and can be used together.

| | **go-diam** | **fiorix/go-diameter** |
|---|---|---|
| Scope | Codec only (encode, decode, validate) | Codec **and** transport: TCP/SCTP/TLS, CER/CEA and DWR/DWA state machine, client and server |
| Message model | Generated Go struct per command and grouped AVP | Generic `*diam.Message` with an AVP tree |
| Field access | `ulr.UserName` (typed) | `m.FindAVP(avp.UserName, 0)` and a type assertion, or `m.Unmarshal(&s)` with reflection and struct tags |
| Dictionary | Compiled in from proto3 files. Mistakes in the schema fail at generation time | XML loaded at runtime. Can be extended without regenerating |
| Grammar checks | Generated `Validate()` returning the Result-Code and Failed-AVP | Rules are parsed from the dictionary XML but not enforced on messages; the application checks |
| Unknown AVPs | Kept and re-encoded verbatim. M-bit ones reported as 5001 | Kept in the AVP tree as `datatype.Unknown` |
| Runtime dependencies | Standard library only | grpc, glog, protobuf, sctp |
| 3GPP interfaces included | Planned: all (see [Coverage](#coverage)) | Base, Credit-Control, Gx, Gy/Ro/Rf, Rx, Sy, S6a, S13, SWx, NASREQ. Cx/Dx, Sh/Dh, SLh and Zh are not included |
| Maturity | Pre-release | Mature, many users |

**Choose go-diameter** if you want a complete stack with connections, peer
handling and routing today, or need to load dictionaries at runtime.

**Choose go-diam** if you want typed messages, grammar validation with ready
Result-Codes, low allocation on the hot path, or interfaces go-diameter
does not ship. You can also combine them, as shown in
[Use with go-diameter's transport](#use-with-go-diameters-transport).

### The same task in both

Read the IMSI and the RAT type from a ULR:

```go
// go-diameter
a, err := m.FindAVP(avp.UserName, 0)
if err != nil {
    return err
}
imsi := string(a.Data.(datatype.UTF8String))
r, err := m.FindAVP(avp.RATType, 10415)
if err != nil {
    return err
}
rat := int32(r.Data.(datatype.Enumerated))
```

```go
// go-diam
var ulr s6a.UpdateLocationRequest
if err := ulr.Unmarshal(b); err != nil {
    return err
}
imsi, rat := ulr.UserName, ulr.RatType
```

## Benchmarks

Both libraries decode the same wire bytes and encode the message they
decoded from them. go-diameter v4.6.0 uses its default dictionary.

| Message | Operation | go-diam | go-diameter |
|---|---|---|---|
| CER (14 AVPs, 1 grouped) | Decode | 2299 ns/op, 240 B, 10 allocs | 7119 ns/op, 1464 B, 40 allocs |
| CER | Encode (reused buffer) | 328 ns/op, 0 B, 0 allocs | 2259 ns/op, 144 B, 16 allocs |
| STR (13 AVPs, 1 grouped) | Decode | 2435 ns/op, 328 B, 15 allocs | 7573 ns/op, 1520 B, 45 allocs |
| STR | Encode (reused buffer) | 211 ns/op, 0 B, 0 allocs | 1813 ns/op, 208 B, 15 allocs |
| S6a ULR (21 AVPs, 7 grouped, 660 B) | Decode | 6267 ns/op, 1176 B, 28 allocs | 21293 ns/op, 4077 B, 122 allocs |
| S6a ULR | Encode (reused buffer) | 777 ns/op, 0 B, 0 allocs | 5677 ns/op, 560 B, 30 allocs |
| S6a ULA with Subscription-Data (2.3 kB) | Decode | 23616 ns/op, 5584 B, 83 allocs | 80436 ns/op, 15598 B, 385 allocs |
| S6a ULA with Subscription-Data | Encode (reused buffer) | 1669 ns/op, 0 B, 0 allocs | 34550 ns/op, 5928 B, 122 allocs |
| S13 ECR (12 AVPs, 3 grouped, 372 B) | Decode | 3009 ns/op, 408 B, 15 allocs | 9868 ns/op, 1728 B, 59 allocs |
| S13 ECR | Encode (reused buffer) | 432 ns/op, 0 B, 0 allocs | 3195 ns/op, 344 B, 19 allocs |
| SWx MAA (15 AVPs, 6 grouped, 628 B) | Decode | 5169 ns/op, 936 B, 16 allocs | 21567 ns/op, 4205 B, 116 allocs |
| SWx MAA | Encode (reused buffer) | 579 ns/op, 0 B, 0 allocs | 5538 ns/op, 616 B, 28 allocs |

Intel i3-12100F, Go 1.26.4, linux/amd64, `-benchtime=200000x`, on a shared
machine under a CPU quota. These are measurements of single messages on one
machine, not a general claim; compare the two columns of a row, not rows
with each other. Reproduce with:

```sh
cd internal/interop && go test -run='^$' -bench=. -benchtime=200000x .
```

## Coverage

Statuses:

- **verified**: all six gates below pass, including captures from a real
  peer.
- **spec-only**: transcribed and cross-checked, but no capture from a real
  peer is committed.
- **partial**: some commands missing.
- **prototype**: exists on the `spike/proto3-codegen` branch only.
- **planned**: not started.

| Application | Spec | Wave | Status |
|---|---|---|---|
| Base protocol (CER/CEA, DWR/DWA, DPR/DPA, RAR/RAA, STR/STA, ASR/ASA) | RFC 6733 | 0 | spec-only: all 12 commands byte-identical with go-diameter in both directions, dictionary cross-checked, fuzzed; CEA, DWA and DPA captured from freeDiameter and replayed (`testdata/captures/base`), no capture for RAR/RAA, STR/STA, ASR/ASA |
| S6a/S6d (ULR/ULA, AIR/AIA, CLR/CLA, IDR/IDA, DSR/DSA, PUR/PUA, RSR/RSA, NOR/NOA) | TS 29.272 V18.7.0 | 1 | spec-only: all 16 commands byte-identical with go-diameter in both directions (on the AVPs go-diameter knows), full Subscription-Data, fuzzed; 146 of 314 dictionary entries cross-checked, the 168 go-diameter lacks are listed in `internal/interop/testdata/dictcheck-absent.txt`; no peer capture yet |
| Cx/Dx (UAR/UAA, SAR/SAA, LIR/LIA, MAR/MAA, RTR/RTA, PPR/PPA) | TS 29.229 V18.1.0 | 1 | spec-only: all 12 commands byte-identical with go-diameter in both directions, every grouped AVP of the commands, fuzzed. go-diameter has no Cx application: its side is built from the Cx AVPs it defines under SWx, Ro/Rf, Rx, S6a, NASREQ and base, and decodes each AVP in the application that defines it; the 48 AVPs it defines nowhere (Public-Identity, Digest-*, the restoration and P-CSCF AVPs, ...) carry codes and flags restated from the spec in the test. 37 of 85 dictionary entries cross-checked, the 48 others listed in `internal/interop/testdata/dictcheck-absent.txt`; no benchmark (go-diameter would decode Cx as opaque AVPs, not a like-for-like comparison); no peer capture yet |
| Sh/Dh (UDR/UDA, PUR/PUA, SNR/SNA, PNR/PNA) | TS 29.329 V18.0.0 | 1 | spec-only: all 8 commands byte-identical with go-diameter in both directions, every grouped AVP of the commands and Wildcarded-IMPU, fuzzed. go-diameter has no Sh application: its side is built from the Sh AVPs it defines under Ro/Rf, Rx, SWx and base (MSISDN, Server-Name, Session-Priority, Supported-Features, DRMP, OC-\*, Load), and decodes each AVP in the application that defines it; the 26 AVPs it defines nowhere (User-Identity, User-Data, Data-Reference, Wildcarded-IMPU, External-Identifier, ...) carry codes and flags restated from the spec in the test. 18 of 45 dictionary entries cross-checked, the 27 others listed in `internal/interop/testdata/dictcheck-absent.txt`; no benchmark (go-diameter would decode Sh as opaque AVPs); no peer capture yet |
| S13/S13' (ECR/ECA) | TS 29.272 V18.7.0 | 1 | spec-only: both commands byte-identical with go-diameter in both directions, built from go-diameter's S13 dictionary (16777252), including an EQUIPMENT_UNKNOWN (5422) answer and 3GPP2-MEID, fuzzed, benchmarked (ECR). No dictionary of its own: `s13.proto` imports the S6a schema, so its 6 AVPs (Terminal-Information, IMEI, Software-Version, 3GPP2-MEID, Equipment-Status, DRMP) are S6a dictionary entries; all 6 are cross-checked against go-diameter and agree, none is missing from it, and the 0 new entries add nothing to `internal/interop/testdata/dictcheck-absent.txt`. go-diameter's S13 grammar makes User-Name required where TS 29.272 §7.2.19 has it optional; no peer capture yet |
| SWx (MAR/MAA, PPR/PPA, SAR/SAA, RTR/RTA) | TS 29.273 V18.6.0 | 1 | spec-only: all 8 commands byte-identical with go-diameter in both directions, built from go-diameter's SWx dictionary (16777265), including a full Non-3GPP-User-Data, Access-Network-Info and an unknown-user SAA, fuzzed, benchmarked (MAA). go-diameter's SWx application is the Release 9 one: it has no PPR/PPA command (it still reads them), 13 AVPs of the samples are built from its other applications and 20 it defines nowhere (PPR-Flags, TWAN-\*, Access-Network-Info, ERP-Authorization, Local-Time-Zone, ...) carry codes and flags restated from the spec in the test. Its own dictionary has 35 entries, the other re-used AVPs come from the Cx and S6a schemas: 21 of the 35 cross-checked against go-diameter (one reviewed M-bit disagreement, AN-Trusted), the 14 others listed in `internal/interop/testdata/dictcheck-absent.txt`; no peer capture yet |
| S6t | TS 29.336 | 1 | planned |
| S6m/S6c | TS 29.336 / 29.338 | 1 | planned |
| SLh (RIR/RIA) | TS 29.173 V18.0.0 | 1 | spec-only: both commands byte-identical with go-diameter in both directions, Serving-Node and Additional-Serving-Node with every member, an ABSENT_USER (4201) answer, fuzzed. go-diameter has no SLh application: its side is built from the SLh AVPs it defines under Ro/Rf, S6a and SWx (Serving-Node, MME-Name, MME-Realm, GMLC-Address, MSISDN, SGSN-Number, GMLC-Number, 3GPP-AAA-Server-Name, Supported-Features), and decodes each AVP in the application that defines it; the 8 AVPs it defines nowhere (LMSI, MSC-Number, LCS-Capabilities-Sets, Additional-Serving-Node, PPR-Address, SGSN-Name, SGSN-Realm, RIA-Flags) carry codes and flags restated from the spec in the test. `slh.proto` imports the S6a schema for MSISDN, SGSN-Number, GMLC-Number and Supported-Features (6 S6a entries, all cross-checked) and has 13 entries of its own: 5 cross-checked against go-diameter with no disagreement, the 8 others listed in `internal/interop/testdata/dictcheck-absent.txt`; no benchmark (go-diameter would decode SLh as opaque AVPs); no peer capture yet |
| Zh (MAR/MAA) | TS 29.109 V18.1.0 | 1 | spec-only: both commands byte-identical with go-diameter in both directions, an AKA and a SIP Digest answer and an IDENTITY_UNKNOWN (5401) answer, fuzzed. go-diameter has no Zh application and none of the GAA AVPs: its side is built from SIP-Auth-Data-Item and its AKA members (SWx) and the base protocol, and decodes each AVP in the application that defines it; the 9 AVPs it defines nowhere (GBA-UserSecSettings, GUSS-Timestamp, SHA-256-Indication, Public-Identity, SIP-Digest-Authenticate and the 4 Digest-\* AVPs) carry codes and flags restated from the spec in the test. `zh.proto` imports the Cx schema for Public-Identity and SIP-Auth-Data-Item (TS 29.109 §6.3: used as they are) and has 3 entries of its own, none in go-diameter, all 3 listed in `internal/interop/testdata/dictcheck-absent.txt`, so 0 of 3 cross-checked; Zn and Zpn are not included; no benchmark; no peer capture yet |
| Credit-Control | RFC 8506 | 2 | prototype (CCR/CCA) |
| Gx | TS 29.212 | 2 | planned |
| Rx | TS 29.214 | 2 | planned |
| Gy/Ro, Rf | TS 32.299 | 2 | planned |
| Sd | TS 29.212 | 2 | planned |
| Sy | TS 29.219 | 2 | planned |
| Gq | TS 29.209 | 2 | planned |
| S6b, SWm, STa | TS 29.273 | 3 | planned |
| SLg | TS 29.172 | 3 | planned |
| T6a/T6b | TS 29.128 | 3 | planned |
| Np | TS 29.217 | 3 | planned |
| Nt | TS 29.154 | 3 | planned |
| NASREQ | RFC 7155 | 3 | planned |
| EAP | RFC 4072 | 3 | planned |

Schemas follow the latest 3GPP release as a superset and stay compatible
with peers back to **Rel-8**. An AVP that became mandatory only in a later
release is marked optional, so messages from older peers still validate.

## How correctness is checked

A wrong AVP code, vendor ID or M-bit can pass every round-trip test against
your own encoder. go-diam therefore gates each interface on independent
evidence:

1. **Spec transcription.** Every proto file names the spec version and
   clauses it was written from, and carries the spec's ABNF as comments.
2. **Dictionary cross-check.** CI compares name, code, vendor, M-bit and
   type of every AVP with the go-diameter dictionary (freeDiameter's is to
   follow). A disagreement fails the build until it is fixed or listed with
   a spec citation in `internal/interop/testdata/dictcheck-allow.txt`. AVPs
   go-diameter does not have are listed in
   `internal/interop/testdata/dictcheck-absent.txt`: for them the
   transcription is the only evidence.
3. **Interop.** Each command is built once with go-diameter and once with
   go-diam; the bytes must be identical, and each library must decode and
   re-encode the other's output. Where go-diameter has no dictionary for an
   application (Cx/Dx, Sh/Dh, SLh, Zh), it decodes each AVP with its definition from
   whichever of its applications has one, and the AVPs it defines nowhere
   are listed in the test with their code and flags restated from the
   specification.
4. **Real captures.** Frames received from open-source peers running in CI
   (freeDiameter for the base protocol; Open5GS and PyHSS are planned for
   the 3GPP applications) are committed with their source and replayed: they
   must decode, validate and re-encode to the same AVPs. Today that covers
   CEA, DWA and DPA from freeDiameter; the base protocol stays spec-only
   until the other three command pairs have a capture.
5. **Fuzzing.** `Unmarshal` must never panic, and encoding a decoded message,
   decoding that and encoding again must give the same bytes. Every command
   has a fuzz target, and CI runs each for a short budget.
6. **Benchmarks.** At least one command per application is measured against
   go-diameter and published above.

## Defining your own application

Write a proto3 file with go-diam options:

```proto
syntax = "proto3";
package example.myapp;
import "diam/options.proto";
import "ietf/rfc6733/base.proto";
option go_package = "example.com/myapp/myapp";

// AVP dictionary: value number = AVP code.
enum Avp {
  option (diam.dictionary) = true;
  AVP_UNSPECIFIED = 0;
  MY_COUNTER = 5001 [(diam.def) = {vendor: 99999, m: true, type: UNSIGNED32}];
  MY_LABEL   = 5002 [(diam.def) = {vendor: 99999, type: UTF8_STRING}];
}

// MyR ::= < Diameter Header: 8388700, REQ, PXY, 99999 >
//         < Session-Id > { Origin-Host } { Origin-Realm } { My-Counter }
//         [ My-Label ] *[ Route-Record ] *[ AVP ]
message MyRequest {
  option (diam.command) = {code: 8388700, app_id: 99999, request: true, proxiable: true};
  option (diam.extensible) = true;
  string session_id   = 1 [(diam.fixed) = true];  // < Session-Id >
  string origin_host  = 2;                        // { Origin-Host }
  string origin_realm = 3;                        // { Origin-Realm }
  uint32 my_counter   = 4;                        // { My-Counter }
  optional string my_label = 5;                   // [ My-Label ]
  repeated string route_record = 6;               // *[ Route-Record ]
}
```

| ABNF | proto3 |
|---|---|
| `{ X }` | `T x = n;` |
| `[ X ]` | `optional T x = n;` |
| `*[ X ]` | `repeated T x = n;` |
| `1*{ X }` | `repeated T x = n [(diam.min) = 1];` |
| `< X >` | `[(diam.fixed) = true]` |
| `*[ AVP ]` | `option (diam.extensible) = true;` |

Fields find their AVP by name (`my_counter` → `MY_COUNTER`) in the dictionary
enums of the file and its imports. Then generate:

```sh
go install github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam@latest
buf generate   # or: protoc --go-diam_out=. --go-diam_opt=paths=source_relative ...
```

```yaml
# buf.gen.yaml
version: v2
plugins:
  - local: protoc-gen-go-diam
    out: .
    opt: paths=source_relative
```

The files are standard proto3, so `buf lint`, `buf breaking` and editor
support work as usual.

## Roadmap

- **Wave 0:** runtime, generator, base protocol, CI correctness gates.
- **Wave 1:** HSS/UDM/EIR side: S6a/S6d, Cx/Dx, Sh/Dh, S13, SWx, S6t,
  S6m/S6c, SLh, Zh.
- **Wave 2:** policy and charging: Credit-Control, Gx, Rx, Gy/Ro, Rf, Sd,
  Sy, Gq.
- **Wave 3:** S6b, SWm, STa, SLg, T6a/T6b, Np, Nt, NASREQ, EAP.

Releases stay at `v0.x` until wave 1 is complete. The full design is in
[`docs/design.md`](docs/design.md),
and [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to add an application.

Out of scope: transport, peer state machines, routing and DRA logic,
metrics.

## License

Apache-2.0. AVP definitions are transcribed from public 3GPP and IETF
specifications. The freeDiameter (BSD) and go-diameter (BSD-3) dictionaries
are used only to cross-check in tests, as credited in `NOTICE`.
