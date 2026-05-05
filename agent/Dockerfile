FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/agent ./cmd/agent

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/agent /app/agent
EXPOSE 8125/udp
ENTRYPOINT ["/app/agent"]
