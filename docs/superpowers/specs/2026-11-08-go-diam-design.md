# go-diam design

Date: 2026-11-08. Status: draft for owner approval.

## 1. Intent

go-diam is an open-source Go library that turns Diameter message definitions,
written as standard proto3 files, into typed Go codecs. Each command and each
grouped AVP becomes a Go struct with `Marshal`, `AppendMarshal`, `Unmarshal`,
and `Validate`. There is no runtime dictionary and no reflection.

Why it should exist next to fiorix/go-diameter:

- go-diameter decodes into a generic AVP tree that is checked against an XML
  dictionary at runtime. Callers walk the tree with `FindAVP` and type
  assertions. Its dictionaries leave out whole 3GPP interfaces (Cx, Sh) and
  many AVPs (S6a is missing about 25).
- go-diam gives compile-time typed fields and generated grammar checks. A
  spike on 2026-11-08 measured, for a ULR on one machine: decode 510 ns / 9
  allocs vs 1774 ns / 42 allocs, encode 65 ns / 0 allocs vs 520 ns / 15 allocs.
- The selling point is "typed, codegen-first Diameter codecs for 3GPP
  interfaces". Do not claim "better than go-diameter" in public text without
  published benchmarks and interop results.

The owner (hsdfat) wrote an earlier internal library on the same idea for a
commercial HSS/UDM. go-diam is a **clean-room rewrite** of the idea. See
Constraints.

## 2. Goals, non-goals, anti-goals

Goals

- G1. Standard proto3 schema language for Diameter, readable by `protoc` and
  `buf` with no forks or custom syntax.
- G2. A generator that produces idiomatic, fast, stdlib-only Go codecs.
- G3. Over time, full coverage of the 3GPP and IETF Diameter applications
  listed in §9. The coverage table must always say what is done.
- G4. Interoperate with real peers built for both older and newer 3GPP
  releases from one schema.
- G5. Correct on the wire. Every shipped interface is verified against an
  independent implementation or a real capture, not only against itself.
- G6. A Rust code backend is possible later without redesigning the schema.

Non-goals (v1)

- Transport: no TCP/SCTP/TLS connections, peer state machine, watchdog,
  failover, connection pools, routing, or DRA logic. Users bring their own
  transport, or use go-diam's codec on top of go-diameter's transport.
- Metrics, logging, middleware.
- A Rust backend (G6 only requires that it remains possible).
- Runtime-loaded dictionaries.

Anti-goals: things that make the project a failure even if it works

- Silent wrong bytes: an AVP code, vendor-id, or M-bit that is wrong but
  passes self round-trip tests.
- A client/server layer creeping back in and repeating the tangled
  transport code the owner wanted to leave behind.
- Any code, proto file, test vector, or capture copied from the owner's
  employer codebase or networks.
- Runtime dependencies beyond the Go standard library.
- Public claims that coverage or correctness are higher than they are.

## 3. Constraints

- **Clean room.** Write every file fresh. Nothing may be copied from
  `gitlab.vht.vn/...` repositories, including `.proto` files, generated code,
  tests, and PCAPs. AVP facts come from public specs (3GPP TS, IETF RFC). The
  2026-11-08 spike on branch `spike/proto3-codegen` was written fresh and may
  be reused.
- **License:** Apache-2.0 for the code. Dictionary data from freeDiameter
  (BSD) and go-diameter (BSD-3) may be used only for cross-checking in tests,
  with attribution in `NOTICE`. **Never use Wireshark dictionaries (GPL).**
  Write the spec transcriptions in our own structure. AVP facts cannot be
  copyrighted, but the layout of ETSI/3GPP tables can be.
- **One part-time maintainer.** Prefer boring, small, testable pieces.
- **Host limits for local work:** builds run inside a `systemd-run`
  MemoryMax=5G scope. No testcontainers on the dev host; container interop
  tests run in CI only.
- **Go versions:** library minimum `go 1.23`. CI tests the minimum and the
  two latest releases.

## 4. Decisions made

Decided by the owner

- D1. The library scope is encoding and decoding (no client/server).
- D2. Definitions are written in standard proto3.
- D3. Go output first. A Rust backend may come later.
- D4. The end goal is all interfaces, not a subset.
- D5. Support both older and recent 3GPP releases.
- D6. The owner delegated the generator language and all remaining design
  calls to a council (four voices: architect, skeptic, pragmatist, critic).

