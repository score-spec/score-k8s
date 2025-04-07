FROM golang:1.24.2-alpine@sha256:7772cb5322baa875edd74705556d08f0eeca7b9c4b5367754ce3f2f00041ccee AS builder

ARG VERSION=0.0.0

# Set the current working directory inside the container.
WORKDIR /go/src/github.com/score-spec/score-k8s

# Copy just the module bits
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project and build it.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X github.com/score-spec/score-k8s/internal/version.Version=${VERSION}" -o /usr/local/bin/score-k8s ./cmd/score-k8s

# We can use gcr.io/distroless/static since we don't rely on any linux libs or state, but we need ca-certificates to connect to https/oci with the init command.
FROM gcr.io/distroless/static:530158861eebdbbf149f7e7e67bfe45eb433a35c@sha256:5c7e2b465ac6a2a4e5f4f7f722ce43b147dabe87cb21ac6c4007ae5178a1fa58

# Set the current working directory inside the container.
WORKDIR /score-k8s

# Copy the binary from the builder image.
COPY --from=builder /usr/local/bin/score-k8s /usr/local/bin/score-k8s

# Run the binary.
ENTRYPOINT ["/usr/local/bin/score-k8s"]
