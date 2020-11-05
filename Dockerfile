# Build the manager binary
FROM golang:1.13 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
ENV GOPROXY=https://goproxy.io
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY apis/ apis/
COPY pkg/ pkg/
COPY cmd/oam-kubernetes-runtime/main.go main.go

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o controller main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# oamdev/gcr.io-distroless-static:nonroot is syncd from gcr.io/distroless/static:nonroot as somewhere can't reach gcr.io
FROM oamdev/gcr.io-distroless-static:nonroot
WORKDIR /
COPY --from=builder /workspace/controller .
USER nonroot:nonroot

ENTRYPOINT ["/controller"]
