FROM golang:alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Install gcc and other build dependencies for CGO (required by go-sqlite3 used in whatsmeow)
RUN apk add --no-cache gcc g++ musl-dev

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o main .

# Start a new stage from scratch
FROM alpine:latest  

# Install certificates and tzdata
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/main .

# Optimize Go Garbage Collection for Container (Limit 250 MB)
ENV GOMEMLIMIT=250MiB

# Expose port 8088 to the outside world
EXPOSE 8088

# Command to run the executable
CMD ["./main"]
