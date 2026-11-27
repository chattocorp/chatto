# Deploy Chatto on Fly.io

Run one always-on Fly Machine with embedded NATS JetStream. Fly terminates HTTPS for your domain and forwards requests to Chatto on port 4000.

## Deploy

Install Bash, `flyctl`, `curl`, and `jq`. Sign in with `flyctl auth login`. Choose an unused Fly app name, a region, and a hostname in a DNS zone you control. Copy this example to a directory you can keep. Run the deployment script in that directory.

```sh
./deploy.sh --app acme-chat --region fra --domain chat.example.com
```

1. Select your Fly organization if prompted. The script creates a billable Machine with one shared CPU, 1 GB of RAM, and a 10 GB volume.
2. Open the local setup URL printed by the script and create the owner account. The tunnel listens on your computer. Chatto has no public HTTP service during setup. If port 14000 is in use, set `LOCAL_PORT` to a free port.
3. Add the DNS records that Fly prints. Once the certificate is ready, sign in at `https://chat.example.com`. If DNS or certificate issuance is pending, rerun the same command from the same directory.

Keep the generated `fly-acme-chat/` directory. It contains your app configuration and installation progress. The script uses it to resume an interrupted setup.

## Storage and upgrades

The volume holds `/data/chatto/chatto.toml`, including generated secrets, and `/data/chatto/jetstream/`, including messages and uploaded files. Keep one Machine attached to this volume. A deploy or host failure can cause downtime. Keep an independent backup of the configuration and Chatto data. See [Backup and restore](https://docs.chatto.run/guides/operations/backup-restore/).

To upgrade, edit the image tag in the generated `fly-acme-chat/fly.toml` and deploy that configuration.

```sh
cd fly-acme-chat
flyctl deploy --ha=false
```

After the deploy, sign in and open a message and an attachment that you created before the upgrade.