Decided by the council and the spike (owner may override)

- C1. **The generator is written in Go**, as a protoc plugin
  (`protoc-gen-go-diam`) built on `google.golang.org/protobuf/compiler/protogen`.
  All four voices agreed. Reasons: the owner's main language, Go-only
  contributors, `go install`, and protogen handles Go naming and import paths.
  A future Rust backend can be a second plugin that reads the same
  descriptors. The proto files plus options are the language-neutral
  description, so no separate JSON IR is built now.
- C2. **Coverage is shipped in waves (§9), not all at once.** Each interface
  counts as done only when it passes the gate in §8.
- C3. One proto file per application, written against the latest 3GPP
  release (Rel-18 at the time of writing) as a superset. Each AVP carries a
  `since` release note for documentation. **No `-max-release` generator
  flag**, because library users never run the generator.
- C4. Optional scalar AVPs are `diam.Opt[T]` (value plus presence, no
  allocation). Optional grouped AVPs are `*T`. Repeated AVPs are `[]T`.
- C5. **Decoding is lenient and `Validate` is strict.** `Unmarshal` fails
  only when it cannot frame the message. All grammar problems are reported
  by `Validate`.
- C6. Build only small helpers that do not touch the network: `ReadFrame`,
  answer-header construction, and typed CER/CEA, DWR/DWA, DPR/DPA builders
  (these come for free as generated base commands).
- C7. The spike showed that proto3 can express the Diameter ABNF (§5). The
  skeptic's and critic's concern about expressiveness is closed.

## 5. Schema language

### 5.1 Options (`proto/diam/options.proto`, package `diam`)

| Option | On | Meaning |
|---|---|---|
| `(diam.dictionary) = true` | enum | This enum is an AVP dictionary |
| `(diam.def) = {vendor, m, type, since}` | enum value | One AVP. The value number is the AVP code |
| `(diam.command) = {code, app_id, request, proxiable}` | message | This message is a command |
| `(diam.extensible) = true` | message | Grammar ends in `*[ AVP ]` (documentation; see §7) |
| `(diam.fixed) = true` | field | `< AVP >` fixed position |
| `(diam.min)`, `(diam.max)` | repeated field | Occurrence bounds; `max` 0 = unbounded |
| `(diam.avp) = "NAME"` | field | Dictionary entry name when it differs from the field name |
| `(diam.m) = true/false` | field | Overrides the dictionary M-bit for this use only (per-command flag rules, e.g. Supported-Features) |

`diam.Type`: OCTET_STRING, INTEGER32, INTEGER64, UNSIGNED32, UNSIGNED64,
FLOAT32, FLOAT64, GROUPED, ADDRESS, TIME, UTF8_STRING, DIAMETER_IDENTITY,
DIAMETER_URI, ENUMERATED. Derived types not listed here (IPFilterRule,
QoSFilterRule) map to OCTET_STRING or UTF8_STRING, following the spec's
base type.

### 5.2 Grammar mapping

| ABNF | proto3 |
|---|---|
| `{ X }` | `T x = n;` |
| `[ X ]` | `optional T x = n;` |
| `*[ X ]` | `repeated T x = n;` |
| `1*{ X }`, `2*5[ X ]` | `repeated` + `(diam.min)` / `(diam.max)` |
| `< X >` | `(diam.fixed) = true` |
| `*[ AVP ]` | `option (diam.extensible) = true;` |
| AVP order | field number order |

Proto field kind must agree with the dictionary type. The generator rejects
mismatches:

- UTF8_STRING / DIAMETER_IDENTITY / DIAMETER_URI → `string`
- OCTET_STRING → `bytes` (or `string` for text-like octet strings)
- ADDRESS → `bytes` (Go `netip.Addr`)
- TIME → `uint32` (Go `time.Time`)
- UNSIGNED32 → `uint32`, UNSIGNED64 → `uint64`, INTEGER32 → `int32`,
  INTEGER64 → `int64`, FLOAT32 → `float`, FLOAT64 → `double`
- ENUMERATED → a proto enum (or `int32`)
- GROUPED → a message

