# FDR-029: Transactional Email Delivery

**Status:** Active
**Last reviewed:** 2026-07-12

## Overview

Chatto sends transactional email for account registration, email-address verification, and password resets. Server operators choose either SMTP or JMAP submission; SMTP remains the default for existing and new deployments.

## Behavior

- Local-account registration, email-address verification, and password-reset flows use the selected transactional email transport without changing their user-facing workflow.
- Existing SMTP configuration continues to work unchanged. Operators select JMAP explicitly and configure an HTTPS JMAP session URL, bearer token, and sender address.
- JMAP uses an available sending identity matching the configured sender and a Drafts mailbox. Operators can explicitly select the account, identity, or Drafts mailbox when automatic selection is unsuitable. Chatto requests removal of the temporary draft after submission and records a safe operator warning if that cleanup fails.
- A successful JMAP request means the JMAP server accepted the submission. Chatto does not claim final delivery to every recipient.

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

## Related

- **ADRs:** None
- **FDRs:** FDR-018 (Account Lifecycle), FDR-023 (Authentication & Sessions)
