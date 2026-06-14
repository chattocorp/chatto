# Chatto Docker Compose Example

This example deploys a clustered Chatto setup with:

- **NATS** - Message broker with JetStream persistence
- **LiveKit** - WebRTC media server for voice calls
- **Chatto** - 3 replicas connecting to external NATS
- **Caddy** - Reverse proxy with automatic HTTPS and load balancing

## Prerequisites

- Docker and Docker Compose (v2) installed
- A domain pointing to your server (for automatic HTTPS)
- A `livekit.` subdomain pointing to the same server (e.g., `livekit.chat.example.com`)

## Configuration

1. Copy the example environment file:

   ```bash
   cp .env.example .env
   ```

2. Edit `.env` and fill in your values:

   ```bash
   # Generate secrets with:
   openssl rand -hex 32
   ```

   Key settings:
   - `PUBLIC_URL` - Your domain (e.g., `chat.example.com`)
   - `NATS_TOKEN` and `CHATTO_NATS_CLIENT_TOKEN` - Must match (shared auth token)
   - `CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET` - Session cookie signing
   - `CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET` - Session cookie encryption
   - `CHATTO_CORE_SECRET_KEY` - Bearer-token and account-flow verifier key
   - `CHATTO_CORE_ASSETS_SIGNING_SECRET` - Asset URL signing
   - `CHATTO_LIVEKIT_API_KEY` / `CHATTO_LIVEKIT_API_SECRET` - Must match the keys in `livekit.yaml`

3. Edit `livekit.yaml` and update:
   - The API key/secret pair under `keys:` (must match the `.env` values)
   - The webhook URL to match your `PUBLIC_URL`

4. Uncomment and configure SMTP settings if you need email verification.

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

If you don't need voice calls, remove the `livekit` service from `compose.yml`, delete the `livekit.yaml` file, and remove the LiveKit environment variables from `.env`.

## Troubleshooting

**Chatto can't connect to NATS**: Ensure `NATS_TOKEN` and `CHATTO_NATS_CLIENT_TOKEN` match in your `.env` file.

**Caddy not getting certificates**: Ensure your domain's DNS points to your server and ports 80/443 are open.

**Container startup order issues**: The `depends_on` with `condition: service_healthy` ensures NATS and LiveKit are ready before Chatto starts.

**Voice calls not working**: Ensure the LiveKit API key/secret in `.env` matches the `keys:` section in `livekit.yaml`. Also verify the webhook URL in `livekit.yaml` points to your Chatto instance. Make sure `CHATTO_LIVEKIT_URL` uses the public `wss://livekit.` subdomain (not the internal Docker hostname), since browsers connect to it directly.

**LiveKit UDP ports**: WebRTC requires UDP ports 50000-50200. Ensure your firewall allows inbound UDP on this range.