A grouped message type may be reused under different AVP codes, because the
code comes from the referring field. Example: `ServiceUnit` serves
Granted-, Requested- and Used-Service-Unit.

### 5.3 Dictionary resolution

A field resolves to an AVP by its name in upper case
(`visited_plmn_id` → `VISITED_PLMN_ID`). The generator looks first in
dictionary enums of the same file. If nothing matches there, it uses the
single match among all files in the request. If there are several matches,
it fails and asks for `(diam.avp)`.

The base protocol dictionary lives in one file that other files import.

Flag rules are per AVP in the dictionary. When a spec gives a different
M-bit for one particular use, the field sets `(diam.m)`. dictcheck compares
the dictionary value, and any per-use override must cite its spec clause in
a comment.

### 5.3a Which AVPs are required (release compatibility)

Transcribe each command's ABNF from the latest release as written, with one
exception. If an AVP is required (`{ }`) in a newer release but optional or
absent in an older supported release, mark it optional (`[ ]`). Give its
release history in a comment.

The oldest supported release is **Rel-8**, the start of EPC, for 3GPP
applications. A Rel-8 peer's message must therefore pass `Validate`.

In practice 3GPP almost always adds new AVPs as optional, so exceptions
should be rare. Each one is listed in the file header.

### 5.4 Enums

- Proto3 requires a zero value. When a Diameter enumeration has no 0, add
  `<PREFIX>_UNSPECIFIED = 0`. The generator omits it.
- Value names carry the enum's SCREAMING prefix (a proto scoping rule). The
  generator strips it: `RAT_TYPE_EUTRAN` → `RatTypeEutran`.
- Values unknown to the schema (a peer on a newer release) decode as the raw
  number and are not an error.

### 5.5 File layout and naming

```
proto/diam/options.proto
proto/ietf/rfc6733/base.proto       proto package godiam.base -> Go package base
proto/ietf/rfc8506/cc.proto         -> cc
proto/3gpp/ts29272/s6a.proto        -> s6a
proto/3gpp/ts29229/cx.proto         -> cx
...
```

Each file starts with a header comment giving the spec number, the version
transcribed (for example `TS 29.272 V18.x.0`), and the sections or tables
covered. Each command and grouped message carries its ABNF from the spec as a
leading comment.

## 6. Generated Go API

Example: S6a ULR (shape confirmed by the spike).

```go
type UpdateLocationRequest struct {
    Header diam.Header
    SessionId                   string                            // < >
    VendorSpecificApplicationId *base.VendorSpecificApplicationId // [ ]
    AuthSessionState            base.AuthSessionState             // { }
    DestinationHost             diam.Opt[string]                  // [ ]
    SupportedFeatures           []SupportedFeatures               // *[ ]
    ...
    Unknown []diam.RawAVP // unknown AVPs, re-encoded verbatim
    // unexported: required-seen bitmask, excess occurrences, decode errors
}
func (m *UpdateLocationRequest) Marshal() ([]byte, error)
func (m *UpdateLocationRequest) AppendMarshal(b []byte) ([]byte, error)
func (m *UpdateLocationRequest) Unmarshal(b []byte) error
func (m *UpdateLocationRequest) Validate() error
func (m *UpdateLocationRequest) DiamHeader() *diam.Header
```

Grouped types expose `AppendAVPs`, `DecodeAVPs`, and `Validate` so that
types from other packages can be nested in them. They satisfy
`diam.Group`. A violation found inside a nested group is returned as an
`*AVPError` whose `AVP` is the innermost offending AVP, and whose `Path`
lists the enclosing grouped AVP codes. Callers can then build a nested
Failed-AVP. Commands register in `init` so that
`diam.Decode(b)` can return the right type from (app-id, command code, R-bit).
Go field names follow protogen (`SessionId`). Go acronym style
(`SessionID`) is not used.

Encoding rules:

- Fixed-position AVPs are emitted first, then the rest in field order, then
  `Unknown`.
- Required scalar fields are always emitted, including zero values: CC-Request-Number 0 is valid.
- `AppendMarshal` sets the command code and app-id from the schema and forces
  the R and P bits to the schema's values. Other header fields come from the
  user.
- The AVP V-bit is set exactly when vendor != 0. The M-bit comes from the
  dictionary or the field override. The AVP P-bit (protected) is never set.
