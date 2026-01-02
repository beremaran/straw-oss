# Migration to NATS

## Goal

Replace RabbitMQ with NATS as the message broker for the `datadot-proxy-server`.

## Context

The current system uses RabbitMQ with an AMQP-centric interface (`MessageBroker`). The interface includes methods like `DeclareExchange`, `BindQueue`, which are specific to AMQP.

## Strategy

### 1. Interface Adaptation

We have two options:

1. **Refactor Interface**: Remove AMQP-specific terms (`Exchange`, `Bind`) and use generic terms (`Topic`, `Subscribe`). This touches the consuming code (`endpoint/*`, `service/*`).
2. **Adapt NATS**: Map AMQP concepts to NATS.
    * `Publish(exchange, routingKey)` -> `Publish(subject="{exchange}.{routingKey}")`.
    * `Subscribe(queue, handler)` -> `QueueSubscribe(subject="mapped_from_queue", queue=queue, handler)`.
    * `DeclareExchange` -> No-op (NATS subjects are dynamic) or JetStream Stream creation.

**Recommendation**: **Adapt NATS initially** (Option 2) to minimize code churn, then refactor the interface later if needed. However, since the user asked for a "big task", we should probably verify if a refactor is cleaner.
Looking at `broker.go`, `DeclareExchange` and `BindQueue` are used. If we remove them, we need to know *what* subjects to subscribe to.
In AMQP: Exchange + RoutingKey --bind--> Queue.
In NATS: Publisher -> Subject -> Subscriber.

If we keep the interface, `BindQueue(queue, exchange, routingKey)` could simply register that `queue` is interested in `exchange.routingKey`.
`Subscribe(queue, handler)` would then subscribe to the subjects bound to that queue.

But NATS "Queue Groups" are strictly for load balancing subscribers.
A simpler mapping might be:
* `exchange` + `routingKey` = `subject` (e.g., `events.task_created`).
* The Consumer knows what subject it wants.

Let's assume we implement `NatsBroker` adhering to the current interface but treating `DeclareExchange` / `DeclareQueue` / `BindQueue` as setup steps for internal mapping or JetStream streams.

### 2. Dependency

Use `github.com/nats-io/nats.go`.

### 3. Implementation Details (`internal/broker/nats.go`)

* **Struct**:

    ```go
    type NatsBroker struct {
        conn *nats.Conn
        js   nats.JetStreamContext // If using JetStream
    }
    ```

* **Publish**:
    `nc.Publish(fmt.Sprintf("%s.%s", exchange, routingKey), body)`
* **Subscribe**:
    `nc.QueueSubscribe(subject, queue, handler)`
    Wait, `Subscribe` in the interface takes `(ctx, queue, handler)`. It doesn't take a subject/routing key.
    The binding happens via `BindQueue`.
    So `NatsBroker` must store bindings in memory: `bindings[queue] = []subject`.
    When `Subscribe(queue)` is called, we look up the subjects bound to that queue and subscribe to them.
    BUT: `BindQueue` might be called *after* `Subscribe`? Usually setup is before.
    If `BindQueue` is called, we must ensure the explicit subscription exists.

    *Alternative*: Refactor the application code to not use `BindQueue` separate from `Subscribe`.
    Most modern Go apps with NATS just do `Subscribe(topic, handler)`.
    The current usage in the codebase needs to be checked.

### 4. Configuration

Add `NatsURL` to `config.Config`.

## Plan Steps

1. **Repo Check**: See how `internal/broker` is used. (`grep -r "DeclareExchange"`, `grep -r "BindQueue"`).
2. **Draft Implementation**: Create `internal/broker/nats.go`.
3. **Switch**: Update `main.go`.

## Verification

- Run integration tests.
* Check load tests (NATS should be faster).
