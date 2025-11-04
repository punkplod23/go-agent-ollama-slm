# Use the official Golang image to build the application
FROM golang:1.20.7-alpine3.18 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the Go modules manifests
COPY go.mod go.sum ./

# Download Go modules
RUN go mod download

# Copy the source code
COPY . .

# Build the Go application
RUN go build -o main ./cmd/app

# Use a minimal base image for the final container
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/main .

# Expose the port the application runs on
EXPOSE 8080
# Set environment variables (can be overridden at runtime)
ENV APP_ENV=production \
    OPENWEBUIHOSTURL="" \
    OPENWEBUIAPITOKEN="" \
    OPENWEBUIMODELNAME="" \
    DVSAAPIURL="" \
    OPENALPRAPIURL="" 

# Run the application

# Command to run the application
CMD ["./main"]