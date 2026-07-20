# AWS EC2 Docker Compose Deploy

This guide deploys `go-order-service` to a single Ubuntu EC2 instance with Docker Compose.

Current production-inspired setup:

- EC2 runs Docker Compose
- A single Kafka broker/controller runs in KRaft mode with persistent storage
- Nginx is the public reverse proxy on `80/443`
- The Go app image is built by GitHub Actions and pushed to GHCR
- GitHub Actions deploys to EC2 over SSH
- SSH access for GitHub Actions is opened temporarily with an AWS security group rule
- HTTPS is served by Nginx with a Let's Encrypt certificate managed by Certbot

## EC2

Recommended first-pass setup:

- Region: `ap-southeast-1`
- AMI: Ubuntu Server 24.04 LTS
- Instance type: choose from measured CPU and memory usage; Kafka adds meaningful overhead to the existing stack
- Storage: 30 GiB gp3
- Elastic IP: recommended so GitHub Actions, DNS, and HTTPS use a stable address
- Security group inbound rules:
  - SSH `22` from your IP only
  - HTTP `80` from `0.0.0.0/0`
  - HTTPS `443` from `0.0.0.0/0`
  - Grafana `3000` from your IP only, if you want browser access to Grafana

Do not expose PostgreSQL, Redis, Kafka, publisher metrics, consumer metrics, or Prometheus publicly.

## Install Docker

SSH into the instance:

```bash
ssh -i go-order-service-key.pem ubuntu@<public-ip>
```

Install Docker:

```bash
sudo apt update
sudo apt install -y ca-certificates curl gnupg git
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker ubuntu
exit
```

SSH back in, then verify:

```bash
docker --version
docker compose version
docker run hello-world
```

## Deploy

Clone the repository:

```bash
git clone https://github.com/hieutrinh02/go-order-service.git
cd go-order-service
```

Create the production environment file:

```bash
cp .env.prod.example .env.prod
nano .env.prod
```

Generate a stable Kafka cluster ID before the first Kafka startup:

```bash
docker run --rm apache/kafka:4.3.1 \
  /opt/kafka/bin/kafka-storage.sh random-uuid
```

Copy the generated value into `.env.prod`. Do not change it after the `kafka_data` volume has been initialized.

Configure the required production values, using strong secrets for credentials:

```env
POSTGRES_PASSWORD=<long-random-password>
DATABASE_URL=postgres://orderservice:<same-password>@postgres:5432/order_service?sslmode=disable
JWT_SECRET=<long-random-secret>
GRAFANA_ADMIN_PASSWORD=<long-random-password>
COOKIE_SECURE=true
KAFKA_CLUSTER_ID=<generated-cluster-id>
KAFKA_BOOTSTRAP_SERVERS=kafka:29092
KAFKA_TOPIC=order.events.v1
KAFKA_CONSUMER_GROUP=notification-consumer-v1
```

`GRAFANA_ADMIN_USER` defaults to `admin`; override it too if you do not want the default username.
Use `COOKIE_SECURE=false` only for plain HTTP/local testing. With the HTTPS domain, refresh cookies should be marked secure.

Start the stack:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d
```

Check containers:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml ps
```

Follow logs:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml logs -f api_1 api_2 publisher consumer migrate
```

Check Kafka and the one-shot topic initializer:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml ps -a kafka kafka-init publisher consumer
```

Expected state:

```text
kafka       Up (healthy)
kafka-init  Exited (0)
publisher   Up
consumer    Up
```

Describe the application topic:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml exec kafka \
  /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka:29092 \
  --describe \
  --topic order.events.v1
```

The current Compose deployment creates three partitions with replication factor one and `min.insync.replicas=1`.

Verify from the EC2 instance:

```bash
curl http://localhost/healthz
curl http://localhost/readyz
```

Verify from your local machine:

```bash
curl http://<public-ip>/healthz
curl http://<public-ip>/readyz
```

## Kafka Deployment Scope

The current EC2 deployment runs one Kafka process with combined broker and KRaft controller roles. Kafka data is persisted in the `kafka_data` Docker volume, the application topic is created explicitly by `kafka-init`, and Kafka is reachable only inside the Compose network at `kafka:29092`.

This is a production-inspired single-instance deployment, not a highly available Kafka cluster. The broker, controller, and EC2 host remain a single point of failure, and replication factor one cannot tolerate broker loss. A higher-availability deployment should use multiple brokers and controllers across failure domains or a managed Kafka service, together with encrypted and authenticated client connections.

After an end-to-end order and payment test, inspect the notification consumer group:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml exec kafka \
  /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server kafka:29092 \
  --group notification-consumer-v1 \
  --describe
```

