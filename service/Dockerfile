FROM golang:1.26-alpine AS build

WORKDIR /src

COPY . .

RUN apk add --no-cache ca-certificates git && update-ca-certificates
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/service ./cmd/service

# Corporate CA notes:
# - Alpine trusts certificates installed under /usr/local/share/ca-certificates.
# - To enable an internal CA, copy a .crt file to that directory and run
#   update-ca-certificates.
# - For AWS SDK/CLI calls that need a custom bundle, set:
#   ENV AWS_CA_BUNDLE=/usr/local/share/ca-certificates/internal-ca.crt
# - Example:
#   COPY certs/internal-ca.crt /usr/local/share/ca-certificates/internal-ca.crt
#   RUN update-ca-certificates
FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates curl && update-ca-certificates
COPY --from=build /out/service .

EXPOSE 8080

CMD ["/app/service"]
