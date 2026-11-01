FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /exchange ./cmd/exchange

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S exchange && adduser -S -G exchange exchange
WORKDIR /app
COPY --from=build /exchange /app/exchange
COPY config /app/config
USER exchange
EXPOSE 8888
ENTRYPOINT ["/app/exchange"]
