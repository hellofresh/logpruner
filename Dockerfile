# See: https://github.com/gliderlabs/docker-alpine
FROM gliderlabs/alpine:3.4

#
# Usage:
# - mount 'logpruner_config.yaml' to '/etc/logpruner/logpruner_config.yaml',
# - use '-e' to set following environment variables: AWS_DEFAULT_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY.
# - use -it' and '--rm' options.
#

# Install command line tools used by logpruner utility.
RUN apk add --update --no-cache \
    python \
    py-pip && \
    pip install awscli elasticsearch-curator==3.5.1

# Put logpruner utility into place. Use GitHub release version.
ADD ["https://github.com/hellofresh/logpruner/releases/download/v0.0.3/logpruner", "/usr/local/bin/logpruner"]

# Make sure logpruner is executable.
RUN chmod +x /usr/local/bin/logpruner

# Allow mounting of logpruner_config.yaml file into container.
VOLUME ["/etc/logpruner"]

# It is a cmd-like container, so logpruner utility runs immediately after container startup.
ENTRYPOINT ["/usr/local/bin/logpruner"]
