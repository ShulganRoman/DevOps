FROM golang:1.23-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod ./
RUN go mod download

COPY ["cmd", "./cmd"]

RUN go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/server


FROM alpine:3.21 AS runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
	&& addgroup -S app \
	&& adduser -S -G app -h /app app

COPY --from=builder /out/app /app/app

ENV APP_HOST=0.0.0.0 \
	APP_PORT=8080 \
	APP_NAME="Note manager" \
	APP_USERS="demo:demo" \
	APP_SESSION_COOKIE="notes_session" \
	APP_READ_TIMEOUT=5s \
	APP_WRITE_TIMEOUT=10s \
	APP_SHUTDOWN_TIMEOUT=10s

EXPOSE 8080

USER app

ENTRYPOINT ["/app/app"]
