FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/cost-dashboard .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /app/cost-dashboard .
EXPOSE 3001
USER appuser
CMD ["./cost-dashboard"]