For partitions containing records, `CURRENT-OFFSET` should catch up to `LOG-END-OFFSET` and `LAG` should be zero.

## Nginx Reverse Proxy and HTTPS

The production Compose stack runs Nginx as the public HTTP/HTTPS entrypoint:

```text
Frontend client -> EC2:443 -> nginx -> /usr/share/nginx/html
API client      -> EC2:443 -> nginx -> api_1:8080 / api_2:8080
```

After Nginx is deployed, the EC2 security group should allow HTTP `80` and HTTPS `443` publicly. The API port `8080` does not need to be exposed publicly.

Verify through Nginx:

```bash
curl -I https://go-order-service.hieutrinh02.dev
curl -i https://api.go-order-service.hieutrinh02.dev/healthz
curl -i https://api.go-order-service.hieutrinh02.dev/readyz
```

HTTP requests should return a `301` redirect response with a HTTPS `Location` header. HTTPS requests should return `200 OK`.

The Compose production stack uses two explicit API services for a simple load-balancing demo. Dynamic replica discovery is better handled by Traefik, Kubernetes Service, or AWS ALB.

Check the running API replicas:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml ps api_1 api_2
```

Nginx uses round-robin load balancing and returns an `X-Upstream-Addr` response header so you can see which API container handled a request:

```bash
for i in $(seq 1 10); do
  curl -s -D - https://api.go-order-service.hieutrinh02.dev/healthz -o /dev/null | grep -i x-upstream-addr
done
```

## Domain and Let's Encrypt

The current deployment uses:

```text
go-order-service.hieutrinh02.dev     -> Elastic IP -> Nginx frontend
api.go-order-service.hieutrinh02.dev -> Elastic IP -> Nginx API proxy
```

Cloudflare DNS record:

```text
Type: A
Name: go-order-service
Target: <elastic-ip>
Proxy status: DNS only

Type: A
Name: api.go-order-service
Target: <elastic-ip>
Proxy status: DNS only
```

The `.dev` TLD is HTTPS-first in browsers, so verify setup with `curl` while configuring certificates.

Certbot is included in `docker-compose.prod.yml` and shares two Docker volumes with Nginx:

```text
certbot_www    ACME HTTP-01 challenge files
certbot_certs  Let's Encrypt certificate and private key
```

Initial certificate issue command:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm certbot certonly \
  --webroot \
  --webroot-path /var/www/certbot \
  --email <email> \
  --agree-tos \
  --no-eff-email \
  -d go-order-service.hieutrinh02.dev \
  -d api.go-order-service.hieutrinh02.dev
```

Certificate files are stored inside the `certbot_certs` volume:

```text
/etc/letsencrypt/live/go-order-service.hieutrinh02.dev/fullchain.pem
/etc/letsencrypt/live/go-order-service.hieutrinh02.dev/privkey.pem
```

Manual renewal:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm certbot renew
docker compose --env-file .env.prod -f docker-compose.prod.yml exec nginx nginx -s reload
```

## GitHub Actions CI/CD

The repository includes a CI/CD workflow at `.github/workflows/deploy.yml`.

On pull requests, it runs:

```bash
go test ./...
APP_ENV_FILE=.env.prod.example docker compose --env-file .env.prod.example -f docker-compose.prod.yml config --quiet
```

On pushes to `main`, it:

1. Runs the same checks.
2. Builds the Docker image in GitHub Actions.
3. Pushes the image to GHCR with both `latest` and commit SHA tags.
4. Uses AWS OIDC to assume an IAM role.
5. Adds the GitHub runner public IP as a temporary `/32` SSH ingress rule.
6. SSHes into EC2.
7. Pulls and starts the GHCR image with Docker Compose.
8. Runs local health checks.
9. Revokes the temporary SSH ingress rule.

Configure these GitHub repository variables:

```text
APP_IMAGE=ghcr.io/hieutrinh02/go-order-service
AWS_REGION=ap-southeast-1
EC2_HOST=<elastic-ip-or-dns>
EC2_USER=ubuntu
EC2_PROJECT_DIR=/home/ubuntu/go-order-service
EC2_SECURITY_GROUP_ID=<security-group-id>
```

Configure these GitHub repository secrets:

```text
AWS_ROLE_TO_ASSUME=<iam-role-arn>
EC2_SSH_KEY=<private-key-content>
```

## Operations

Restart:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml restart
```

Pull new code and redeploy:

```bash
git pull
docker compose --env-file .env.prod -f docker-compose.prod.yml pull
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --remove-orphans
```

Stop containers while keeping PostgreSQL, Kafka, and Prometheus data volumes:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml down
```

Remove containers and volumes:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml down -v
```

Use `down -v` only when you are comfortable deleting local PostgreSQL, Kafka, Prometheus, and Grafana data.
