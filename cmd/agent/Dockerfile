# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/zonetax-agent ./cmd/agent

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/zonetax-agent /zonetax-agent
# The agent needs root to read /proc/net/nf_conntrack via a hostPath mount and to hold
# NET_ADMIN; distroless "nonroot" base still works since capabilities/hostPath access are
# granted by the Pod's securityContext (see Helm chart), not by container UID alone. If host
# nf_conntrack access requires UID 0 in your environment, override with runAsUser: 0.
USER 65532:65532
ENTRYPOINT ["/zonetax-agent"]
