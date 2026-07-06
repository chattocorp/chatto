# Chatto Docker Compose Example

This example deploys a clustered Chatto setup with:

- **NATS** - Message broker with JetStream persistence
- **LiveKit** - WebRTC media server for voice calls
- **Chatto** - App server connecting to external NATS
- **Caddy** - Reverse proxy with automatic HTTPS and load balancing

## Prerequisites

- Docker and Docker Compose (v2) installed
- A domain pointing to your server (for automatic HTTPS)
- A `livekit.` subdomain pointing to the same server (e.g., `livekit.chat.example.com`)
- Firewall allowing inbound TCP 80/443 and UDP 3478, 50000-50200

## Configuration

1. Generate `.env` and the LiveKit config:

   ```bash
   ./init-env.sh chat.example.com admin@example.com
   ```

   Replace `chat.example.com` with your Chatto domain and `admin@example.com`
   with the email address you will use for the first account.

   The script is the recommended setup path. It writes `.env` and
   `livekit.generated.yaml`, generates strong secrets, and keeps the shared
   values aligned:

   - `NATS_TOKEN` and `CHATTO_NATS_CLIENT_TOKEN`
   - Chatto cookie, core, and asset signing secrets
   - `CHATTO_LIVEKIT_API_KEY` / `CHATTO_LIVEKIT_API_SECRET`
   - The matching LiveKit `keys:` and webhook URL

2. Edit `.env` and review the generated values.

   In most cases, you should only need to change:

   - `PUBLIC_URL` - Your domain (e.g., `chat.example.com`)
   - `CHATTO_OWNERS_EMAILS` - Comma-separated verified email addresses that should become Chatto owners. Include the email address you will use for the first account.
   - `CHATTO_SMTP_*` - Required for direct email/password registration, email verification, and password reset.
   - `PUID` and `PGID` - Optional host user/group IDs for files Chatto writes to mounted volumes. Defaults to `1000:1000`.
   - `CHATTO_OPERATOR_API_*` - Enables the private in-container operator socket used by `chatto operator ...`.

   Leave `LIVEKIT_CONFIG_FILE=./livekit.generated.yaml` unless you deliberately
   want to maintain `livekit.yaml` by hand.

3. Configure SMTP settings if you use direct email/password registration.

### Manual setup fallback

Use this only if you cannot run `init-env.sh` or need to maintain secrets
outside this folder:

```bash
cp .env.example .env
```

Then edit `.env`, replace every placeholder secret with output from
`openssl rand -hex 32`, and edit `livekit.yaml` so the API key, API secret, and
webhook URL match `.env`. LiveKit requires the API secret to be at least 32
characters.

## Usage

```bash
# Start the stack
docker compose up -d

# View logs
docker compose logs -f

# View logs for a specific service
docker compose logs -f chatto

# Restart a service
docker compose restart chatto

# Stop the stack
docker compose down

# Stop and remove volumes (deletes all data)
docker compose down -v
```

## Scaling

```bash
# Scale to 5 replicas
docker compose up -d --scale chatto=5
```

## Inspecting NATS

The Chatto image includes the `nats` CLI and writes a context for the runtime
NATS connection. Run it as the `chatto` user so the CLI reads the context from
`/home/chatto`:

```bash
docker compose exec -u chatto chatto nats stream ls
```

## Operator Commands

The generated `.env` enables the local operator API socket inside the Chatto
container. Run operator commands as the `chatto` user and use `list --search`
to find a stable user ID before mutating an account:

```bash
docker compose exec -u chatto chatto /chatto operator user list
docker compose exec -u chatto chatto /chatto operator user list --search admin@example.com
docker compose exec -u chatto chatto /chatto operator user set-password USER_ID
```

Do not mount or publish the operator socket unless the target container or host
is fully trusted; socket access is root-equivalent Chatto authority.

## Updating

```bash
# Pull new images and recreate containers
docker compose pull
docker compose up -d
```

## Volumes

Data is persisted in Docker volumes:

- `nats_data` - NATS/JetStream data (messages, KV stores)
- `caddy_data` - TLS certificates
- `caddy_config` - Caddy configuration cache

## Disabling Voice Calls

If you don't need voice calls, remove the `livekit` service from `compose.yml`, delete the selected LiveKit config (`livekit.generated.yaml` or `livekit.yaml`), and remove the LiveKit environment variables from `.env`.

## Troubleshooting

**Chatto can't connect to NATS**: Ensure `NATS_TOKEN` and `CHATTO_NATS_CLIENT_TOKEN` match in your `.env` file.

**Registration says email delivery is not configured**: Configure the `CHATTO_SMTP_*` settings in `.env`. Direct email/password registration sends a code by email.

**The first account is not an owner**: Ensure `CHATTO_OWNERS_EMAILS` contains that account's verified email address. Chatto assigns matching owner roles when the email is verified and on server boot.

**Caddy not getting certificates**: Ensure your domain's DNS points to your server and ports 80/443 are open.

**Container startup order issues**: The `depends_on` with `condition: service_healthy` ensures NATS and LiveKit are ready before Chatto starts.

**Voice calls not working**: Ensure the LiveKit API key/secret in `.env` matches the `keys:` section in the selected LiveKit config (`livekit.generated.yaml` or `livekit.yaml`). Also verify the webhook URL points to your Chatto instance. Make sure `CHATTO_LIVEKIT_URL` uses the public `wss://livekit.` subdomain (not the internal Docker hostname), since browsers connect to it directly.

**LiveKit UDP ports**: The example exposes UDP 50000-50200 for direct WebRTC media and UDP 3478 for LiveKit's embedded TURN/STUN relay. Ensure your firewall allows inbound UDP on both.

**Calls fail for some users**: The built-in TURN/UDP relay helps with symmetric NATs and some mobile, Firefox, and restrictive-network cases. Networks that block UDP entirely still need an advanced TURN/TLS setup, such as a dedicated TURN host or L4 TLS forwarding with matching certificates.
