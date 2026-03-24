## Build stage
FROM golang:1.26.1-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates && update-ca-certificates

COPY go.mod ./
RUN go mod download

COPY . ./

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build \
		-ldflags "-s -w \
			-X 'main.Version=${VERSION}' \
			-X 'main.Commit=${COMMIT}' \
			-X 'main.BuildDate=${BUILD_DATE}'" \
		-o /bot ./cmd/bot

## Runtime stage
FROM alpine:3.23

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates

COPY --from=builder /bot /app/bot

# TOKEN и USERNAME должны приходить через окружение или --env-file
ENV TOKEN=""
ENV USERNAME=""
ENV TZ="Europe/Moscow"

CMD ["/app/bot"]

