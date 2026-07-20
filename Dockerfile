FROM golang:1.26.2-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git build-base

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/pressly/goose/v3/cmd/goose@v3.27.1

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /out/publisher ./cmd/publisher
RUN CGO_ENABLED=1 GOOS=linux go build -tags musl -o /out/consumer ./cmd/consumer

FROM alpine:3.22 AS runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/api /app/api
COPY --from=builder /out/publisher /app/publisher
COPY --from=builder /out/consumer /app/consumer
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY migrations /app/migrations

EXPOSE 8080

CMD ["/app/api"]
