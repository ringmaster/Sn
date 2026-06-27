FROM --platform=$BUILDPLATFORM golang:alpine AS builder

ARG TARGETOS
ARG TARGETARCH

ENV GONOPROXY="none" \
    CGO_ENABLED=0 \
    GOFLAGS="-mod=readonly -trimpath -modcacherw"

# git is needed for go modules, ca-certificates for system CAs.
RUN apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /src

RUN mkdir -p /output

# --- Create Non-Root User ---
RUN addgroup --gid "1337" app && \
    adduser --disabled-password --gecos "" --no-create-home --uid 1337 --ingroup app --shell /bin/false app && \
    cat /etc/passwd | grep app > "/etc/passwd_app" && \
    cat /etc/group | grep app > "/etc/group_app"

COPY go.mod go.sum /src/
RUN go mod download

# === CACHE BREAKS HERE ON CODE CHANGE ===
# --- Copy Source Code ---
COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -tags timetzdata -buildvcs=false \
    -ldflags "-s -w" \
    -o /output/sn

FROM scratch

# Copy minimal user and group files for the non-root user
# This is necessary because scratch has no users defined
COPY --from=builder /etc/passwd_app /etc/passwd
COPY --from=builder /etc/group_app /etc/group

## /server executable
COPY --from=builder --chown=1337:1337 /output/ /app/
COPY --from=builder --chown=1337:1337 /etc/ssl/certs/ca-certificates.crt /app/certs/

# Set SSL cert environment variables
ENV SSL_CERT_DIR="/app/certs" \
    SSL_CERT_FILE="/app/certs/ca-certificates.crt"

WORKDIR /app

# --- Metadata & Security ---
LABEL security.read-only-root-filesystem="true" \
      security.non-root-user="1337" \
      security.capabilities.drop="ALL"

# Create and switch to non-root user
USER 1337:1337

# Expose port (documentation only - doesn't actually open ports)
EXPOSE 8080

# Use exec form to avoid shell and ensure proper signal handling
ENTRYPOINT ["/app/sn"]
