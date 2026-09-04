# syntax=docker/dockerfile:1

# Stage 1: build the React + TypeScript dashboard (ui/app) into static assets. Kept as its own
# stage (rather than committing ui/dist to git) so the built JS/CSS can never silently drift
# from the source in ui/app/src — every image build regenerates it from scratch.
FROM node:22-alpine AS ui-build
WORKDIR /src/ui/app
COPY ui/app/package.json ui/app/package-lock.json ui/app/.npmrc ./
# .npmrc here matters: this machine's/CI's global npm config can default to omit=dev, which
# would silently skip installing vite/typescript and fail the build with a confusing "tsc: not
# found" rather than an obvious missing-dependency error — see ui/app/.npmrc's own comment.
RUN npm ci
COPY ui/app/ ./
RUN npm run build

# Stage 2: compile the Go binary, embedding the UI assets built above.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-build /src/ui/dist ./ui/dist
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/zonetax-collector ./cmd/collector

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/zonetax-collector /zonetax-collector
USER 65532:65532
ENTRYPOINT ["/zonetax-collector"]
