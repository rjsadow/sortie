# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

Only the latest release receives security patches. We recommend
always running the most recent version.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please report vulnerabilities through one of these channels:

1. **GitHub Security Advisories (preferred):**
   Go to [Security Advisories](https://github.com/rjsadow/sortie/security/advisories/new)
   and create a new private advisory.

2. **Email:**
   Send details to the maintainer at the email listed on the
   [GitHub profile](https://github.com/rjsadow).

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact
- Suggested fix (if any)

## Response Timeline

- **Acknowledgment:** Within 3 business days
- **Initial assessment:** Within 7 business days
- **Fix or mitigation:** Depends on severity, but we aim for:
  - Critical/High: patch release within 14 days
  - Medium/Low: included in the next regular release

## Scope

The following are considered security issues:

- Authentication or authorization bypass
- Injection vulnerabilities (SQL, command, XSS)
- Sensitive data exposure (credentials, tokens, PII)
- Container escape or privilege escalation in session pods
- Cryptographic weaknesses in JWT handling
- Denial of service via resource exhaustion

The following are **not** security issues (file as a regular bug):

- Crashes that require authenticated admin access to trigger
- Issues only exploitable with physical access to the host
- Vulnerabilities in dependencies that don't affect Sortie's usage

## Acknowledgments

We appreciate responsible disclosure and will credit reporters
in the release notes (unless you prefer to remain anonymous).
