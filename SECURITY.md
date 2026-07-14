# Security policy

## Supported versions

Only the latest released version of semrel is actively supported. Security fixes
are released as patch versions following Semantic Versioning.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities by sending a private security advisory via
[GitHub Security Advisories](https://github.com/nrkno/github-action-semantic-release/security/advisories/new)
or by emailing **security@nrk.no**.

Include as much detail as possible:

- A description of the vulnerability and its impact
- Steps to reproduce or a minimal proof-of-concept
- Affected version(s)
- Any suggested mitigation

We will acknowledge your report within **3 business days** and aim to release a
fix within **14 days** for critical issues.

## Scope

In scope:

- The `semrel` Go binary and its internal packages
- The container image published to GHCR
- The GitHub Actions workflow definitions in this repository
- Supply-chain issues (compromised dependencies, unsigned images)

Out of scope:

- Vulnerabilities in third-party dependencies that have no available fix
- Social-engineering attacks against NRK employees
- Issues that require physical access to systems

## Disclosure policy

We follow coordinated disclosure. Please allow us reasonable time to release a
fix before any public disclosure. We will credit reporters in the release notes
unless anonymity is requested.

## Supply-chain attestations

Container images are signed with [cosign keyless](https://docs.sigstore.dev/cosign/signing/overview/)
on every release. Verify an image signature:

```bash
cosign verify ghcr.io/nrkno/github-action-semantic-release:<tag> \
  --certificate-identity-regexp="https://github.com/nrkno/github-action-semantic-release/.*" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```
