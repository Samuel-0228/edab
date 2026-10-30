# Security Policy

## Supported Versions

Kerob actively maintains and supports the latest minor release tracks with security patches.

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |
| < 0.1.0 | :x:                |

## Reporting a Vulnerability

The Kerob maintainers take the security of our platform seriously. We appreciate responsible disclosure of potential vulnerabilities.

If you believe you have found a security vulnerability in Kerob:

1. **Do not** report security vulnerabilities through public GitHub issues or public forums.
2. Email the maintainer directly at **samuaelyeshambel@gmail.com** or open a [GitHub Private Security Advisory](https://github.com/Samuel-0228/edab/security/advisories/new).
3. Please include:
   - A detailed description of the vulnerability.
   - Exact steps or script to reproduce the issue.
   - Proof-of-concept payload if applicable.
   - The impact of the vulnerability.

### Response Timeline
- **Initial Response:** Within 48 hours acknowledging receipt of the report.
- **Triage & Assessment:** Within 5 business days confirming severity and affected versions.
- **Patch Release:** Security advisories and patches are prioritized and released in a patch release.

## Security Architecture Principles

Kerob enforces several built-in defensive layers:
- **SSRF Protection:** Outbound network requests through webhooks, Discord, GitHub, and HTTP actions are validated by proactive socket-level dialers blocking loopback, private IPv4/IPv6 ranges (RFC1918), link-local, cloud metadata services (`169.254.169.254`), and defeating DNS rebinding.
- **Encrypted Secrets at Rest:** External integration tokens and API keys are stored encrypted via AES-256-GCM.
- **Safe Evaluation Engines:** Condition evaluations use an AST without arbitrary code execution or runtime eval. Transformations use deterministic JSON path templating.
- **Container Hardening:** Docker images run as an unprivileged, non-root user (`kerob:kerob`, UID/GID 1000).
- **Constant-Time Verification:** Management API authentication tokens are verified using constant-time string comparisons.
