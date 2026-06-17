# Chatto Kubernetes Example

This example deploys a clustered Chatto setup on Kubernetes with:

- **NATS** - StatefulSet with persistent storage
- **Chatto** - Deployment with 3 replicas
- **Ingress** - For external access with TLS

## Prerequisites

- Kubernetes cluster (1.19+)
- kubectl configured
- An Ingress controller (e.g., ingress-nginx, Traefik)
- Optional: cert-manager for automatic TLS

## Quick Start: Single-Node Cluster with k3s

If you don't have a Kubernetes cluster, you can set one up on a single VM using [k3s](https://k3s.io/):

```bash
# Install k3s (includes kubectl, Traefik ingress, and local-path storage)
curl -sfL https://get.k3s.io | sh -

# Verify installation
sudo k3s kubectl get nodes

# Copy kubeconfig for regular kubectl usage
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $USER ~/.kube/config
chmod 600 ~/.kube/config

# Now kubectl works without sudo
kubectl get nodes
```

k3s includes:

- **Traefik** as the default Ingress controller (the manifests already use this)
- **local-path-provisioner** for persistent volumes
- **CoreDNS** for service discovery

## Setting Up Let's Encrypt with cert-manager

Install cert-manager for automatic TLS certificates:

```bash
# Install cert-manager (check for newer versions!)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl -n cert-manager rollout status deployment/cert-manager
kubectl -n cert-manager rollout status deployment/cert-manager-webhook
```

Create a ClusterIssuer for Let's Encrypt:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com  # Change this!
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            ingressClassName: traefik  # or nginx
EOF
```

The included `ingress.yaml` is already configured to use cert-manager. After applying it, verify the certificate:


```bash
# Check certificate status
kubectl -n chatto get certificate
kubectl -n chatto describe certificate chatto-tls
```

## Files

| File             | Description                                        |
| ---------------- | -------------------------------------------------- |
| `namespace.yaml` | Dedicated namespace for Chatto                     |
| `secrets.yaml`   | **Central config** - all environment variables     |
| `nats.yaml`      | NATS StatefulSet (sources token from secret)       |
| `chatto.yaml`    | Chatto Deployment (sources all config from secret) |
| `ingress.yaml`   | Ingress for external access                        |

## Configuration

All configuration lives in manifest files. Copy and edit them for your environment:

```bash
cp secrets.yaml secrets.local.yaml
cp ingress.yaml ingress.local.yaml
```

### secrets.local.yaml

Update these values (generate secrets with `openssl rand -hex 32`):

- `NATS_TOKEN` and `CHATTO_NATS_CLIENT_TOKEN` - Must match
- `CHATTO_WEBSERVER_URL` - Your domain (e.g., `https://chat.example.com`)
- `CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET` - Session signing secret
- `CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET` - Session encryption secret
- `CHATTO_CORE_SECRET_KEY` - Bearer-token and account-flow verifier key
- `CHATTO_CORE_ASSETS_SIGNING_SECRET` - Asset URL signing secret

### ingress.local.yaml

Update these values:

- `host` and `tls.hosts` - Your domain
- `ingressClassName` - Your ingress controller (default: `traefik`)

## Deployment

```bash
kubectl apply -f namespace.yaml
kubectl apply -f secrets.local.yaml
kubectl apply -f nats.yaml
kubectl apply -f chatto.yaml
kubectl apply -f ingress.local.yaml
```

## Management

```bash
# Check status
kubectl -n chatto get pods
kubectl -n chatto get svc
kubectl -n chatto get ingress

# View logs
kubectl -n chatto logs -f deployment/chatto

# Scale replicas
kubectl -n chatto scale deployment/chatto --replicas=5

# Rolling restart
kubectl -n chatto rollout restart deployment/chatto

# Watch rollout status
kubectl -n chatto rollout status deployment/chatto
```

## Updating

```bash
# Update to a new image
kubectl -n chatto set image deployment/chatto chatto=ghcr.io/chattocorp/chatto:v1.2.3

# Or update the manifest and apply
kubectl apply -f chatto.yaml
```

The deployment uses a rolling update strategy with `maxUnavailable: 0` to ensure zero-downtime updates.

## Storage

NATS uses a PersistentVolumeClaim for data persistence. The default request is 10Gi. Adjust in `nats.yaml` if needed.

## Troubleshooting

**Pods not starting**: Check events and logs:

```bash
kubectl -n chatto describe pod <pod-name>
kubectl -n chatto logs <pod-name>
```

**Chatto can't connect to NATS**: Ensure NATS is running and the token matches:

```bash
kubectl -n chatto get pods -l app=nats
kubectl -n chatto logs statefulset/nats
```

**Ingress not working**: Verify your Ingress controller is installed and check the ingress status:

```bash
kubectl -n chatto describe ingress chatto
```

**TLS issues**: If using cert-manager, check certificate status:

```bash
kubectl -n chatto get certificate
kubectl -n chatto describe certificate chatto-tls
```
