# Deployment Guide

This guide details how to deploy the Straw Proxy Server in a production environment.

## Production Setup

We use **Docker Compose** for a simplified production deployment. Be sure to use the production configuration file (`docker-compose.prod.yml`) which includes resource limits, restart policies, and logging configurations.

### 1. Configuration

Ensure your `.env` file is configured with production-grade secrets.

* `POSTGRES_PASSWORD`: Use a strong, random password.
* `REDIS_PASSWORD`: Enable Redis authentication.
* `HMAC_SECRET`: Generate a cryptographically secure random string.
* `RABBITMQ_DEFAULT_PASS`: Set a strong password for the broker.

### 2. Launching Services

```bash
docker compose -f docker-compose.prod.yml up -d
```

This will start:

* **Relay Server**: The main entry point.
* **Endpoints**: Worker nodes (scale needs as required).
* **PostgreSQL**: Persistent data storage.
* **Redis**: Caching layer.
* **RabbitMQ**: Message broker.

### 3. Verification

Check the health of your deployment:

```bash
curl http://localhost:8080/healthz
```

## Operations

### Troubleshooting

#### Common Startup Errors

**Database Connection Failed**

* **Symptoms**: `relay-server` restarts loop, logs verify `dial tcp: lookup postgres: no such host`.
* **Fix**: Ensure the postgres container is healthy (`docker compose -f docker-compose.prod.yml ps postgres`). Verify `POSTGRES_DSN` matches credentials.

**Redis Authentication Failed**

* **Symptoms**: `NOAUTH Authentication required`.
* **Fix**: Update `REDIS_PASSWORD` in `relay-server` environment variables to match the Redis container's password.

**RabbitMQ Connection Refused**

* **Symptoms**: Services fail to connect to AMQP.
* **Fix**: RabbitMQ takes time to start. Ensure your services wait for it to be healthy. Check the management UI on port `15672`.

#### Viewing Logs

Use Docker Compose to view aggregated logs:

```bash
docker compose -f docker-compose.prod.yml logs -f relay-server
```

### Maintenance

#### Database Migrations

Migrations should ideally run on startup. If you need to run them manually:

```bash
# Assuming you have the helper script or Makefile
make migrate-up
```

#### Backups

**PostgreSQL**:

```bash
docker exec -t straw-postgres-prod pg_dumpall -c -U straw > dump_$(date +%F).sql
```

**Redis**:
Backup the `appendonly.aof` file from the `redis-prod-data` volume.

#### Updates

To update to the latest version:

1. Pull new images:

    ```bash
    docker compose -f docker-compose.prod.yml pull
    ```

2. Restart containers:

    ```bash
    docker compose -f docker-compose.prod.yml up -d
    ```
