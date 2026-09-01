# AWS EC2 K3s Deployment

This guide deploys the order service to a single-node K3s cluster on the existing EC2 instance.

The deployment is production-inspired but not highly available:

- One EC2 node
- K3s with its bundled Traefik ingress controller
- Local-path persistent volumes
- PostgreSQL, Redis and Kafka run inside Kubernetes
- Traefik serves the frontend and API on ports 80 and 443
- Existing Docker volumes are retained for rollback

## Architecture

```text
Internet
  |
  v
EC2 :80/:443
  |
  v
K3s Traefik
  |-- go-order-service.hieutrinh02.dev
  |     -> frontend Service
  |     -> Nginx Pod
  |     -> /home/ubuntu/go-order-service-fe/dist
  |
  `-- api.go-order-service.hieutrinh02.dev
        -> API Service
        -> API Pods
```

## Internal workloads

```text
API        -> PostgreSQL + Redis
Publisher  -> PostgreSQL + Kafka
Consumer   -> PostgreSQL + Kafka
Prometheus -> API + publisher + consumer metrics
Grafana    -> Prometheus
```

## Scope and data

The first K3s deployment starts with new Kubernetes persistent volumes. It does not migrate data from the existing Docker Compose volumes.

The Docker Compose volumes are retained for rollback or a later data migration. Do not run the following command unless the old data is no longer needed:

```bash
docker compose -f docker-compose.prod.yml down -v
```

## 1. Prepare the EC2 instance

SSH to the EC2 instance and check its available resources:

```bash
ssh ubuntu@<ec2-public-ip>

free -h
df -h
nproc
```

For this single-node learning environment, aim for roughly 4 GiB of RAM. Prefer 30–40 GiB of free disk space for container images, K3s/containerd data, logs, frontend files and the retained Docker volumes.

Pull the latest backend and frontend code:

```bash
cd /home/ubuntu/go-order-service
git pull

cd /home/ubuntu/go-order-service-fe
git pull
```

## 2. Build the frontend

The frontend Pod serves the build output from `/home/ubuntu/go-order-service-fe/dist` through a read-only host mount.

```bash
cd /home/ubuntu/go-order-service-fe

npm ci
VITE_API_BASE_URL=https://api.go-order-service.hieutrinh02.dev npm run build
test -f dist/index.html
```

Rebuild the frontend whenever its source code or production environment variables change.

## 3. Back up the existing PostgreSQL database

Create a database backup before stopping Docker Compose:

```bash
cd /home/ubuntu/go-order-service

backup_dir=/home/ubuntu/order-service-backups
install -d -m 700 "$backup_dir"

docker compose --env-file .env.prod -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U orderservice -d order_service -Fc \
  >"$backup_dir/order-service-before-k3s.dump"

chmod 600 "$backup_dir/order-service-before-k3s.dump"
test -s "$backup_dir/order-service-before-k3s.dump"
```

The backup is for safety. The initial K3s deployment intentionally starts with an empty PostgreSQL volume.

## 4. Export the existing TLS certificate

Export the certificate while the Docker Compose Nginx container is still running:

```bash
cd /home/ubuntu/go-order-service

tls_dir=/home/ubuntu/.local/share/go-order-service/tls
install -d -m 700 "$tls_dir"

nginx_container="$(docker compose --env-file .env.prod -f docker-compose.prod.yml ps -q nginx)"
test -n "$nginx_container"

docker exec "$nginx_container" \
  cat /etc/letsencrypt/live/go-order-service.hieutrinh02.dev/fullchain.pem \
  >"$tls_dir/fullchain.pem"

docker exec "$nginx_container" \
  cat /etc/letsencrypt/live/go-order-service.hieutrinh02.dev/privkey.pem \
  >"$tls_dir/privkey.pem"

chmod 600 "$tls_dir/fullchain.pem" "$tls_dir/privkey.pem"
openssl x509 -in "$tls_dir/fullchain.pem" -noout -dates -ext subjectAltName
```

Confirm that the certificate is valid and covers both domains:

- `go-order-service.hieutrinh02.dev`
- `api.go-order-service.hieutrinh02.dev`

## 5. Stop Docker Compose

Stop the old stack without deleting its volumes:

```bash
cd /home/ubuntu/go-order-service
docker compose --env-file .env.prod -f docker-compose.prod.yml down
```

Confirm that ports 80 and 443 are free for K3s Traefik:

```bash
sudo ss -lntp | grep -E ':(80|443)\b' || true
```

## 6. Install K3s

Install the single-node K3s server with its bundled Traefik ingress controller and local-path storage provisioner:

```bash
curl -sfL https://get.k3s.io | sh -
```

Configure `kubectl` for the current user:

```bash
mkdir -p /home/ubuntu/.kube
sudo cp /etc/rancher/k3s/k3s.yaml /home/ubuntu/.kube/config
sudo chown ubuntu:ubuntu /home/ubuntu/.kube/config
chmod 600 /home/ubuntu/.kube/config

