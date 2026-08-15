FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /limitry ./cmd/gateway

FROM alpine:3.22

RUN apk add --no-cache ca-certificates
COPY --from=builder /limitry /usr/local/bin/limitry

ENTRYPOINT ["limitry"]
CMD ["--config", "/etc/limitry/config.yaml"]
