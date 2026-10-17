FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/claudeproxy .

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /out/claudeproxy /claudeproxy
COPY configs/config.yaml /etc/claudeproxy/config.yaml
ENV CLAUDEPROXY_CONFIG=/etc/claudeproxy/config.yaml
EXPOSE 8787
ENTRYPOINT ["/claudeproxy"]
CMD ["serve", "--listen", "0.0.0.0:8787", "--allow-non-loopback"]