- Header bits: R is forced from `(diam.command).request` and P (proxiable)
  from `(diam.command).proxiable`. E (error) and T (retransmit) are left as
  the user set them. Hop-by-Hop and End-to-End IDs are always the user's;
  the codec never allocates them.
- Every command pair gets a generated `New<Answer>For(req) *<Answer>`. It
  copies Session-Id and the header IDs, clears R, and keeps P. It is built on
  `diam.AnswerHeader`.
- Strings and bytes copy on decode. Decoded messages never alias the input
  buffer.

## 7. Runtime (`github.com/hsdfat/go-diam/diam`)

Standard library only. Contents:

- `Header`, `Message` interface, `Opt[T]`/`Some`, `RawAVP`.
- Framing: `AppendHeader`, `FinishMessage`, `ParseHeader`, `ReadFrame`,
  `BeginAVP`/`EndAVP`, `NextAVP`, `AppendRaw`.
- Base data types: encode and decode for every `diam.Type`. Time handles the
  RFC 6733 §4.3.1 2036 wrap. Address supports IPv4 and IPv6. Other address
  families decode as an error that Validate reports with 5004.
- `AnswerHeader(req)` (and `NewAnswer` helpers per command pair, generated
  when the schema pairs a request with an answer of the same code).
- `Register`/`Decode`.
- Result-Code constants and `AVPError{ResultCode, AVP, Msg}`.

### Error model (`Validate`)

`Validate` returns the first violation as an `*AVPError` that carries the
Result-Code and the offending AVP, ready for a Failed-AVP:

| Condition | Result-Code |
|---|---|
| Unknown AVP with M-bit (in any command or group) | 5001 AVP_UNSUPPORTED |
| Bad value (address family, enum not required to be known) | 5004 INVALID_AVP_VALUE |
| Required AVP missing (decoded messages only) | 5005 MISSING_AVP |
| Too many occurrences | 5009 AVP_OCCURS_TOO_MANY_TIMES |
| Bad AVP data length for its type, including inside a group | 5014 INVALID_AVP_LENGTH |

- Unknown AVPs without the M-bit are kept and ignored, per RFC 6733 §4.1.
- Unknown AVPs with the M-bit always give 5001, including ones sent by a
  newer-release peer. That is the RFC behaviour. An application that wants
  to tolerate specific ones inspects `Unknown` itself. No allow-list hook in
  v1.
- `extensible` stays as documentation. It does not change validation, because
  RFC 6733 handles unknown AVPs the same way inside and outside `*[ AVP ]`.
- **This changes the spike's behaviour.** Bad data inside a grouped AVP must
  not abort `Unmarshal`. The decoder records the error with its AVP, keeps
  going, and `Validate` reports 5014 or 5004.
- Fixed-position AVPs are checked for position by `Validate` only when
  `(diam.fixed)` is set. A misplaced Session-Id yields 5004 with that AVP.
- Required-presence on hand-built messages is not checked. The zero value
  is a valid encoding for numbers, so there is nothing to check.

## 8. Correctness gate

An interface is "done" (and enters the coverage table as such) only when all
of these hold:

1. **Transcription.** Every AVP, command, and grouped ABNF comes from the
   spec version named in the file header.
2. **Dictionary cross-check (CI).** A test-only tool compares code, vendor,
   M/V flags, and type for every AVP that also appears in the freeDiameter or
   go-diameter dictionaries. Disagreements fail CI until they are resolved in
   a reviewed allow-list that cites the spec clause.
3. **Independent interop.** Both directions with go-diameter, as in the
   spike. go-diameter's base dictionary covers every base command, so base
   uses it too.
4. **Real captures.** At least one golden capture per command pair decodes
   with `Validate() == nil` and re-encodes byte-identically. Allowed sources:
   - captures generated in CI against open-source peers (freeDiameter test
     apps, Open5GS HSS/PCRF, PyHSS) in containers;
   - public sample captures whose license allows redistribution;
   - hand-built vectors checked against the spec.

   No captures from any operator or employer network.
   Equality rule: our encoder emits grammar order, so byte-identical
   re-encode is required only for inputs that are already in grammar order
   (go-diameter output, our own vectors). For real-peer captures the gate is
   semantic: decode → encode → decode gives an equal struct, and the multiset
   of AVPs (code, vendor, flags, data) is preserved. Original AVP order is not
   stored.
