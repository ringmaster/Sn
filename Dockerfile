FROM golang:alpine AS builder
RUN adduser -D -u 1001 appuser
WORKDIR /app
COPY . .
RUN go mod download && go mod verify
RUN CGO_ENABLED=0 go build -ldflags="-w -s"

#---
FROM scratch
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/ssl /etc/ssl
COPY --from=builder /app/Sn /Sn
USER 1001
EXPOSE 8080
ENTRYPOINT ["/Sn"]
