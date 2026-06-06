FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY cost-dashboard/go.mod cost-dashboard/go.sum ./cost-dashboard/
COPY internal/engine/go.mod ./internal/engine/
WORKDIR /app/cost-dashboard
RUN go mod download
COPY cost-dashboard/ .
COPY internal/engine/ ../internal/engine/
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/cost-dashboard .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /app/cost-dashboard .
EXPOSE 3001
USER appuser
CMD ["./cost-dashboard"]
