ARG GOLANG_VERSION="1.21.4"

FROM golang:${GOLANG_VERSION}-alpine as builder
ARG LDFLAGS
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /go/src/github.com/z0rr0/spts
COPY . .
RUN echo "LDFLAGS = $LDFLAGS"
RUN GOOS=linux go build -ldflags "$LDFLAGS" -o ./spts

FROM alpine:3.18
LABEL org.opencontainers.image.authors="me@axv.email" \
        org.opencontainers.image.url="https://hub.docker.com/r/z0rr0/spts" \
        org.opencontainers.image.documentation="https://github.com/z0rr0/spts" \
        org.opencontainers.image.source="https://github.com/z0rr0/spts" \
        org.opencontainers.image.licenses="MIT" \
        org.opencontainers.image.title="SPTS" \
        org.opencontainers.image.description="Speed Test Service"

COPY --from=builder /go/src/github.com/z0rr0/spts/spts /bin/
RUN chmod 0755 /bin/spts

ENTRYPOINT ["/bin/spts -server"]
