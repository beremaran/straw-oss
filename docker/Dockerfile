ARG BASE_IMAGE=beremaran/straw:base
FROM ${BASE_IMAGE} AS builder

FROM alpine:latest

ARG BINARY_NAME=control
ENV BINARY_NAME=${BINARY_NAME}

WORKDIR /app
RUN apk add --no-cache ca-certificates curl

COPY --from=builder /app/${BINARY_NAME} /app/app

RUN adduser -D -g '' appuser \
    && chown appuser:appuser /app/app \
    && chmod +x /app/app

USER appuser

ENTRYPOINT ["/app/app"]
