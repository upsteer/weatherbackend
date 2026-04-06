FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o weather-api .

FROM alpine:3.20

WORKDIR /app

RUN adduser -D -H -u 10001 appuser

COPY --from=builder /app/weather-api /app/weather-api

USER appuser

EXPOSE 5000

ENTRYPOINT ["/app/weather-api"]
