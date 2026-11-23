# Chatto Server Personas

**Last reviewed:** 2026-11-23

Each persona is a typical type of self-hosted Chatto server. A real server
usually combines one **deployment** persona with one **identity** persona and
one **community** persona. For example, a company can run the high-availability
persona with the SSO-only persona. When a requirement depends on such a
combination, examine the combination.

Requirements state what the persona needs. They do not state how Chatto
satisfies the need, or if it does.

| ID | Persona | Type |
| --- | --- | --- |
| D1 | Single host | Deployment |
| D2 | Docker Compose stack | Deployment |
| D3 | Chatto Cloud | Deployment |
| D4 | High availability | Deployment |
| D5 | Private or offline network | Deployment |
| I1 | Company SSO only | Identity |
| I2 | AT Protocol only | Identity |
| C1 | Open public community | Community |
| C2 | Private invite-only group | Community |

## Deployment Personas

### D1: Single Host

**Goal:** Run Chatto with the fewest moving parts: a trial on a laptop, a
small VPS, a homelab or NAS appliance such as Unraid, TrueNAS, or
Synology, or a platform such as Coolify or Railway.

**Setup:** The standalone binary with systemd, or one container managed
through a web UI. Embedded NATS. Chatto's automatic TLS, a simple reverse
proxy, a platform proxy, or `http://localhost` for a trial. Often no email
provider. Short downtime during upgrades is acceptable.

**Requirements:**

- Starts with no external service and no account at a third party.
- Gets to a working owner account and a first room without email delivery.
- The documentation states which features need email, and what replaces them.
- Complete host preparation is documented: system user, directories, file
  permissions, service unit, and log location.
- One container is sufficient. All configuration is possible with
  environment variables in a UI form, with arbitrary user and group IDs.
- The documentation lists the required secrets and how to generate them.
- First-run setup and recovery work without an interactive shell, or the
  documentation names the alternative.
- The image runs on `amd64` and `arm64`.
- Durable writes survive a power loss of the host.
- An upgrade is a documented sequence: back up, replace, restart, verify.
- Backups can run on a schedule and go to a different host or a NAS share.
- The operator can move to D2 or D4 later without data loss.

### D2: Docker Compose Stack

**Goal:** Run the recommended production stack on one server: Chatto, NATS,
LiveKit, and a reverse proxy.

**Setup:** `examples/dockercompose`, `init-env.sh`, `.env`, Caddy for TLS.

**Requirements:**

- One script generates all secrets with matching values for each service.
- Image versions are pinned. An upgrade happens only when the operator
  decides to upgrade.
- The upgrade procedure is documented in the Compose guide, including
  releases that do not permit a rolling upgrade.
- Backup, restore, and operator commands work through `docker compose`.
- The operator can replace Caddy with an existing proxy, such as nginx or
  Traefik, from a documented example.
- The operator can remove optional services, such as LiveKit, without errors.

### D3: Chatto Cloud

**Goal:** ChattoCorp hosts and operates one isolated Chatto server for each
customer. Customers administer their community, but they never touch the
infrastructure.

**Setup:** A control plane provisions each server on Kubernetes, with no
manual step. Configuration through environment variables and secrets.
`owners.emails` contains the customer's email. ChattoCorp schedules upgrades
and backups.

**Requirements:**

- The control plane can provision a complete, working server with no
  interactive step and no browser.
- Customer administrators can do all community administration in the admin
  UI. No customer task needs the operator CLI, host access, or a
  configuration change.
- A hosted server is structurally equal to a self-hosted server. A customer
  can move their server out to their own infrastructure, and a self-hoster
  can move in, with no data loss and no product change.
- Upgrades across many servers are safe to automate, and the release notes
  state when a server needs special handling.
- Health, metrics, and alerts let one team monitor many servers.
- The operator can recover a customer's owner access without the customer's
  password.

### D4: High Availability

**Goal:** Keep the server available during upgrades, node failures, and
maintenance.

**Setup:** Several Chatto replicas, often on Kubernetes, a three-node or
five-node NATS cluster, a load balancer, and a monitoring system. Often managed services: hosted S3,
hosted NATS, LiveKit Cloud, and a hosted email provider.

**Requirements:**