export KUBECONFIG=/home/ubuntu/.kube/config
```

Optionally persist `KUBECONFIG` for future SSH sessions:

```bash
grep -qxF 'export KUBECONFIG=/home/ubuntu/.kube/config' /home/ubuntu/.profile \
  || echo 'export KUBECONFIG=/home/ubuntu/.kube/config' >>/home/ubuntu/.profile
```

Verify the cluster components:

```bash
kubectl wait --for=condition=Ready node --all --timeout=180s
kubectl -n kube-system rollout status deployment/traefik --timeout=180s
kubectl wait --for=condition=Established \
  crd/middlewares.traefik.io \
  --timeout=180s

kubectl get nodes
kubectl get pods -n kube-system
kubectl get storageclass
```

The `local-path` StorageClass keeps persistent data on this EC2 node. It is suitable for this single-node deployment, but it does not provide high availability.

## 7. Create the application Secrets

Create the namespace first:

```bash
cd /home/ubuntu/go-order-service
kubectl apply -f deploy/k8s/base/namespace.yaml
```

Build the application Secret from the existing production environment file without committing secret values:

```bash
secret_env="$(mktemp /tmp/order-service-secret.XXXXXX)"
chmod 600 "$secret_env"

grep -E '^(DATABASE_URL|POSTGRES_PASSWORD|JWT_SECRET|GRAFANA_ADMIN_USER|GRAFANA_ADMIN_PASSWORD)=' \
  .env.prod >"$secret_env"

for key in DATABASE_URL POSTGRES_PASSWORD JWT_SECRET GRAFANA_ADMIN_USER GRAFANA_ADMIN_PASSWORD; do
  grep -q "^${key}=" "$secret_env" || {
    echo "missing required key: ${key}" >&2
    exit 1
  }
done

kubectl -n order-service create secret generic order-service-secret \
  --from-env-file="$secret_env" \
  --dry-run=client -o yaml \
  | kubectl apply -f -

rm -f "$secret_env"
```

The Kubernetes Services use the same internal hostnames as Docker Compose (`postgres`, `redis` and `kafka`), so the existing production connection URLs remain compatible.

Create the TLS Secret from the exported certificate:

```bash
tls_dir=/home/ubuntu/.local/share/go-order-service/tls

kubectl -n order-service create secret tls order-service-tls \
  --cert="$tls_dir/fullchain.pem" \
  --key="$tls_dir/privkey.pem" \
  --dry-run=client -o yaml \
  | kubectl apply -f -
```

Verify only the Secret names and types, not their values:

```bash
kubectl -n order-service get secrets
```

## 8. Deploy the application

The GHCR image can be pulled without additional configuration when the package is public. If the package is private, create an image pull Secret using a GitHub token that can read the package:

```bash
read -rsp 'GHCR token: ' GHCR_TOKEN
echo

kubectl -n order-service create secret docker-registry ghcr-pull-secret \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password="$GHCR_TOKEN" \
  --dry-run=client -o yaml \
  | kubectl apply -f -

unset GHCR_TOKEN

kubectl -n order-service patch serviceaccount default \
  --type=merge \
  -p '{"imagePullSecrets":[{"name":"ghcr-pull-secret"}]}'
```

Skip this block when the GHCR package is public. Do not commit the GitHub token or the generated Secret manifest.

Use an immutable backend image tag, preferably the Git commit SHA produced by CI:

```bash
cd /home/ubuntu/go-order-service

./scripts/deploy-k8s.sh \
  "ghcr.io/hieutrinh02/go-order-service:<git-commit-sha>"
```

Keep the GitHub Actions repository variable `K3S_DEPLOY_ENABLED` unset or set to `false` during the initial migration. After this manual deployment and its verification succeed, set it to `true` so future pushes to `main` deploy immutable images through K3s.

For a new installation, the script:

1. Applies the bootstrap overlay, which creates the full EC2 stack with API, publisher and consumer scaled to zero.
2. Waits for PostgreSQL, Redis and Kafka to become ready.
3. Waits for the Kafka topic initialization and database migration Jobs to complete.
4. Applies the normal EC2 overlay, which scales API, publisher and consumer up.
5. Waits for API, publisher, consumer, Prometheus, Grafana and frontend rollouts.

Prometheus, Grafana and frontend are created by the bootstrap overlay and may start while the infrastructure and Jobs are being prepared. They are not a separate apply stage.

For an existing installation, the script runs a fresh migration Job before applying the updated EC2 overlay. It then waits for all application and observability Deployments and prints the final resource state.

## 9. Verify Kubernetes resources

```bash
kubectl -n order-service get pods -o wide
kubectl -n order-service get deployments,statefulsets,jobs
kubectl -n order-service get services,ingresses
kubectl -n order-service get pvc
```

Expected results:

- All Pods are `Running` and ready.
- The `kafka-topic-init` and `migrate` Jobs are `Complete`.
- All persistent volume claims are `Bound`.
- The frontend and API Ingresses show ports 80 and 443.

If a workload fails, inspect it before retrying:

```bash
kubectl -n order-service describe pod <pod-name>
kubectl -n order-service logs <pod-name> --all-containers=true
```

## 10. Verify the public endpoints

Confirm that HTTP redirects to HTTPS:

```bash
curl -I http://go-order-service.hieutrinh02.dev
curl -I http://api.go-order-service.hieutrinh02.dev/healthz
```

Confirm that the frontend and API are available over HTTPS:

```bash
curl --fail --silent --show-error \
  https://go-order-service.hieutrinh02.dev >/dev/null \
  && echo 'frontend ok'

