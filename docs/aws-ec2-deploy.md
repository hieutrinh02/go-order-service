# AWS EC2 Docker Compose Deploy

This guide deploys `go-order-service` to a single Ubuntu EC2 instance with Docker Compose.

## EC2

Recommended first-pass setup:

- Region: `ap-southeast-1`
- AMI: Ubuntu Server 24.04 LTS
- Instance type: `t3.micro` for a cheap test, `t3.small` for more headroom
- Storage: 30 GiB gp3
- Security group inbound rules:
  - SSH `22` from your IP only
  - HTTP `80` from your IP only while testing

Do not expose PostgreSQL, NATS, publisher metrics, consumer metrics, or Prometheus publicly.

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

Set strong values for:

```env
POSTGRES_PASSWORD=<long-random-password>
DATABASE_URL=postgres://orderservice:<same-password>@postgres:5432/order_service?sslmode=disable
JWT_SECRET=<long-random-secret>
COOKIE_SECURE=false
```

Start the stack:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build
```

Check containers:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml ps
```

Follow logs:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml logs -f api_1 api_2 publisher consumer migrate
```

Verify from the EC2 instance:

```bash
curl http://localhost/healthz
curl http://localhost/readyz
```

Verify from your local machine if HTTP port `80` is allowed from your IP:

```bash
curl http://<public-ip>/healthz
curl http://<public-ip>/readyz
```

## Nginx Reverse Proxy

The production Compose stack runs Nginx as the public HTTP entrypoint:

```text
Client -> EC2:80 -> nginx -> api_1:8080 / api_2:8080
```

After Nginx is deployed, the EC2 security group should allow HTTP `80` from your IP. The API port `8080` does not need to be exposed publicly.

Verify through Nginx:

```bash
curl -i http://<public-ip>/healthz
curl -i http://<public-ip>/readyz
```

The Compose production stack uses two explicit API services for a simple load-balancing demo. Dynamic replica discovery is better handled by Traefik, Kubernetes Service, or AWS ALB.

Check the running API replicas:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml ps api_1 api_2
```

Nginx uses round-robin load balancing and returns an `X-Upstream-Addr` response header so you can see which API container handled a request:

```bash
for i in $(seq 1 10); do
  curl -s -D - http://<public-ip>/healthz -o /dev/null | grep -i x-upstream-addr
done
```

## Operations

Restart:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml restart
```

Pull new code and redeploy:

```bash
git pull
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build
```

Stop containers while keeping database and Prometheus volumes:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml down
```

Remove containers and volumes:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml down -v
```

Use `down -v` only when you are comfortable deleting local PostgreSQL and Prometheus data.
