FROM golang:1.25-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the Go app statically
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o go-bittorrent main.go

# Start a new stage from scratch using a minimal alpine image
FROM alpine:latest  

# Add ca-certificates in case the client needs to connect to HTTPS trackers
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/go-bittorrent .

# Create a directory to mount volumes for reading .torrent files and writing downloads
RUN mkdir /data

# Run the executable
ENTRYPOINT ["./go-bittorrent"]