5. **Fuzzing.** `Unmarshal` never panics, and decode → encode → decode is
   stable. Go native fuzz runs in CI, with a short budget per command.
6. **Benchmarks.** A benchmark against go-diameter for at least one command
   of the interface. Numbers are published in the README with the machine
   and Go version.

Coverage table statuses:

- **verified** — gates 1–6, with gate 3 or 4 satisfied by an independent
  implementation or capture.
- **spec-only** — gates 1, 2, 5 pass, but the only vectors are hand-built,
  because no open-source peer exists (likely S13, SLh, Zh, S6t, S6m/S6c,
  parts of Sh).
- **partial** — some commands of the application are missing.

The README shows the status per interface. A spec-only interface is
published, but never described as verified.

## 9. Roadmap

- **Wave 0 (foundation):** options, runtime, generator, base protocol
  (CER/CEA, DWR/DWA, DPR/DPA, ASR/ASA, STR/STA, RAR/RAA), the CI pipeline
  for gates 2–6, coverage table, README.
- **Wave 1 (HSS/UDM/EIR side):** S6a/S6d (TS 29.272, including full
  Subscription-Data), Cx/Dx (29.229), Sh/Dh (29.329), S13/S13' (29.272 §7),
  SWx (29.273), S6t (29.336), S6m/S6c (29.336/29.338), SLh (29.173),
  Zh (29.109).
- **Wave 2 (policy and charging):** Credit-Control (RFC 8506), Gx (29.212),
  Rx (29.214), Gy/Ro (32.299), Rf (32.299), Sd (29.212), Sy (29.219),
  Gq (29.209).
- **Wave 3 (rest):** S6b, SWm, STa (29.273), SLg (29.172), T6a/T6b (29.128),
  Np (29.217), Nt (29.154), NASREQ (RFC 7155), EAP (RFC 4072).

Order inside a wave: the builder decides, but S6a comes first in wave 1.
Releases: `v0.x` until wave 1 is done, then `v1.0.0`. Tag at the end of
each wave.

## 9a. Provenance (clean room)

- Each proto file header cites spec number, version, and clauses. Each
  command and grouped message carries its spec ABNF.
- Do not open the employer's library while writing go-diam. Git history is
  the provenance log; no separate log is kept.
- The spike branch counts as clean, because it was written fresh in a session
  with no access to that code's proto files. Its `.proto` files and
  generator may be the starting point for wave 0. Its `base.proto` is
  extended to all six base command pairs, not rewritten.

## 10. Repository and modules

```
go.mod                         module github.com/hsdfat/go-diam (runtime + generated, stdlib only)
diam/                          runtime
base/ cc/ s6a/ cx/ ...         generated packages (committed)
proto/                         schema (§5.5)
cmd/protoc-gen-go-diam/        generator: its OWN go.mod (depends on protobuf);
                               holds internal/diampb (Go bindings of options.proto,
                               generated by protoc-gen-go via buf.gen.options.yaml)
internal/interop/              interop + capture tests: its OWN go.mod (depends on go-diameter)
tools/dictcheck/               cross-check tool: its OWN go.mod
testdata/captures/<iface>/     golden captures with a SOURCE file each
buf.yaml, buf.gen.yaml
LICENSE (Apache-2.0), NOTICE, README.md, CONTRIBUTING.md
```

- The root module must not require protobuf or go-diameter. A committed
  `go.work` ties the modules together for development. Users who write their
  own `.proto` files import `diam/options.proto` from the repo's `proto/`
  (or from the BSR, if published); they never need the Go bindings.
- Pinned tools: the buf CLI version is pinned in CI. protoc-gen-go is pinned
  in the generator's go.mod. protoc-gen-go-diam is built from the same
  commit. The regenerate-and-diff job uses exactly these.
- CI: `buf lint`, `buf breaking` against the last tag, regenerate and fail on
  diff, `go vet`, `go test -race`, fuzz smoke, dictcheck, interop, and
  containerised peer captures (scheduled job).
- Commit messages are plain: no AI attribution footers.

## 11. Left to the builder

- Internal structure of the generator (one file or several, templates or
  `g.P`).
