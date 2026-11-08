# go-diam Wave 0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the 2026-11-08 spike into the wave 0 foundation:
- the stdlib-only runtime;
- the `protoc-gen-go-diam` generator;
- the complete RFC 6733 base protocol package;
- the CI correctness gates 2–6 from the design.

**Architecture:** Three Go modules tied together by a committed `go.work`:

- root `github.com/hsdfat/go-diam`: runtime in `diam/` and generated packages, stdlib only;
- `cmd/protoc-gen-go-diam`: the generator, with `diampb`, depends on protobuf;
- `internal/interop`: go-diameter interop, dictionary cross-check, peer tests and benchmarks.

Generated code is committed. `make generate` rebuilds it, and CI fails if regenerating produces a diff.

**Tech Stack:**
- Go: root `go 1.23`, other modules may use newer.
- `google.golang.org/protobuf` v1.36.12 (protogen).
- buf CLI 1.47.2.
- `github.com/fiorix/go-diameter/v4` v4.6.0, used in tests only.
- GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-11-08-go-diam-design.md`. Executors read it alongside this plan. It wins on any conflict except the one deviation noted in Task 9.

**Prototype source:** branch `spike/proto3-codegen`. "Port from spike X" means start from `git show spike/proto3-codegen:X` and apply the listed changes. The spike is clean-room code and may be reused (spec §9a).

## Global Constraints

- The root module requires **nothing** outside the standard library. `go list -m all` in the root prints exactly one line.
- Root `go.mod` says `go 1.23`. Do not use language or library features newer than 1.23 in the root module.
- License Apache-2.0. Add a `NOTICE` that credits the freeDiameter (BSD) and go-diameter (BSD-3) dictionaries, used for cross-checking only.
- **Never** copy from `gitlab.vht.vn` code, protos, tests or captures. **Never** use Wireshark dictionaries (GPL).
- AVP facts come from RFC 6733 text. When this plan lists a value from memory and the RFC disagrees, the RFC wins. Note the correction in the proto comment.
- Commit messages are plain conventional commits. **No** `Co-Authored-By`, "Generated with", or session-link footers.
- Local builds and tests on the dev host run as `systemd-run --user --scope -q -p MemoryMax=5G <cmd>`. No containers on the dev host; container work runs in CI only.
- Never push. The owner decides when `main` goes to GitHub.
- Generated Go field names follow protogen (`SessionId`, `HostIpAddress`).

## Review Focus

1. **Lying length fields.** A message length larger than the buffer, an AVP length smaller than its header, or a grouped AVP whose inner lengths overrun it must return an error or record 5014. They must never panic or read out of bounds. Tests are in Task 2 (`TestNextAVPMalformed`) and Task 5 (`TestGroupedOverrun`).
2. **Reused structs.** `Unmarshal` into a struct that already holds a previous message must leave no old fields, `Unknown` entries or bookkeeping behind. Test: Task 4, `TestUnmarshalResets`.
3. **Same code, different vendor.** An AVP whose code matches a field but whose vendor differs (or whose V-bit differs) belongs in `Unknown`, not in the field. Test: Task 4, `TestVendorMismatchIsUnknown`.
4. **Empty values.** An empty grouped AVP (length 8), an empty string and an empty OctetString must decode as present and re-encode identically. Test: Task 4, `TestEmptyValuesPreserved`.
5. **Padding variants.** A last AVP without its padding bytes, and non-zero padding bytes, must decode the same as clean input. Re-encode emits zero padding. Test: Task 2, `TestNextAVPPadding`.

---

## File structure

```
go.mod  go.work  Makefile  buf.yaml  buf.gen.yaml  buf.gen.options.yaml
LICENSE  NOTICE  README.md  CONTRIBUTING.md  .gitignore
proto/diam/options.proto
proto/ietf/rfc6733/base.proto
proto/gentest/v1/types.proto          # generator fixture (all types and cardinalities)
proto/gentest/v1/nested.proto         # fixture importing base, cross-package groups
diam/header.go        diam/header_test.go     # Header, flags, AppendHeader, FinishMessage, ParseHeader, ReadFrame, AnswerHeader
diam/avp.go           diam/avp_test.go        # RawAVP, BeginAVP, EndAVP, NextAVP, AppendRaw, Clone
diam/types.go         diam/types_test.go      # base data type encode/decode, Opt, Some
diam/errors.go        diam/errors_test.go     # AVPError, result codes, Missing/TooMany/TooFew/Misplaced/CheckUnknown/InGroup
diam/message.go       diam/message_test.go    # Message, Group, Register, Decode
diam/fuzz_test.go
internal/diamtest/diamtest.go                 # FuzzMessage helper, multiset compare
internal/gentest/   (generated + *_test.go)
base/               (generated base.diam.go + base_test.go + fuzz_test.go)
cmd/protoc-gen-go-diam/go.mod
cmd/protoc-gen-go-diam/main.go        # entry, request wiring
cmd/protoc-gen-go-diam/dict.go        # dictionary collection and lookup
cmd/protoc-gen-go-diam/model.go       # field model: cardinality, Go types, flags
cmd/protoc-gen-go-diam/emit_types.go  # enums, dict consts, structs
cmd/protoc-gen-go-diam/emit_codec.go  # AppendAVPs, DecodeAVPs
cmd/protoc-gen-go-diam/emit_validate.go
cmd/protoc-gen-go-diam/emit_command.go # header methods, Register, New<Answer>For
cmd/protoc-gen-go-diam/main_test.go   # negative tests
cmd/protoc-gen-go-diam/diampb/options.pb.go
internal/interop/go.mod
internal/interop/interop_test.go  bench_test.go  dictcheck_test.go  captures_test.go  peer_test.go
internal/interop/testdata/dictcheck-allow.txt
testdata/captures/base/   (frames + SOURCE)
ci/freediameter/Dockerfile  ci/freediameter/freeDiameter.conf
.github/workflows/ci.yml  .github/workflows/peer.yml
```

---

### Task 1: Repository skeleton, modules and options schema

**Files:**
- Create: `go.mod`, `go.work`, `Makefile`, `buf.yaml`, `buf.gen.yaml`, `buf.gen.options.yaml`, `LICENSE`, `NOTICE`, `.gitignore`
- Create: `proto/diam/options.proto` (port from spike `proto/diam/options.proto`)
- Create: `cmd/protoc-gen-go-diam/go.mod`, `cmd/protoc-gen-go-diam/diampb/options.pb.go` (generated)

**Interfaces:**
- Produces:
  - Module paths `github.com/hsdfat/go-diam`, `github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam`, `github.com/hsdfat/go-diam/internal/interop`.
  - Package `github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb`. It is exported so that `internal/interop` can read the options too.
  - Make targets `generate`, `test`, `lint`, `check-generated`.

- [ ] **Step 1: Port `options.proto` with two changes**
  - Set `option go_package = "github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb";`.
  - Add the per-use M-bit override: `extend google.protobuf.FieldOptions { optional bool m = 50124; }`, with the comment "overrides the dictionary M-bit for this use only; cite the spec clause". Read it with `proto.HasExtension`.
- [ ] **Step 2: Write the module files**
  - Root `go.mod`: `module github.com/hsdfat/go-diam`, `go 1.23`.
  - `cmd/protoc-gen-go-diam/go.mod`: requires `google.golang.org/protobuf v1.36.12`.
  - `internal/interop/go.mod`: requires go-diameter v4.6.0 and protobuf v1.36.12.
  - `go.work`: lists the three modules.
- [ ] **Step 3: Write the buf config**
  - `buf.yaml` (v2, module path `proto`, lint `STANDARD` with `ENUM_ZERO_VALUE_SUFFIX` and `PACKAGE_VERSION_SUFFIX` excepted).
  - `buf.gen.options.yaml`: `protoc-gen-go`, `out: cmd/protoc-gen-go-diam`, `opt: module=github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam`, path `proto/diam`.
  - `buf.gen.yaml`: local plugin `bin/protoc-gen-go-diam`, `out: .`, `opt: module=github.com/hsdfat/go-diam`.
- [ ] **Step 4: Write the Makefile**
  - `generate`:
    1. Run `buf generate --template buf.gen.options.yaml --path proto/diam`.
    2. Run `go build -o bin/protoc-gen-go-diam ./cmd/protoc-gen-go-diam`.
    3. Run `buf generate --exclude-path proto/diam`.
  - `check-generated`: runs `generate`, then `git diff --exit-code`.
  - `test`: runs `go test ./... ./cmd/protoc-gen-go-diam/... ./internal/interop/...`.
  - `lint`: runs `buf lint` and `go vet` for the three modules.
  - `.gitignore`: `bin/`.
- [ ] **Step 5: Verify**
  - `make generate` exits 0, and `cmd/protoc-gen-go-diam/diampb/options.pb.go` exists.
  - `buf lint` passes.
  - `go list -m all` in the root prints only `github.com/hsdfat/go-diam`.
- [ ] **Step 6: Add `LICENSE` (Apache-2.0 text) and `NOTICE`, then commit** `chore: repository skeleton and options schema`

---

### Task 2: Runtime framing and data types

**Files:**
- Create: `diam/header.go`, `diam/avp.go`, `diam/types.go` (port from spike `diam/diam.go`, split by responsibility)
- Test: `diam/header_test.go`, `diam/avp_test.go`, `diam/types_test.go`

**Interfaces:**
- Produces (exact):
  - `type Header struct{ Version, Flags uint8; CommandCode, AppID, HopByHop, EndToEnd uint32 }`
  - Constants `FlagRequest, FlagProxiable, FlagError, FlagRetransmit`, `AVPFlagVendor, AVPFlagMandatory, AVPFlagProtected uint8`, `HeaderLen = 20`.
  - `AppendHeader(b []byte, h *Header) []byte`. Version 0 is written as 1.
  - `FinishMessage(b []byte, start int) ([]byte, error)`. It errors if the message is over 0xFFFFFF bytes.
  - `ParseHeader(b []byte) (Header, []byte, error)`, `ReadFrame(r io.Reader) ([]byte, error)`, `AnswerHeader(req *Header) Header`.
  - `type RawAVP struct{ Code uint32; Flags uint8; Vendor uint32; Data []byte }` with `Mandatory() bool`, `Clone(a RawAVP) RawAVP`.
  - `BeginAVP(b []byte, code uint32, flags uint8, vendor uint32) ([]byte, int)`, `EndAVP(b []byte, start int) []byte`, `AppendRaw(b []byte, a RawAVP) []byte`, `NextAVP(b []byte) (RawAVP, []byte, error)`.
  - `Opt[T any] struct{ V T; Set bool }`, `Some[T any](v T) Opt[T]`, `(Opt[T]) Get() (T, bool)`.
  - Encode functions: `AppendUnsigned32/Unsigned64/Integer32/Integer64/Float32/Float64/Time/Address`.
  - Decode functions `Unsigned32/Unsigned64/Integer32/Integer64/Float32/Float64/String/Bytes/Time/Address(a RawAVP) (T, error)`. They return `*AVPError`, whose type is defined in Task 3. For this task, temporarily declare `AVPError` in `errors.go` with only `ResultCode`, `AVP` and `Msg`.
  - `ErrMalformed`.

- [ ] **Step 1: Write failing tests**
  - `TestHeaderRoundTrip`: all fields survive Append → Finish → Parse. Version 0 encodes as 1.
  - `TestParseHeaderRejects`: length under 20, length over the buffer, and length not a multiple of 4 each return `ErrMalformed`.
  - `TestReadFrame`: reads two back-to-back messages from a `bytes.Reader`. A short read returns `io.ErrUnexpectedEOF`.
  - `TestAnswerHeader`: from flags `R|P|T` the answer has flags `P`. IDs, code and app are copied.
  - `TestNextAVPMalformed` (Review Focus 1): AVP length 7, vendor flag with only 10 bytes, and length greater than the remaining bytes each return `*AVPError` with ResultCode 5014. None panic.
  - `TestNextAVPPadding` (Review Focus 5): the last AVP without padding decodes. Non-zero padding bytes are ignored. `AppendRaw` emits zero padding.
  - `TestTimeWrap`: NTP value `0x80000000` decodes to 1968-01-20T03:14:08Z. Value `0x00000001` decodes into 2036 (RFC 6733 §4.3.1). `AppendTime` of `2026-11-08T10:00:00Z` round-trips.
  - `TestAddress`: IPv4 and IPv6 round-trip. Family 8 (E.164) returns ResultCode 5004. Family 1 with 5 bytes of data returns 5014.
  - `TestIntegerSigns`: `Integer32(-2)` and `Integer64(-12345)` round-trip. A 3-byte Unsigned32 returns 5014.
- [ ] **Step 2: Run `go test ./diam/` and confirm it fails to compile or fails**
- [ ] **Step 3: Port the code from the spike into the three files to satisfy the interfaces**
- [ ] **Step 4: Run `go test -race ./diam/` and confirm PASS**
- [ ] **Step 5: Commit** `feat(diam): framing and base data types`

---

### Task 3: Runtime errors, validation helpers and registry

**Files:**
- Modify: `diam/errors.go` (Task 2 left a minimal `AVPError` there)
- Create: `diam/message.go`
- Test: `diam/errors_test.go`, `diam/message_test.go`

**Interfaces:**
- Consumes: Task 2.
- Produces (exact):
  - `type AVPError struct{ ResultCode uint32; AVP RawAVP; Path []uint32; Msg string }`. `Path` holds the enclosing grouped AVP codes, outermost first. `Error()` includes all fields.
  - Constants: `ResultAVPUnsupported=5001, ResultInvalidAVPValue=5004, ResultMissingAVP=5005, ResultAVPOccursTooMany=5009, ResultInvalidAVPLength=5014, ResultInvalidMessageLength=5015`.
  - `Missing(code, vendor uint32, name string) error` returns 5005.
  - `TooMany(a RawAVP, name string) error` returns 5009.
  - `TooFew(code, vendor uint32, name string, min int) error` returns 5005.
  - `Misplaced(a RawAVP, name string) error` returns 5004.
  - `CheckUnknown(unknown []RawAVP) error` returns 5001 for the first one with the M-bit.
  - `InGroup(err error, code uint32) error`. If err is `*AVPError`, it returns a copy with `code` prepended to `Path`. Otherwise it returns err unchanged.
  - `type Message interface{ DiamHeader() *Header; AppendMarshal([]byte) ([]byte, error); Unmarshal([]byte) error; Validate() error }`.
  - `type Group interface{ AppendAVPs([]byte) []byte; DecodeAVPs([]byte) error; Validate() error }`.
  - `Register(appID, code uint32, request bool, newFn func() Message)`, which panics on a duplicate registration.
  - `Decode(b []byte) (Message, error)`.

- [ ] **Step 1: Write failing tests**
  - `TestInGroupNesting`: `InGroup(InGroup(TooMany(a,"x"), 1401), 1400)` has `Path == []uint32{1400, 1401}` and ResultCode 5009. A non-`AVPError` passes through unchanged.
  - `TestCheckUnknown`: an M-clear entry gives nil. An M-set entry in the second position is returned with code 5001 and that AVP.
  - `TestRegisterDuplicatePanics`.
  - `TestDecodeUnregistered`: the error message names the app, command and R-bit.
  - `TestDecodeDispatch`: register a test `Message` stub for (99, 1, true). `Decode` returns that type and calls `Unmarshal`.
- [ ] **Step 2: Run `go test ./diam/` and confirm FAIL**
- [ ] **Step 3: Implement**, porting from the spike `diam/diam.go` and `diam/registry.go`
- [ ] **Step 4: Run `go test -race ./diam/` and confirm PASS**
- [ ] **Step 5: Commit** `feat(diam): AVPError paths, validation helpers, registry`

---

### Task 4: Generator port with fixture coverage of every type and cardinality

**Files:**
- Create: `cmd/protoc-gen-go-diam/{main,dict,model,emit_types,emit_codec,emit_validate,emit_command}.go` (port from spike `cmd/protoc-gen-go-diam/main.go`, split by the responsibilities in the file structure)
- Create: `proto/gentest/v1/types.proto`
- Generated: `internal/gentest/types.diam.go`
- Test: `internal/gentest/types_test.go`, `cmd/protoc-gen-go-diam/main_test.go`

**Interfaces:**
- Consumes: Tasks 1–3 (runtime names exactly as listed there).
- Produces, as the generated API every later package relies on:
  - Per message: struct with fields per spec §6, then `Unknown []diam.RawAVP`, then unexported bookkeeping.
  - Methods `AppendAVPs(b []byte) []byte`, `DecodeAVPs(data []byte) error`, `Validate() error` (satisfies `diam.Group`).
  - Commands additionally get `DiamHeader`, `AppendMarshal`, `Marshal() ([]byte, error)`, `Unmarshal(b []byte) error`, and registration in `init`.
  - Per non-dictionary enum: `type <Enum> int32`, constants `<Enum><Value>` (prefix stripped, `*_UNSPECIFIED` at 0 omitted) and `String()`.
  - Per dictionary enum: constants `AVP<Name> uint32`.

`types.proto` fixture (package `gentest.v1`, go_package `github.com/hsdfat/go-diam/internal/gentest`, vendor 99999 for every AVP) must contain:
- a dictionary covering all 14 `diam.Type` values;
- a group `AllTypes` with one required field per type;
- a group `Cards` with `optional`, `repeated`, `repeated [(diam.min)=1]` and `repeated [(diam.max)=2]` fields;
- an enum `Color` with `COLOR_UNSPECIFIED = 0`, `COLOR_RED = 1`;
- an `octet_string` field declared as `string`;
- a command `TestRequest` (code 1, app 99999, request, proxiable) with `< session_id >`, an `AllTypes` field and a `Cards` field;
- a command `TestAnswer` (code 1, app 99999, proxiable).

- [ ] **Step 1: Write the generator negative tests in `main_test.go`**
  - Build `pluginpb.CodeGeneratorRequest` values in-process from hand-written `descriptorpb.FileDescriptorProto` literals, and run them through the same function `main` uses. Factor `run(gen *protogen.Plugin) error` out of `main` for this.
  - `TestRejectsKindMismatch`: `uint32` field for a UTF8_STRING AVP. The error contains `does not fit AVP type UTF8_STRING`.
  - `TestMissingDictionaryEntry`: the error contains `no dictionary entry`.
  - `TestAmbiguousDictionaryEntry`: the same name in two imported dictionaries. The error contains `ambiguous; set (diam.avp)`.
  - `TestTooManyRequired`: 65 required fields. The error contains `more than 64 required AVPs`.
- [ ] **Step 2: Write the fixture round-trip tests in `internal/gentest/types_test.go`**
  - `TestAllTypesRoundTrip`: set every field to a non-zero value (address `2001:db8::1`, time `2026-11-08T10:00:00Z`, float64 `-1.5`). Marshal → `diam.Decode` → `Validate()==nil`. Fields are equal and re-encode is byte-identical.
  - `TestZeroRequiredEncoded`: every scalar at its zero value still emits each required AVP. Count the AVPs in the output with `diam.NextAVP`.
  - `TestCardinality`: `max=2` with 3 occurrences gives 5009. `min=1` with 0 occurrences gives 5005. An optional field with 2 occurrences gives 5009.
  - `TestUnmarshalResets` (Review Focus 2): unmarshal message A (with Unknown entries), then message B (none) into the same struct. The result equals a fresh decode of B, and `Validate` gives no stale 5009.
  - `TestVendorMismatchIsUnknown` (Review Focus 3): a field's code with vendor 0 instead of 99999 lands in `Unknown`.
  - `TestEmptyValuesPreserved` (Review Focus 4): an empty `Cards` group, empty string and empty bytes are present after decode, and the bytes are identical.
  - `TestFlags`:
    - `Marshal` forces R|P on the request and P only on the answer, whatever flags the user set.
    - User-set E and T bits survive.
    - V is set exactly when vendor ≠ 0, and the AVP P-bit is never set.
  - `TestEnumNames`: `ColorRed == 1`, there is no `ColorUnspecified` constant (compile-time: reference only `ColorRed`), and `Color(7).String() == "7"`.
- [ ] **Step 3: Run `make generate && go test ./internal/gentest/ ./cmd/protoc-gen-go-diam/` and confirm FAIL**
- [ ] **Step 4: Port and split the generator so the tests pass**
  - The spike behaviour is the baseline.
  - Keep `pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL`.
  - `Unmarshal` must zero the struct before decoding, as the spike does with `*m = T{Header: h}`.
- [ ] **Step 5: Run `make generate && go vet ./... && go test -race ./internal/gentest/ ./cmd/protoc-gen-go-diam/` and confirm PASS**
- [ ] **Step 6: Commit** `feat(gen): protoc-gen-go-diam with type and cardinality fixtures`

---

### Task 5: Lenient decode, nested error paths, fixed position, M override

**Files:**
- Modify: `cmd/protoc-gen-go-diam/emit_codec.go`, `emit_validate.go`, `model.go`
- Create: `proto/gentest/v1/nested.proto` (imports `ietf/rfc6733/base.proto` once Task 7 lands; until then it imports `types.proto` and nests across files)
- Test: `internal/gentest/nested_test.go`

**Interfaces:**
- Consumes: Task 3 `InGroup`, `Misplaced`; Task 4 generated API.
- Produces these behaviour changes, which are binding for every package:
  - `DecodeAVPs` returns an error **only** when `NextAVP` fails at its own level.
  - A value decode error (any `diam.<Type>(a)` error) is recorded as the message's first decode error. The field is skipped and decoding continues.
  - A nested group whose `DecodeAVPs` returns an error is recorded as a 5014 error. Its `AVP` is that grouped AVP (cloned) and its `Path` is the enclosing codes. Decoding continues.
  - `Validate` checks in this order:
    1. the first recorded decode error;
    2. excess occurrences (5009);
    3. missing required AVPs (5005, decoded messages only);
    4. repeated-field bounds;
    5. fixed position (5004 via `Misplaced`);
    6. nested `Validate`, wrapped with `diam.InGroup(err, <field AVP code>)`;
    7. `CheckUnknown`.
  - Fixed position: the k-th `(diam.fixed)` field in field order must be the k-th AVP of the decoded sequence.
  - `(diam.m)` set on a field replaces the dictionary M-bit for that field's encoding.

- [ ] **Step 1: Write failing tests in `nested_test.go`**
  - `TestBadValueInsideGroupDoesNotAbort`: an `AllTypes` group whose Unsigned32 AVP has 3 data bytes, inside `TestRequest`. `Unmarshal` returns nil, the fields after it are decoded, and `Validate` returns 5014 with `AVP.Code` = the Unsigned32 AVP and `Path` = [the group's AVP code].
  - `TestGroupedOverrun` (Review Focus 1): a grouped AVP whose inner AVP length overruns the group. `Unmarshal` returns nil and `Validate` returns 5014 with `AVP` = that grouped AVP.
  - `TestNestedPathTwoLevels`: in the cross-file nested fixture, a 5009 two groups deep reports `Path` with both codes, outermost first.
  - `TestSessionIdNotFirst`: Session-Id as the third AVP gives 5004 with `AVP.Code == 263`-equivalent (the fixture's session AVP code).
  - `TestMOverride`: a field with `(diam.m) = false` over a dictionary entry with `m: true` encodes the M-bit cleared. Another field using the same AVP without an override keeps it set.
- [ ] **Step 2: Run `make generate && go test ./internal/gentest/` and confirm FAIL**
- [ ] **Step 3: Implement in the generator.** Store bookkeeping as unexported fields `seen uint64`, `excess []diam.RawAVP`, `decodeErr error`, `fixedPos []int`, `decoded bool`. The names are the builder's choice; the behaviour above is binding.
- [ ] **Step 4: Run `make generate && go test -race ./internal/gentest/` and confirm PASS. Run Task 4's tests again and confirm they still PASS.**
- [ ] **Step 5: Commit** `feat(gen): lenient decode, nested error paths, fixed position, M override`

---

### Task 6: Answer constructors

**Files:**
- Modify: `cmd/protoc-gen-go-diam/emit_command.go`
- Test: `internal/gentest/answer_test.go`

**Interfaces:**
- Produces: for every pair of command messages in one file with equal `code` and `app_id`, one `request: true` and one not, generate
  `func New<AnswerName>For(req *<RequestName>) *<AnswerName>`.
  - It sets `Header = diam.AnswerHeader(&req.Header)`.
  - When both messages have a `session_id` field, it copies `SessionId`.
  - It sets nothing else.

- [ ] **Step 1: Write the failing test** `TestNewAnswerFor`: request with HopByHop 7, EndToEnd 9, flags R|P|T and SessionId "s". The answer has flags P (R and T cleared), the same IDs, code and app, and SessionId "s". Origin-Host stays empty.
- [ ] **Step 2: Run and confirm FAIL** (undefined function)
- [ ] **Step 3: Implement the pairing and emission**
- [ ] **Step 4: Run `make generate && go test ./internal/gentest/` and confirm PASS**
- [ ] **Step 5: Commit** `feat(gen): New<Answer>For constructors`

---

### Task 7: RFC 6733 base protocol package

**Files:**
- Create: `proto/ietf/rfc6733/base.proto` (package `godiam.base`, go_package `github.com/hsdfat/go-diam/base`; extend the spike's `proto/spike/base.proto`)
- Generated: `base/base.diam.go`
- Test: `base/base_test.go`

**Interfaces:**
- Produces: package `base` with:
  - commands `CapabilitiesExchangeRequest/Answer` (257), `DeviceWatchdogRequest/Answer` (280), `DisconnectPeerRequest/Answer` (282), `ReAuthRequest/Answer` (258), `SessionTerminationRequest/Answer` (275), `AbortSessionRequest/Answer` (274), all app 0;
  - grouped `ProxyInfo`, `VendorSpecificApplicationId`, `ExperimentalResult`, `FailedAvp`, `E2ESequence`;
  - enums `AuthSessionState`, `DisconnectCause`, `ReAuthRequestType`, `TerminationCause`, `InbandSecurityId`, `RedirectHostUsage`, `SessionServerFailover`, `AuthRequestType`;
  - the base dictionary enum `Avp`, imported by every later application.

File-header rules (spec §5.5):
- Cite `RFC 6733 (October 2012)` and the sections transcribed: §5.3.1–5.3.2, §5.4.1–5.4.2, §5.5.1–5.5.2, §8.3.1–8.3.2, §8.4.1–8.4.2, §8.5.1–8.5.2, and the AVP tables in §4.5 and §8.21.
- Carry the RFC ABNF verbatim as a leading comment on every command and grouped message.
- Include every AVP that appears in those grammars.

Values recalled from memory. **Verify each against the RFC; the RFC wins:**
- Disconnect-Cause: REBOOTING 0, BUSY 1, DO_NOT_WANT_TO_TALK_TO_YOU 2.
- Re-Auth-Request-Type: AUTHORIZE_ONLY 0, AUTHORIZE_AUTHENTICATE 1.
- Termination-Cause: LOGOUT 1, SERVICE_NOT_PROVIDED 2, BAD_ANSWER 3, ADMINISTRATIVE 4, LINK_BROKEN 5, AUTH_EXPIRED 6, USER_MOVED 7, SESSION_TIMEOUT 8 (needs `_UNSPECIFIED = 0`).
- Inband-Security-Id: NO_INBAND_SECURITY 0, TLS 1.
- Session-Server-Failover: REFUSE_SERVICE 0, TRY_AGAIN 1, ALLOW_SERVICE 2, TRY_AGAIN_ALLOW_SERVICE 3.
- Redirect-Host-Usage: DONT_CACHE 0 … ALL_USER 6.
- Auth-Request-Type: AUTHENTICATE_ONLY 1, AUTHORIZE_ONLY 2, AUTHORIZE_AUTHENTICATE 3.
- Product-Name, Firmware-Revision and Error-Message have the M-bit **clear**.

- [ ] **Step 1: Write failing tests in `base/base_test.go`**
  - `TestBaseRoundTrip`: a table over all 12 commands with realistic values. Marshal → `diam.Decode` returns the right type, `Validate()==nil`, and re-encode is byte-identical.
  - `TestCERHostIPAddressMin`: CER without Host-IP-Address gives 5005.
  - `TestFlagsBase`: CER, DWR and DPR are encoded **without** the P-bit; ASR, STR and RAR **with** it (RFC 6733 command headers).
  - `TestNewCEAFor`: `NewCapabilitiesExchangeAnswerFor(cer)` copies the IDs. CER has no Session-Id, so none is copied.
  - `TestAnswerWithFailedAvp`: STA with a `FailedAvp{Unknown: []diam.RawAVP{x}}` round-trips, and `x` is intact.
- [ ] **Step 2: Run `make generate && go test ./base/` and confirm FAIL**
- [ ] **Step 3: Write `base.proto`.** Repoint `proto/gentest/v1/nested.proto` to import it (Task 5's note). Delete `proto/spike/` and `spike/` from `main` if Task 1 copied them.
- [ ] **Step 4: Run `make generate && buf lint && go test -race ./base/ ./internal/gentest/` and confirm PASS**
- [ ] **Step 5: Commit** `feat(base): RFC 6733 base protocol`

---

### Task 8: Fuzzing

**Files:**
- Create: `internal/diamtest/diamtest.go`, `diam/fuzz_test.go`, `base/fuzz_test.go`

**Interfaces:**
- Produces:
  - `diamtest.FuzzMessage(f *testing.F, newFn func() diam.Message, seeds ...[]byte)`. Property: `Unmarshal` never panics. If it returns nil, then `b1 := Marshal`, `Unmarshal(b1)`, `b2 := Marshal` gives `bytes.Equal(b1, b2)`.
  - `diamtest.SameAVPs(a, b []byte) error`, a multiset comparison of top-level AVPs (code, vendor, flags without V, data), recursing into AVPs that both sides decode as grouped. It is used by Task 10.

- [ ] **Step 1: Write the fuzz targets**
  - `FuzzNextAVP` in `diam`: loop `NextAVP` until error or empty. Never panic, and the total consumed is ≤ the input length.
  - In `base`: `FuzzCER`, `FuzzCEA`, `FuzzSTR`, `FuzzASA`, seeded with the outputs of `TestBaseRoundTrip`.
- [ ] **Step 2: Run each target for 30 s, e.g. `go test ./base/ -run=^$ -fuzz=FuzzCER -fuzztime=30s`. Expected: no failures.** Any crasher found gets fixed in the generator or runtime, and its corpus file is committed under `testdata/fuzz/`.
- [ ] **Step 3: Commit** `test: fuzz targets for runtime and base`

---

### Task 9: Interop, dictionary cross-check and benchmarks

**Deviation from spec §10:** dictcheck lives in `internal/interop/dictcheck_test.go` instead of `tools/dictcheck/`. That module already depends on go-diameter, and a test runs in CI with no extra binary. Record this deviation in the spec when the task lands.

**Files:**
- Create: `internal/interop/{interop_test,dictcheck_test,bench_test}.go`, `internal/interop/testdata/dictcheck-allow.txt`

**Interfaces:**
- Consumes:
  - go-diameter `dict.Default`, `(*dict.Parser).FindAVPWithVendor(appid uint32, code any, vendor uint32) (*dict.AVP, error)` (`dict.AVP` has `Code`, `VendorID`, `Must`, `MustNot`, `Data.Type`);
  - `diampb` extension descriptors;
  - `diamtest.SameAVPs`.

- [ ] **Step 1: Write `interop_test.go`**
  - `TestFiorixToGoDiam/<cmd>` for all 12 base commands: build with go-diameter (`diam.NewRequest`/`NewMessage`, `NewAVP`). go-diam decodes with `Validate()==nil` and re-encodes byte-identically.
  - `TestGoDiamToFiorix/<cmd>`: go-diam encodes. `fd.ReadMessage(..., dict.Default)` succeeds, the key AVPs match via `FindAVP`, and go-diameter's re-`Serialize` is byte-identical.
  - Port the spike's `spike/interop` cases as the starting point.
- [ ] **Step 2: Write `dictcheck_test.go`**
  - `TestDictionaryAgreesWithGoDiameter`:
    1. Run `buf build ../.. -o <tmp>/image.binpb`. Fail with a clear message if `buf` is not on PATH.
    2. Load it as a `FileDescriptorSet`, walk every `(diam.dictionary)` enum value and read `(diam.def)`.
    3. Look up `(appid 0, code, vendor)` in `dict.Default`, also trying its other applications.
  - Skip entries go-diameter lacks, and log their count.
  - For entries it has: compare vendor; M-bit (`Must` contains "M" ⇔ `m: true`); and type, after mapping go-diameter type names (`UTF8String`→UTF8_STRING, `DiameterIdentity`→DIAMETER_IDENTITY, `Enumerated`→ENUMERATED, `Grouped`→GROUPED, `OctetString`→OCTET_STRING, `Address`→ADDRESS, `Time`→TIME, `Unsigned32/64`, `Integer32/64`, `Float32/64`, `DiameterURI`→DIAMETER_URI).
  - Each mismatch fails unless `dictcheck-allow.txt` has a line `<code> <vendor> <field> <spec clause>`. A stale allow-list line (no longer mismatching) also fails.
- [ ] **Step 3: Write `bench_test.go`** with `BenchmarkCER` and `BenchmarkSTR` sub-benchmarks `go-diam/{marshal,unmarshal}` and `go-diameter/{marshal,unmarshal}`, shaped as in the spike's `BenchmarkULR`.
- [ ] **Step 4: Run `cd internal/interop && go test -race . && go test -run=^$ -bench=. -benchtime=200000x .`**
  - Expected: tests PASS. Bench output is recorded for the README in Task 11.
  - Any dictcheck mismatch is investigated against the RFC. Fix the proto, or add an allow line citing the clause.
- [ ] **Step 5: Commit** `test(interop): go-diameter interop, dictcheck, benchmarks`

---

### Task 10: Real-peer captures (freeDiameter) and offline replay

**Files:**
- Create: `ci/freediameter/Dockerfile`, `ci/freediameter/freeDiameter.conf`, `internal/interop/peer_test.go` (build tag `peer`), `internal/interop/captures_test.go`, `testdata/captures/base/SOURCE`

**Interfaces:**
- Consumes: `base` package, `diam.ReadFrame`, `diamtest.SameAVPs`.
- Produces: capture files `testdata/captures/base/<name>.bin`, one raw Diameter message each, plus `SOURCE`. `SOURCE` records the peer software and version, the date, the command line, and the statement "generated in CI against freeDiameter; no operator traffic".

- [ ] **Step 1: Write `ci/freediameter/Dockerfile` and its config**
  - Dockerfile: `debian:bookworm-slim`, then `apt-get install -y freediameter`. Generate a self-signed cert at build time, because freeDiameter requires `TLS_Cred` even for plain TCP.
  - `freeDiameter.conf`: Identity `peer.test`, Realm `test`, `No_SCTP`, `ListenOn "0.0.0.0"`, and accept any peer by `RealmName` ACL. Load no extensions beyond the default dictionary.
- [ ] **Step 2: Write `peer_test.go` with tag `peer`**
  - It reads `DIAM_PEER_ADDR` and dials TCP.
  - It sends a go-diam CER, reads the CEA with `ReadFrame`, decodes it as `base.CapabilitiesExchangeAnswer`, and requires `Validate()==nil` and Result-Code 2001.
  - Then the same for DWR→DWA and DPR→DPA.
  - With `-update`, it writes each received frame to `testdata/captures/base/{cea,dwa,dpa}.bin`.
- [ ] **Step 3: Write `captures_test.go` (always runs)**
  - For every `testdata/captures/*/*.bin`: `diam.Decode` succeeds, `Validate()==nil`, and re-encode satisfies `diamtest.SameAVPs(orig, reencoded) == nil`. This is semantic equality (spec §8.4).
  - Each directory must have a `SOURCE` file.
- [ ] **Step 4: Wire it into `.github/workflows/peer.yml`** (Task 11) so the frames are produced as a CI artifact. The maintainer downloads and commits them. Until frames exist, `captures_test.go` passes vacuously and logs "no captures".
- [ ] **Step 5: Run `cd internal/interop && go test .` locally and confirm PASS.** The peer test does not run locally (no containers on the dev host).
- [ ] **Step 6: Commit** `test(interop): freeDiameter peer harness and capture replay`

---

### Task 11: CI, README, contributor docs

**Files:**
- Create: `.github/workflows/ci.yml`, `.github/workflows/peer.yml`, `CONTRIBUTING.md`
- Modify: `README.md` (coverage, benchmarks, package paths), `docs/superpowers/specs/2026-11-08-go-diam-design.md` (record Task 9's deviation)

**Interfaces:** none new.

- [ ] **Step 1: Write `ci.yml`.** Triggers: push and pull_request. Jobs:
  - `lint`: `bufbuild/buf-action` pinned to buf 1.47.2. Runs `buf lint`; `buf breaking --against '.git#tag=<latest v*>'` is skipped while no tag exists.
  - `generated`: `make check-generated`.
  - `test`: matrix Go `1.23.x`, `oldstable`, `stable`, running `go vet` and `go test -race` on the three modules. The root module is tested on 1.23. `internal/interop` and `cmd` use their own `go` lines.
  - `root-deps`: assert `go list -m all` in the root prints one line.
  - `fuzz`: each fuzz target for 20 s.
- [ ] **Step 2: Write `peer.yml`.** Triggers: `workflow_dispatch` and weekly schedule.
  1. Build `ci/freediameter`.
  2. Run it with port 3868 published.
  3. Run `go test -tags peer -run TestPeer -update ./internal/interop/`.
  4. Upload `testdata/captures/base/` as an artifact.
- [ ] **Step 3: Update `README.md`**
  - Package paths `github.com/hsdfat/go-diam/{diam,base}`.
  - Coverage row "Base protocol" set to the status the evidence supports: `verified` once capture frames are committed, otherwise `spec-only` with go-diameter interop noted.
  - Replace the benchmark table with Task 9's numbers (CER, STR) and keep the spike's ULR row labelled "prototype".
  - Update the benchmark location line to `internal/interop`.
  - Remove the sentence saying `NewUpdateLocationAnswerFor` is design-only, if present.
- [ ] **Step 4: Write `CONTRIBUTING.md`** covering:
  - clean-room rules (§3, §9a);
  - how to add an application (proto header requirements, ABNF comments, `make generate`, gates 1–6, coverage status);
  - the commit-message style (plain, no AI footers);
  - "no Wireshark dictionaries".
- [ ] **Step 5: Run the full local check:** `make lint && make check-generated && make test` under the MemoryMax scope. All three exit 0.
- [ ] **Step 6: Commit** `ci: workflows; docs: README coverage and CONTRIBUTING`

---

## Done criteria for wave 0

- `make lint`, `make check-generated` and `make test` pass locally.
- The root module has zero dependencies.
- All 12 base commands pass interop in both directions with go-diameter v4.6.0. dictcheck is clean or every mismatch has a cited allow-list line.
- The fuzz targets run clean for 30 s each.
- The README matches the evidence.
- Nothing has been pushed. The owner reviews `main` and decides on the first push and the `v0.1.0` tag.