- Rolling upgrades are possible, or the release notes clearly say when they
  are not.
- A failure of one Chatto replica or one NATS node does not stop the service.
- The move from embedded NATS to a cluster is a documented step-by-step
  procedure.
- The documentation states rollback and downgrade limits for each upgrade.
- Metrics and alert rules cover each component that can fail.
- Maintained Kubernetes manifests or a chart exist and agree with the
  documentation. Image versions are pinned.
- Scheduled backups can run as a job to object storage.
- Each managed service has documented connection settings, required
  permissions, limits, and the data that it receives.
- The operator can back up the data in each managed service.

### D5: Private Or Offline Network

**Goal:** Run Chatto only inside a LAN, a private overlay network such as
Tailscale, or an air-gapped network.

**Setup:** A private host name or IP address. Certificates from a private CA
or the overlay network. No Let's Encrypt. Limited or no outbound Internet
access. Internal IdP, email, and STUN or TURN.

**Requirements:**

- HTTPS works with a private certificate. Browser features that need a
  secure context still work, such as installation, push, microphone, and
  camera.
- Calls work with no public IP address and no public STUN server.
- One documented list names every outbound connection, the setting that
  controls it, and the effect when it is blocked.
- The server starts and works with all outbound connections blocked.
- Features that need the Internet fail clearly and can be disabled, such as
  link previews, AT Protocol login, and browser push services.

## Identity Personas

### I1: Company SSO Only

**Goal:** All access goes through the company's identity provider. No person
has a Chatto password.

**Setup:** One OIDC provider, such as Entra ID, Okta, Keycloak, Authentik, or
Pocket ID. `direct_login` and `direct_registration` are off. Possibly guest
access for contractors through an invitation or a second provider.

**Requirements:**

- No person ever has a Chatto password, including the first owner.
- The first owner can be declared in configuration by a provider identity or
  a verified email from the provider.
- Group membership in the IdP controls access and roles. Several providers
  can grant roles without conflicts.
- When a person leaves the company, their access ends in a defined time,
  without an administrator action in Chatto.
- Usernames come from the IdP and stay consistent.
- Guests get different default roles and room access than employees.
- An operator recovery path exists when the IdP is unavailable or its issuer
  changes.

### I2: AT Protocol Only

**Goal:** A community where each person signs in with their internet handle.
No passwords and no email.

**Setup:** The `atproto` provider only. `direct_login` and
`direct_registration` are off. No email provider.

**Requirements:**

- No person ever has a Chatto password, including the first owner.
- The first owner can be declared in configuration by handle or DID.
- The operator can limit sign-up to selected handle domains, DIDs, or
  AT Protocol servers, or allow everyone.
- A member keeps their account when their handle changes.
- A member who loses their handle or their AT Protocol server can recover
  their account through an administrator or operator.
- The UI does not offer actions that cannot work without email.
- The documentation states which external systems sign-in and profile
  lookups contact.

## Community Personas

### C1: Open Public Community

**Goal:** Anyone on the Internet can join, such as an open-source project or
a fan community. Often the community moves from a different platform.

**Setup:** Open registration, external providers, and possibly public badges
and an import of the old platform's history.

**Requirements:**

- The operator can limit abuse: sign-up rate limits, request rate limits,
  and bot or spam protection.
- The documentation gives a reverse-proxy rate-limit example.
- Moderators have tools for fast removal of spam and abusive accounts.
- A documented path exists from common platforms, such as Slack and Discord,
  to the import format. A trial import can be discarded.
- Resource use stays predictable as the member count increases.

### C2: Private Invite-Only Group

**Goal:** A closed group of family, friends, or a small team.

**Setup:** Invite-only account creation. Often combined with D1 or D5.

**Requirements:**

- Only invited people can create accounts, with every login method.
- Members can invite others when the owner permits it.
- The server reveals no member data to the public.

## Out Of Scope

Chatto does not optimize for these servers. Do not report their needs as
gaps. Report a gap only when it also affects a persona above.

- **Third-party hosting services.** Chatto Cloud is the only hosting service
  that Chatto optimizes for. Generic tooling for others who host many Chatto
  servers is not a goal: provisioning APIs for other control planes, shared
  infrastructure between servers, and tenant isolation guides.
