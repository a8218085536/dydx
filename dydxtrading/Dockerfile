ARG GOLANG_1_23_ALPINE_DIGEST="ac67716dd016429be8d4c2c53a248d7bcdf06d34127d3dc451bda6aa5a87bc06"

FROM golang@sha256:${GOLANG_1_23_ALPINE_DIGEST} AS builder

RUN set -eux; apk add --no-cache ca-certificates build-base git linux-headers bash

WORKDIR /workspace/dydxtrading
COPY dydxtrading/go.* ./
COPY protocol /workspace/protocol
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY dydxtrading .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    CGO_ENABLED=1 go build \
      -mod=readonly \
      -tags "netgo" \
      -trimpath \
      -o /workspace/build/dydxtrading \
      ./cmd/dydxtrading

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && mkdir -p /var/lib/dydxtrading \
    && chmod 700 /var/lib/dydxtrading

COPY --from=builder /workspace/build/dydxtrading /bin/dydxtrading

ENV DYDX_TRADING_LISTEN=127.0.0.1:8090

EXPOSE 8090

ENTRYPOINT ["/bin/dydxtrading"]