- How unexported decode bookkeeping is stored (bitmask width, error list).
- Exact names of helper functions beyond those listed here.
- Benchmark machine, fuzz budgets, and CI provider details (GitHub Actions
  expected).
- Order of interfaces inside a wave, after S6a.
- README wording, within the positioning rule in §1.

## 12. Deviations recorded while building wave 0

Decisions taken during implementation where this document said otherwise.
The text above is unchanged; these win.

- **§10, dictcheck location.** The cross-check is a test,
  `internal/interop/dictcheck_test.go`, not a `tools/dictcheck/` module. The
  interop module already depends on go-diameter, and a test needs no extra
  binary in CI. It compares against go-diameter only; freeDiameter's
  dictionary is still to be added.
- **§10, diampb path.** The Go bindings of `options.proto` are at
  `cmd/protoc-gen-go-diam/diampb`, not under `internal/`, because the
  cross-check in `internal/interop` reads the options too.
- **§5.3, dictionary resolution.** A field resolves in its own file first,
  then in the files that file imports (directly or not), not in "all files
  in the request". A file that is merely generated in the same run must not
  supply an AVP code.
- **§7, groups without named AVPs.** A message with no fields, such as
  `Failed-AVP ::= 1* {AVP}`, is a container: its `Validate` always succeeds
  and the M-bit rule for unknown AVPs does not apply to its content.
- **§7, V-bit with Vendor-Id 0.** Such an AVP is not the IETF AVP of that
  code. It goes to `Unknown` and is re-encoded with its V-bit.
- **§7, AVPError.** It has a `Path` field (enclosing grouped AVP codes,
  outermost first), as §6 describes.
- **§3, Go versions.** The root module is `go 1.23`. `internal/interop` and
  `go.work` are `go 1.26` because go-diameter v4.6.0 requires it; CI tests
  the root module without the workspace.
- **Base protocol content.** No E2E-Sequence (RFC 6733 deprecated it) and
  Inband-Security-Id is Unsigned32, as in the RFC.

## 13. Deviations recorded while building wave 1

- **§5.5 / C3, re-used AVPs (S6a).** AVPs that an application re-uses from
  other specifications are transcribed into that application's own
  dictionary, not imported from a shared file. TS 29.272 table 7.3.1/2
  gives many re-used AVPs another M-bit than their defining specification
  (NOTE 1: it overrides), and `(diam.m)` cannot reach AVPs nested inside a
  grouped type defined elsewhere. The defining clause is cited per entry.
- **§5.1, one enum per vendor where codes collide.** The dictionary enum
  value is the AVP code, so an IETF and a 3GPP AVP with the same code
  (625, 626) cannot share an enum. A file may hold several dictionary enums
  (`Avp` and `IetfAvp` in `s6a.proto`).
- **§5.3, names starting with a digit.** Proto identifiers cannot start
  with a digit: 3GPP-Charging-Characteristics, 3GPP2-MEID and 5QI are
  `TGPP_CHARGING_CHARACTERISTICS`, `TGPP2_MEID` and `FIVE_QI`; dictcheck
  applies the same rule to go-diameter's names.
- **§8 gate 2, absent entries.** Dictionary entries that go-diameter does
  not have are listed exactly in
  `internal/interop/testdata/dictcheck-absent.txt`, so the README can say
  how much of an interface the cross-check covers.
- **S6a release.** S6a is transcribed from TS 29.272 V18.7.0 (Release 18,
  the latest Release 18 version published), not Release 19.
- **Cx release.** Cx/Dx is transcribed from TS 29.229 V18.1.0, the latest
  Release 18 version, with TS 29.228 V18.0.0 for the information elements.
  The S6a rulings apply unchanged: re-used AVPs in the application's own
  dictionary (`Avp`, `EtsiAvp` for Line-Identifier, `IetfAvp`), and the
  Release 8 `*[ Failed-AVP ]` is not carried over (Release 14 made it
  `[ Failed-AVP ]`).
- **§5.3a, an AVP dropped from a newer grammar (Cx RTR).** TS 29.229
  V18.1.0 lists no Destination-Host in RTR, while TS 29.229 V8.15.0 has
  `{ Destination-Host }` and TS 29.228 V18.0.0 table 6.1.3.1 maps it with
  category M. A Release 8 RTR would otherwise put an M-bit AVP in Unknown
  and fail with 5001. Such an AVP is kept at its older position as
  optional, with the reason in the file header. §5.3a covers the opposite
  case (newly required becomes optional); this is its mirror.
