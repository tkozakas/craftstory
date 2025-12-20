FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/craftstory cmd/craftstory/main.go

FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates ffmpeg

COPY --from=builder /app/bin/craftstory .
COPY --from=builder /app/config.yaml .

CMD ["./craftstory"]
