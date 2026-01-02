# ⚡ Quick Start

This guide will get you from zero to a fully functional proxy network in under 5 minutes. You'll run the "Brain" (Relay Server) and the "Muscle" (Endpoint) on your local machine.

## Prerequisites

Before we begin, ensure you have:

* **[Go 1.22+](https://go.dev/dl/)**: Required to compile the server and endpoint.
* **[Docker](https://www.docker.com/)**: Required for the database and message broker.
* **Make**: Used for running setup scripts.

---

## Step 1: Start Infrastructure

We need a database (PostgreSQL), a cache (Redis), and a message broker (RabbitMQ). We've packaged this into a convenient Docker Compose file.

1. **Clone the repository** (if you haven't already):

    ```bash
    git clone https://github.com/kwilabs/straw-proxy-server.git
    cd straw-proxy-server
    ```

2. **Start the services**:

    ```bash
    make dev-up
    ```

    > ⏳ **Wait a moment**: It might take 10-20 seconds for RabbitMQ and Postgres to be fully ready.

3. **Run Database Migrations**:

    ```bash
    make migrate-up
    ```

## Step 2: Start the Relay Server ("The Brain")

The Relay Server is the entry point for your proxy requests. It handles authentication, routing, and dispatching tasks to endpoints.

Open a **new terminal tab** and run:

```bash
go run cmd/relay-server/main.go
```

✅ **Success**: You should see logs indicating the server is listening on `:8080`.

## Step 3: Start an Endpoint ("The Muscle")

Now we need a worker to actually perform the requests. In a real deployment, this would run on a different server (or residential IP), but for now, we'll run it locally.

Open **another terminal tab** and run:

```bash
go run cmd/endpoint/main.go
```

✅ **Success**: You should see logs like `Connected to broker` and `Waiting for messages`.

## Step 4: Make Your First Request! 🚀

Now the magic happens. We will send a request to the Relay Server, which will route it to the Endpoint, which will fetch the target URL and send the response back.

Run this `curl` command:

```bash
curl -x http://localhost:8080 \
     -H "X-Relay-Tags: target=httpbin" \
     http://httpbin.org/ip
```

**What just happened?**

1. You sent a request to `localhost:8080`.
2. The Relay Server received it and published a task to RabbitMQ.
3. The Endpoint picked up the task.
4. The Endpoint requested `httpbin.org/ip`.
5. Reflexively, `httpbin.org` saw the Endpoint's IP (your local IP in this case).
6. The response traveled back through RabbitMQ -> Server -> You.

## Next Steps

Now that you have it running:

* **[Learn the Architecture](../architecture/overview.md)**: Understand the "Passive Consumer" model.
* **[Deploy to Production](../guides/deployment.md)**: Learn how to run this on real servers.
