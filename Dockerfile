FROM golang:1.19 as builder

# Create and change to the app directory.
WORKDIR /app

# Initial basic step to verify copying files works
COPY go.mod .
COPY go.sum .

# Download dependencies
RUN go mod download

# Copy the rest of the Go code
COPY *.go ./

# Explicitly download dependencies to the vendor directory.
RUN CGO_ENABLED=0 GOOS=linux go mod vendor

# Build the binary.
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -v -o server


FROM alpine:3.15
RUN apk add --no-cache ca-certificates

# Copy the binary to the production image from the builder stage.
COPY --from=builder /app/server /server

# Ensure the server is executable
RUN chmod +x /server

# Run the web service on container startup.
EXPOSE 5000
CMD ["./server"]