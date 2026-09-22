# Build runs on the native arch and cross-compiles to the target, so an arm64
# laptop producing a linux/amd64 image never pays for QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/menu .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/menu /menu
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/menu"]
