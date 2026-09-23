# One Dockerfile for every service. SERVICE picks which cmd/ to build, so the
# services cannot drift apart in base image, build flags or security posture.
#
# Build runs on the native arch and cross-compiles to the target, so an arm64
# laptop producing a linux/amd64 image never pays for QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

ARG SERVICE
ARG TARGETOS
ARG TARGETARCH
RUN test -n "${SERVICE}" || (echo "SERVICE build-arg is required" && exit 1) && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8080
# Numeric UID, not the "nonroot" name: kubelet cannot verify a non-numeric user
# against runAsNonRoot and refuses to start the container.
USER 65532:65532
ENTRYPOINT ["/app"]