curl --fail --silent --show-error \
  https://api.go-order-service.hieutrinh02.dev/healthz \
  && echo

curl --fail --silent --show-error \
  https://api.go-order-service.hieutrinh02.dev/readyz \
  && echo
```

## 11. Verify Prometheus

From the EC2 SSH session, expose Prometheus only on localhost:

```bash
kubectl -n order-service port-forward service/prometheus 9090:9090
```

In a second EC2 SSH session:

```bash
curl --fail --silent \
  http://127.0.0.1:9090/api/v1/targets \
  | grep -o '"health":"[^"]*"'
```

The API, publisher and consumer targets should report `"health":"up"`.

## 12. Access Grafana safely

Grafana is intentionally internal-only. Create an SSH tunnel from the local machine:

```bash
ssh -L 3000:127.0.0.1:3000 ubuntu@<ec2-public-ip>
```

Inside that SSH session, start the Kubernetes port forward:

```bash
kubectl -n order-service port-forward service/grafana 3000:3000
```

Open `http://localhost:3000` locally and sign in with `GRAFANA_ADMIN_USER` and `GRAFANA_ADMIN_PASSWORD` from `.env.prod`.

## Deploy a later backend version

Pull the latest manifests, then deploy the new immutable image:

```bash
cd /home/ubuntu/go-order-service
git pull

./scripts/deploy-k8s.sh \
  "ghcr.io/hieutrinh02/go-order-service:<new-git-commit-sha>"
```

## Useful operations

View application logs:

```bash
kubectl -n order-service logs -f deployment/api
kubectl -n order-service logs -f deployment/publisher
kubectl -n order-service logs -f deployment/consumer
kubectl -n order-service logs -f statefulset/kafka
```

Inspect rollout state and history:

```bash
kubectl -n order-service rollout status deployment/api
kubectl -n order-service rollout history deployment/api
```

Roll back an application Deployment:

```bash
kubectl -n order-service rollout undo deployment/api
kubectl -n order-service rollout undo deployment/publisher
kubectl -n order-service rollout undo deployment/consumer
```

Restart application Deployments after a Secret or ConfigMap change:

```bash
kubectl -n order-service rollout restart deployment/api
kubectl -n order-service rollout restart deployment/publisher
kubectl -n order-service rollout restart deployment/consumer
```

## Roll back to Docker Compose

The Docker volumes were retained, so the old stack can be restored if needed:

```bash
sudo systemctl stop k3s

cd /home/ubuntu/go-order-service
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d
```

Verify the public endpoints after the Docker Compose stack becomes healthy.

Kubernetes and Docker Compose use separate PostgreSQL volumes. Any data written after the cutover exists only in the active environment, so plan a database migration before using this rollback path after real traffic has resumed.

To return to K3s:

```bash
cd /home/ubuntu/go-order-service
docker compose --env-file .env.prod -f docker-compose.prod.yml down
sudo systemctl start k3s
```

## TLS certificate renewal

The initial deployment imports the existing Let's Encrypt certificate into a Kubernetes Secret. This preserves HTTPS during the migration, but it does not automate certificate renewal.

Check the expiry date regularly:

```bash
tls_dir=/home/ubuntu/.local/share/go-order-service/tls
openssl x509 -in "$tls_dir/fullchain.pem" -noout -dates
```

After obtaining renewed certificate files, update the TLS Secret:

```bash
kubectl -n order-service create secret tls order-service-tls \
  --cert="$tls_dir/fullchain.pem" \
  --key="$tls_dir/privkey.pem" \
  --dry-run=client -o yaml \
  | kubectl apply -f -
```

Traefik normally reloads updated Secrets automatically. Verify the certificate served publicly after the update:

```bash
echo | openssl s_client \
  -connect go-order-service.hieutrinh02.dev:443 \
  -servername go-order-service.hieutrinh02.dev 2>/dev/null \
  | openssl x509 -noout -dates -ext subjectAltName
```

For automated renewal in a future phase, install cert-manager and replace the manually managed TLS Secret with a `Certificate` resource.
