# Security Policy

warmbly-go is a client SDK. The most security-relevant code in this repository
is the authentication handling (API keys and OAuth 2.1 tokens), the webhook
signature verification, and the gateway connection layer. We take reports about
any of these — or anything else — seriously.

## Supported versions

While the SDK is pre-1.0, security fixes are released against the latest minor
version. Please make sure you can reproduce an issue on the most recent release
before reporting.

| Version | Supported          |
| ------- | ------------------ |
| 0.x     | :white_check_mark: |

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately through either of:

- GitHub private security advisories:
  <https://github.com/warmbly/warmbly-go/security/advisories/new>
- Email: **team@warmbly.com**

Please include:

- A description of the issue and its impact.
- Steps to reproduce, or a minimal proof of concept.
- The version of warmbly-go and Go you tested with.
- Any suggested remediation, if you have one.

## What to expect

- We will acknowledge your report within **3 business days**.
- We will keep you informed as we investigate and work on a fix.
- Once a fix is available, we will publish a release and a security advisory,
  and — with your permission — credit you for the discovery.

We ask that you give us a reasonable opportunity to address the issue before any
public disclosure. We are happy to coordinate disclosure timing with you.

## Scope

This policy covers the warmbly-go SDK itself. Vulnerabilities in the Warmbly
platform or API (as opposed to this client library) should also be sent to
**team@warmbly.com** and will be routed to the appropriate team.
