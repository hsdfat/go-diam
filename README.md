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

> **Status: pre-release, wave 0.** What is in the repository today: the
> runtime (`diam`), the generator (`cmd/protoc-gen-go-diam`) and the RFC 6733
> base protocol (`base`), tested in both directions against
> fiorix/go-diameter. The S6a examples below show the same API on a 3GPP
> application; `s6a` is a prototype on the `spike/proto3-codegen` branch and
> lands in wave 1 (see [Coverage](#coverage)). The API may still change
> before `v1.0.0`.

What works today, with the base protocol:

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
package per application, today `github.com/hsdfat/go-diam/base`. Generated
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
| 3GPP interfaces included | Planned: all (see [Coverage](#coverage)) | Base, Credit-Control, Gx, Gy/Ro/Rf, Rx, Sy, S6a, S13, SWx, NASREQ. Cx/Dx and Sh/Dh are not included |
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
| S6a ULR (12 AVPs, 2 grouped), prototype | Decode | 510 ns/op, 288 B, 9 allocs | 1774 ns/op, 1376 B, 42 allocs |
| S6a ULR, prototype | Encode (reused buffer) | 65 ns/op, 0 B, 0 allocs | 520 ns/op, 200 B, 15 allocs |

CER and STR: Intel i3-12100F, Go 1.26.4, linux/amd64, `-benchtime=200000x`,
on a shared machine under a CPU quota, which is why they are slower in
absolute terms than the ULR rows. The ULR rows were measured earlier on the
same CPU with the prototype on the `spike/proto3-codegen` branch. These are
measurements of single messages on one machine, not a general claim; compare
the two columns of a row, not rows with each other. Reproduce with:

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
| Base protocol (CER/CEA, DWR/DWA, DPR/DPA, RAR/RAA, STR/STA, ASR/ASA) | RFC 6733 | 0 | spec-only: all 12 commands byte-identical with go-diameter in both directions, dictionary cross-checked, fuzzed; freeDiameter captures pending (the `peer` workflow produces them) |
| S6a/S6d | TS 29.272 | 1 | prototype (ULR/ULA) |
| Cx/Dx | TS 29.229 | 1 | planned |
| Sh/Dh | TS 29.329 | 1 | planned |
| S13/S13' | TS 29.272 | 1 | planned |
| SWx | TS 29.273 | 1 | planned |
| S6t | TS 29.336 | 1 | planned |
| S6m/S6c | TS 29.336 / 29.338 | 1 | planned |
| SLh | TS 29.173 | 1 | planned |
| Zh | TS 29.109 | 1 | planned |
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
2. **Dictionary cross-check.** CI compares code, vendor, M-bit and type of
   every AVP with the go-diameter dictionary (freeDiameter's is to follow).
   A disagreement fails the build until it is fixed or listed with a spec
   citation in `internal/interop/testdata/dictcheck-allow.txt`.
3. **Interop.** Each command is built once with go-diameter and once with
   go-diam; the bytes must be identical, and each library must decode and
   re-encode the other's output.
4. **Real captures.** Frames received from open-source peers running in CI
   (freeDiameter for the base protocol; Open5GS and PyHSS are planned for
   the 3GPP applications) are committed with their source and replayed: they
   must decode, validate and re-encode to the same AVPs. No capture is
   committed yet, which is why the base protocol is listed as spec-only; the
   replay test is skipped until the first one is.
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
[`docs/superpowers/specs/2026-11-08-go-diam-design.md`](docs/superpowers/specs/2026-11-08-go-diam-design.md),
and [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to add an application.

Out of scope: transport, peer state machines, routing and DRA logic,
metrics.

## License

Apache-2.0. AVP definitions are transcribed from public 3GPP and IETF
specifications. The freeDiameter (BSD) and go-diameter (BSD-3) dictionaries
are used only to cross-check in tests, as credited in `NOTICE`.
