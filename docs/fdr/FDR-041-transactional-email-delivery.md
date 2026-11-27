# FDR-041: Transactional Email Delivery

**Status:** Active
**Last reviewed:** 2026-11-27

## Overview

Chatto sends transactional email for account registration, email-address verification, and password resets. Server operators choose either SMTP or JMAP submission; SMTP remains the default for existing and new deployments.

## Behavior

- Each message has a plain-text part and an HTML alternative. Both come from one shared layout in `cli/internal/email/templates/`. The HTML uses tables and inline styles only. It loads no images, fonts, or other remote resources, so opening the message does not reveal the recipient's IP address.
- The queue stores what an email says (the code or reset token and its lifetime), not the rendered message. The worker renders the email when it sends it, with the current templates, server name, and web server URL. A server renamed while an email waits sends it with the new name.
- A one-time code is in the subject line (`123456 is your code for <server>`), so the recipient can read it in a notification. A password reset email has a button and the full link as text.
- The footer shows the host of the public web server URL, so the recipient can identify the server that sent the message.
- When the configured sender address has no display name, Chatto uses the server name as the display name. A configured display name always applies.
- Local-account registration, email-address verification, and password-reset requests accept mail into a durable queue. Success means queue acceptance, not transport or recipient delivery.
- Accepted mail survives process restart. Transport failures retry with increasing delays until the original code or reset-link expiry. Queueing does not extend that expiry or refund a delivery limit.
- The Jobs page shows mail attempts and safe failure categories without addresses, codes, reset links, or message content. Expired jobs cannot be retried.
- A crash after transport acceptance can cause a duplicate. Mail content is encrypted while it waits and is removed after transport acceptance or expiry.
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

**Decision:** JMAP support creates and submits a transactional message (plain text, with an HTML alternative when the message has one), then requests removal of the temporary draft after successful submission. A cleanup failure does not turn an accepted submission into a failed account flow. It does not synchronize a mailbox or track final delivery status.
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

### 5. HTML with a plain-text alternative and no remote resources

**Decision:** Every message has a plain-text part. The HTML part is a `multipart/alternative` addition. The HTML uses the default accent of the bundled frontend, which the layout receives as a parameter. Servers do not have their own accent color yet.
**Why:** Formatted email is easier to read and act on, but some recipients read only plain text, and some tests and tools read the plain-text part. Remote images or fonts would tell a third party, or the server, when and from which IP address a recipient opened the message.
**Tradeoff:** The layout shows the server name as text, not a logo. Email clients render the same HTML differently, so the layout uses only simple, widely supported markup.

### 6. Render when the email is sent

**Decision:** The queue stores the content of an email (recipient, code or reset token, lifetime) and renders it in the job worker.
**Why:** The stored record holds a credential. It should contain only that credential and the data needed to send it, not several kilobytes of rendered text and HTML. A small record also keeps wording and layout out of storage, so template changes and later translations apply to every email that is still waiting.
**Tradeoff:** The worker needs the server branding. The HTTP runtime supplies it when it creates the queue. A record from an older version, which holds a rendered message, is sent as plain text.

## Related

- **ADRs:** [ADR-123](../adr/ADR-123-durable-background-jobs.md)
- **FDRs:** FDR-018 (Account Lifecycle), FDR-023 (Authentication & Sessions)
- **Issues:** [#1440](https://github.com/chattocorp/chatto/issues/1440), [#1454](https://github.com/chattocorp/chatto/issues/1454)
