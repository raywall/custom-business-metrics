FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/generator ./cmd/generator

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/generator /app/generator
ENTRYPOINT ["/app/generator"]
