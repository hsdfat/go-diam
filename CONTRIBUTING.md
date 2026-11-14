# Contributing to go-diam

Thanks for helping. This file covers the rules that are specific to this
project. They exist to keep two promises: the bytes on the wire are right,
and every line in the repository is free to publish.

## Clean room

- Every file is written fresh. Do not copy code, `.proto` files, tests or
  captures from a proprietary Diameter stack, including ones you wrote at
  work.
- AVP facts (codes, vendor ids, flags, types, grammars) come from the public
  specification: IETF RFCs and 3GPP technical specifications. Transcribe
  them in this repository's own structure.
- **No Wireshark dictionaries.** They are GPL. The freeDiameter (BSD) and
  go-diameter (BSD-3-Clause) dictionaries are used for cross-checking in
  tests only, and are credited in `NOTICE`.
- No captures from an operator or employer network. Captures come from
  open-source peers run in CI, from public samples whose license allows
  redistribution, or from hand-built vectors checked against the spec.

## Layout

| Path | What |
|---|---|
| `diam/` | runtime: framing, data types, errors, registry (standard library only) |
| `base/`, ... | generated packages, committed |
| `proto/` | the schema: one `.proto` file per application |
| `cmd/protoc-gen-go-diam/` | the generator (own module; depends on protobuf) |
| `internal/interop/` | interop, dictionary cross-check, capture replay, benchmarks (own module; depends on go-diameter) |
| `internal/gentest/` | generator fixtures and their tests |

The root module must not gain a dependency. `make root-deps` checks it.

## Working on the code

You need Go, [buf](https://buf.build/docs/installation) 1.47.2 and
`protoc-gen-go` v1.36.12 (`go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12`).

```sh
make generate         # rebuild the generator and every generated file
make lint             # buf lint and go vet
make test             # all three modules, with the race detector
make check-generated  # regenerate and fail if anything changed
```

Generated files are committed. A change to a `.proto` file or to the
generator goes in the same commit as the regenerated output.

Write the test first and watch it fail. Generator behaviour is tested
through the fixtures in `proto/gentest/v1` and `internal/gentest`; generator
errors are tested in `cmd/protoc-gen-go-diam/main_test.go`.

## Adding an application

1. **Schema.** Create `proto/<body>/<spec>/<name>.proto` (for example
   `proto/3gpp/ts29272/s6a.proto`). buf allows one proto package per
   directory, so a second application of the same specification goes in a
   subdirectory named after it (`proto/3gpp/ts29272/s13/s13.proto`). The
   file header names the specification, the version transcribed and the
   clauses covered. Every command and grouped message carries its ABNF from
   the specification as a leading comment.
   Write against the latest release; an AVP that an older supported release
   lacks or has as optional stays optional, with its history in a comment
   (3GPP: Rel-8 peers must pass `Validate`).
2. **Dictionary.** AVPs go in an enum marked `(diam.dictionary)`. Import the
   base dictionary instead of redefining base AVPs. A per-use M-bit goes on
   the field as `(diam.m)`, with the clause that says so in a comment.
   An entry is named after the AVP in upper case with `-` and spaces as
   `_`; a name that starts with a digit is spelt `TGPP...` (3GPP) or
   `FIVE_QI` (5QI). The enum value is the AVP code, so when an IETF and a
   vendor AVP share a code, put them in separate dictionary enums (see
   `IetfAvp` in `proto/3gpp/ts29272/s6a.proto`). AVPs re-used from another
   specification go in the application's own dictionary with the M-bit the
   application gives them (3GPP specs override it, e.g. TS 29.272 table
   7.3.1/2), and the defining clause in a comment. An application whose
   AVPs all sit, with the same flags, in another application's dictionary
   from the same specification imports that file and has no dictionary of
   its own (S13/S13' imports `s6a.proto`); one whose AVP table differs has
   its own dictionary even in the same specification (S6m/S6n next to S6t).
3. **Generate** with `make generate` and commit the output.
4. **Pass the gates.** An application is listed as *verified* only when all
   of these hold:
   1. transcription from the named specification version;
   2. dictionary cross-check (`TestDictionaryAgreesWithGoDiameter`): every
      disagreement is fixed or has a line in
      `internal/interop/testdata/dictcheck-allow.txt` citing the clause, and
      every entry go-diameter lacks is listed in
      `internal/interop/testdata/dictcheck-absent.txt`;
   3. interop with go-diameter in both directions for every command, built
      on go-diameter's side from its own constants and dictionary. If
      go-diameter has no dictionary for the application (Cx/Dx, Sh/Dh, S6t, S6m), use
      its definitions of the AVPs from the applications that have them, and
      list the AVPs it defines nowhere with their code and flags restated
      from the specification, as `cxRestated` in
      `internal/interop/cx_test.go` does;
   4. at least one capture per command pair from an independent peer,
      replayed by `TestCaptures`;
   5. a fuzz target per command, clean for 30 s locally and run in CI;
   6. a benchmark against go-diameter for at least one command.

   Without an independent peer or capture (gates 3 and 4) the application is
   *spec-only*. With commands missing it is *partial*. The coverage table in
   the README says which, and never more than the evidence supports.

## Commits and pull requests

- Plain conventional commits: `feat(gen): ...`, `fix(diam): ...`,
  `test(interop): ...`. Describe the change and why.
- No tool or AI attribution footers (`Co-Authored-By`, "Generated with",
  session links) in commit messages or pull request descriptions.
- One logical change per pull request, with its tests.
