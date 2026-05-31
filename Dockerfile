FROM golang:1.25-alpine AS service-build
WORKDIR /src/service
COPY service/go.mod service/go.sum ./
RUN go mod download
COPY service ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/metrics-service ./cmd/service

FROM golang:1.25-alpine AS agent-build
WORKDIR /src/agent
COPY agent/go.mod ./
RUN go mod download
COPY agent ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/metrics-agent ./cmd/agent

FROM alpine:3.22

# Corporate CA notes:
# - Alpine trusts certificates installed under /usr/local/share/ca-certificates.
# - To enable an internal CA, copy a .crt file to that directory and run
#   update-ca-certificates.
# - For AWS SDK/CLI calls that need a custom bundle, set:
#   ENV AWS_CA_BUNDLE=/usr/local/share/ca-certificates/internal-ca.crt
# - Example:
#   COPY certs/internal-ca.crt /usr/local/share/ca-certificates/internal-ca.crt
#   RUN update-ca-certificates
RUN apk add --no-cache ca-certificates curl && update-ca-certificates

ENV SERVICE_ADDR=:8080
ENV AGENT_UDP_ADDR=:8125
ENV SERVICE_INGEST_URL=http://127.0.0.1:8080/v1/metrics
ENV METRICS_WEBVIEW_ADDR=0.0.0.0:5173

WORKDIR /opt/custom-business-metrics
COPY --from=service-build /out/metrics-service /usr/local/bin/metrics-service
COPY --from=agent-build /out/metrics-agent /usr/local/bin/metrics-agent
COPY webview ./webview
COPY docker-entrypoint.sh /usr/local/bin/custom-business-metrics-entrypoint
RUN chmod +x /usr/local/bin/custom-business-metrics-entrypoint

EXPOSE 8080 5173
EXPOSE 8125/udp

ENTRYPOINT ["/usr/local/bin/custom-business-metrics-entrypoint"]
