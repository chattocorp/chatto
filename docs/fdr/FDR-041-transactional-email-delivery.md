# FDR-041: Transactional Email Delivery

**Status:** Active
**Last reviewed:** 2026-09-27

## Overview

Chatto sends transactional email for account registration, email-address verification, and password resets. Server operators choose either SMTP or JMAP submission; SMTP remains the default for existing and new deployments.

## Behavior

- Local-account registration, email-address verification, and password-reset flows use the selected transactional email transport without changing their user-facing workflow.
- Existing SMTP configuration continues to work unchanged. Operators select JMAP explicitly and configure an HTTPS JMAP session URL, bearer token, and sender address.
- JMAP uses an available sending identity matching the configured sender and a Drafts mailbox. Operators can explicitly select the account, identity, or Drafts mailbox when automatic selection is unsuitable. Chatto requests removal of the temporary draft after submission and records a safe operator warning if that cleanup fails.
- A successful JMAP request means the JMAP server accepted the submission. Chatto does not claim final delivery to every recipient.
- SMTP requires STARTTLS with certificate verification by default. Port `465` uses implicit TLS (SMTPS) when `smtp.tls` is empty or `mandatory`. If the server does not offer STARTTLS, or if its certificate is not valid, Chatto does not send the message.
- Two SMTP settings decrease transport security: `smtp.tls = "opportunistic"` sends in plaintext when the server does not offer STARTTLS, and `smtp.tls_skip_verify = true` accepts any server certificate. When SMTP is enabled with one of these settings, the HTTP server logs a warning at startup. The warning names only the setting keys.
- Opportunistic mode does not disable certificate verification. When the server offers STARTTLS but its certificate is not valid, Chatto does not send the message.
- Email delivery errors do not contain email addresses, message IDs, or SMTP server replies. Server replies often repeat the rejected address. The error keeps the failed SMTP command, the reply code, and the enhanced status code.

## Design Decisions

### 1. SMTP remains the default transport

**Decision:** SMTP is the default; JMAP is an explicit alternative.
**Why:** SMTP is the broadly supported self-hosting integration and existing deployments already rely on it. JMAP support serves providers that expose bearer-token submission without replacing the Internet mail-delivery path.
**Tradeoff:** Chatto maintains two submission clients and operators must select JMAP deliberately.

### 2. JMAP is submission-only

**Decision:** JMAP support creates and submits a plain-text transactional message, then requests removal of the temporary draft after successful submission. A cleanup failure does not turn an accepted submission into a failed account flow. It does not synchronize a mailbox or track final delivery status.
**Why:** Chatto needs outbound account-flow messages, not a general-purpose mail client. Keeping the integration narrow avoids mailbox state and user-data concerns while meeting the transactional use case.
**Tradeoff:** Operators cannot use Chatto to inspect sent mail or final per-recipient delivery results. A provider-side cleanup failure can leave a temporary draft that the operator must investigate from the safe warning log.

### 3. Use a bearer token rather than a mailbox password

**Decision:** JMAP configuration uses a bearer access token.
**Why:** Token-based credentials can be scoped and revoked by the mail provider without storing a reusable mailbox password in Chatto configuration.
**Tradeoff:** Provider-specific token issuance and refresh lifecycle remain an operator responsibility; Chatto does not perform an OAuth authorization flow or refresh tokens.

### 4. Keep insecure SMTP modes and warn at startup

**Decision:** Chatto accepts opportunistic TLS and skipped certificate verification, but logs a startup warning when SMTP uses them. It does not reject them or require a development-only override.
**Why:** Some self-hosted deployments use internal relays that do not offer STARTTLS or that use self-signed certificates. Local development uses Mailpit without TLS. If Chatto rejected these modes, these deployments would stop at upgrade. The secure defaults protect new configurations, and the warning makes an insecure choice visible to the operator.
**Tradeoff:** An operator can still send password-reset links and verification codes over a connection that an attacker can read. Chatto makes this risk visible but does not prevent it.

## Related

- **ADRs:** None
- **FDRs:** FDR-018 (Account Lifecycle), FDR-023 (Authentication & Sessions)
- **Issues:** [#1440](https://github.com/chattocorp/chatto/issues/1440), [#1454](https://github.com/chattocorp/chatto/issues/1454)