- **§5.3, per-message M-bit (Cx Supported-Features).** Where the flag table
  says "May" and the procedure fixes the bit per message (TS 29.229 §7.2.1:
  set in requests, clear in answers), the dictionary has M clear and every
  request field sets `(diam.m) = true`.
- **§8 gate 3, no dictionary on the independent side.** go-diameter v4.6.0
  has no Cx application. Its side of the Cx interop is built from the Cx
  AVPs it defines under other applications, and go-diam's output is decoded
  AVP by AVP in the application that defines each one. The AVPs it defines
  nowhere are listed in `internal/interop/cx_test.go` (`cxRestated`) with
  code, vendor and M-bit restated from the specification; for them only
  the value encoding is independent. A test keeps the list exact. No
  benchmark is published for Cx, since go-diameter would decode it as
  opaque AVPs.
- **Sh release.** Sh/Dh is transcribed from TS 29.329 V18.0.0, the latest
  Release 18 version, with TS 29.328 V18.2.0 for the procedures and
  TS 29.329 V8.9.0 as the Release 8 reference. The S6a and Cx rulings apply
  unchanged: re-used AVPs in the application's own dictionary (`Avp` with
  External-Identifier from TS 29.336, `IetfAvp` because code 650 is both
  Load and Session-Priority), `[ Failed-AVP ]` instead of the Release 8
  `*[ Failed-AVP ]`, Supported-Features with the M-bit in requests only
  (TS 29.329 §7.1 applies TS 29.229 §7.2.1), and the interop of §8 gate 3
  built AVP by AVP (`shRestated` in `internal/interop/sh_test.go`), since
  go-diameter v4.6.0 has no Sh application. No benchmark for Sh either.
- **§5.2, `*{ X }`.** TS 29.329 writes `*{ Data-Reference }` in UDR, PUR
  and SNR. RFC 6733 §3.2 makes the default minimum of a required rule one,
  so it is `repeated` with `(diam.min) = 1`, and a message without
  Data-Reference fails with 5005.
- **§7, procedure rules are not grammar (Sh wildcarded identities).**
  TS 29.328 says a request carries Wildcarded-IMPU or
  Wildcarded-Public-Identity, not both. The grammar allows both and the
  rule has no Result-Code of its own, so `Validate` does not enforce it;
  both AVPs decode into their fields and the application decides. The same
  holds for User-Identity carrying "either" one identity.
- **S13 release, and a dictionary imported, not repeated.** S13/S13' is
  transcribed from TS 29.272 V18.7.0, the S6a version, with V8.14.0 as the
  Release 8 reference. Table 7.3.1/1 is one table for S6a/S6d, S7a/S7d and
  S13/S13', with one flag rule per AVP, so the reason re-used AVPs go in the
  application's own dictionary (another M-bit in another application) does
  not arise: `s13.proto` has no dictionary and imports `s6a.proto`. Its six
  AVPs resolve to the S6a entries through the import, and the generated
  `s13` package uses `s6a.TerminalInformation`, `s6a.EquipmentStatus` and
  `s6a.Drmp`; the generator needed no change. The interop of §8 gate 3 is
  built from go-diameter's own S13 dictionary, like S6a, and ECR has a
  benchmark.
- **§5.5, one proto package per directory.** buf's STANDARD lint
  (DIRECTORY_SAME_PACKAGE) rejects two packages in one directory, and
  `godiam.s6a` owns `proto/3gpp/ts29272`. A second application of the same
  specification gets a subdirectory named after its package:
  `proto/3gpp/ts29272/s13/s13.proto`. The Go output path follows
  `go_package` (`s13/`), so the subdirectory does not show in the module.
- **§7, procedure rules are not grammar (S13).** Table 6.2.1.1/1 and
  §6.2.1.2 say Terminal-Information carries IMEI and no 3GPP2-MEID on S13,
  and table 6.2.1.1/2 makes Equipment-Status conditional on
  DIAMETER_SUCCESS. The grammars (§7.3.3, §7.2.20) have all of these
  optional and the rules have no Result-Code of their own, so `Validate`
  checks the grammar only.
