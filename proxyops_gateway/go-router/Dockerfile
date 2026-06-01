FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /go-router .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates && addgroup -S appgroup && adduser -S appuser -G appgroup
COPY --from=builder /go-router /usr/local/bin/
EXPOSE 8080
USER appuser
CMD ["go-router"]
