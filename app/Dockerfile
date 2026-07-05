FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o tokensentinel-app .
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/tokensentinel-app /tokensentinel-app
EXPOSE 8080
ENTRYPOINT ["/tokensentinel-app"]
