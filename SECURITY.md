# Security Policy

## Reporting a Vulnerability

If you believe you have found a security vulnerability in Luna, please report it
privately. **Do not open a public GitHub issue.**

Email: `security@aplfintech.com`

Please include:

- A description of the issue and its impact
- Steps to reproduce, or a proof-of-concept if available
- The version, commit hash, or release tag affected
- Your name and contact details (we are happy to credit you in the fix)

We aim to acknowledge reports within **3 business days** and to provide a status
update within **10 business days**. Coordinated disclosure timelines are agreed
case-by-case once we have assessed the issue.

## Supported Versions

Luna is in early public release. Until a `v1.0.0` tag is published, only the
latest commit on `main` receives security fixes. Once stable releases begin, the
two most recent minor versions will be supported.

## Scope

In scope:

- The peer node (`cmd/lunapeer`)
- The transaction generator (`cmd/txgen`)
- The REST API service (`cmd/api`, `services/api`)
- Cryptographic primitives (`crypto/`)
- Consensus, networking, and ledger logic in this repository

Out of scope:

- Issues in third-party dependencies (please report upstream first; we will
  pick up the fix once it is released)
- The smart-contract VM image (`ghcr.io/vg-grape/luna-smc`) — report there
- Vulnerabilities that require physical access or compromised peer keys
- Denial-of-service attacks on public test networks
