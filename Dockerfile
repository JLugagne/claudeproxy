# The console is built first and copied into the Go embed directory, so the
# image always ships the dashboard built from this source tree.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /internal/proxy/inbound/dashboard/dist ./internal/proxy/inbound/dashboard/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/claudeproxy .

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /out/claudeproxy /claudeproxy
COPY configs/config.yaml /etc/claudeproxy/config.yaml
ENV CLAUDEPROXY_CONFIG=/etc/claudeproxy/config.yaml
EXPOSE 8787
ENTRYPOINT ["/claudeproxy"]
CMD ["serve", "--listen", "0.0.0.0:8787", "--allow-non-loopback"]
