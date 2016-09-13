# See: https://github.com/gliderlabs/docker-alpine
FROM gliderlabs/alpine:3.4

RUN apk add --update --no-cache \
    python \
    py-pip && \
    pip install awscli elasticsearch-curator==3.5.1

ENTRYPOINT ["/bin/sh"]
