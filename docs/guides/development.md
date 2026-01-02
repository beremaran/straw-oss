# 🛠️ Development Guide

This guide covers how to set up your local environment for contributing to Straw Proxy Server.

## 📂 Project Structure

We follow a standard Go project layout:

* **`cmd/`**: Entry points for applications.
  * `relay-server/`: The "Brain" of the operation.
  * `endpoint/`: The "Muscle" (worker node).
* **`internal/`**: Private application code (business logic).
* **`pkg/`**: Public libraries and shared code (e.g., protocol definitions).
* **`migrations/`**: SQL schema changes.

## ⚡ Local Setup

**Prerequisite**: Ensure you are in the root directory of the project.

```bash
cp .env.example .env
```

### 2. GUI Build Dependencies (Linux)

If you plan to build or run the graphical version of the endpoint (`endpoint-gui`) on Linux, you must install the following development libraries:

```bash
# Example for Debian/Ubuntu
sudo apt-get install libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libgl1-mesa-dev libxxf86vm-dev
```

### 3. Start Infrastructure

Spin up Postgres, Redis, and RabbitMQ:

```bash
make dev-up
```

### 3. Apply Database Migrations

Create the necessary tables:

```bash
make migrate-up
```

## 🏗️ Build & Run

You can run the components directly with `go run`:

**Relay Server**:

```bash
go run cmd/relay-server/main.go
```

**Endpoint**:

```bash
go run cmd/endpoint/main.go
```

## 🧪 Testing

We have a comprehensive test suite.

* **Run Unit Tests**:

    ```bash
    make test
    ```

* **Run Linter**:

    ```bash
    make lint
    ```

## 📦 Database Migrations

We use `goose` for migrations.

* **Create a new migration**:

    ```bash
    # Usage: goose -dir migrations/ create [name] sql
    goose -dir migrations/ create add_users_table sql
    ```

* **Apply migrations**: `make migrate-up`
* **Rollback**: `make migrate-down`
