# Independent public protocol conformance

This is a test-only Python reference endpoint and client, not a second UCUPA
product or a production storage implementation. It imports neither Go owner
code, Go SDK/gateway code, nor Store data. Its message classes are constructed
by Python Protobuf from a descriptor set produced by the repository's pinned
`protoc 34.1` and public `.proto` sources. There are no copied message bindings.

`corpus.json` is one ordered public-operation corpus run without alteration
against the independent Python endpoint and real Go Core/Facet product hosts.
Each endpoint has its own state, tokens and receipts. Captures compare opaque
tokens only within the issuing endpoint; no token format is presumed. The
reference endpoint is independently written from the public contract rather
than delegating operations back to Go. It deliberately implements only the
declared local-space and attached-content corpus. It is not evidence of
independent federation, mount recovery, durable storage or deployment maturity.

The separate Python client verifies trusted certificate material against the
configured expected SPKI before loading it as an exact certificate trust root.
The network implementation therefore cannot send a credential to another
certificate merely because the fixture CA signed it. Certificates, credentials,
ports and Stores are temporary fixtures; tests never use external services.

The common TLS certificate algorithm in this corpus is ECDSA P-256. The first
run with the Go-only Ed25519 TLS fixture failed negotiation in grpcio's
BoringSSL (`NO_COMMON_SIGNATURE_ALGORITHMS`); the corpus does not pretend that
all certificate algorithms interoperate. This changes neither production
identity signing (Ed25519) nor the Go-only certificate tests. The reference
endpoint is a bounded application-semantics oracle, not an alternative secure
production host: full authorization/revocation, TLS downgrade/admission,
multisource privacy and crash/recovery matrices remain separate gates.

The 42 cases cover mandatory session/target handling, unknown/missing Protobuf
fields, public Core entry/read/attachment projection, create/update CAS and
original-operation recovery, range intersection when a center lies outside the
query sphere, unchanged-basis pagination/read, prepared observation, and Facet
typed content operations, exact opaque Unicode payloads,
idempotency, recovery and observation. The fixture
pre-provisions one attachment before closing the Stores and starting the Go
products. Thus it does not prove the network attachment-control lifecycle.
The reference retains only process-local state and only identity-rotation
geometry; unsupported rotation fails explicitly. These bounds are not
restrictions introduced into the actual Go product or its public contract.

## Tool setup

Use Python 3.12 and an isolated virtual environment. The suite checks the exact
versions in `requirements.txt`; missing tools or mismatches fail, never skip.
Tests neither install packages nor contact a package registry.

```powershell
& <python-3.12> -m venv "$HOME/.cache/ucupa-interop-python"
& "$HOME/.cache/ucupa-interop-python/Scripts/python.exe" -m pip install -r acceptance/testdata/interop/requirements.txt
$env:UCUPA_INTEROP_PYTHON = "$HOME/.cache/ucupa-interop-python/Scripts/python.exe"
go test ./acceptance -run '^TestNetworkIndependentImplementationCorpus$' -count=1
```

On Unix use `bin/python` in that virtual environment. The explicit
`UCUPA_INTEROP_PYTHON` setting takes precedence; otherwise the test checks only
this dedicated user-cache environment, not a Windows Store stub or arbitrary
global packages. `protoc` must already be available through the ordinary pinned
generation toolchain. All calls and child lifetimes have finite deadlines.
