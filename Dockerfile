FROM quay.io/hellofresh/hf-golangimage

# - Install pip
# - Install Elasticsearch Curator [1]
# - Cleanup APT
# - Install go-sqsd [2]
#     [1] https://www.elastic.co/guide/en/elasticsearch/client/curator/current/index.html
#     [2] https://github.com/hellofresh/go-sqsd
RUN apt-get update && apt-get install -y python-pip && \
    pip install elasticsearch-curator && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* && \
    go get github.com/hellofresh/go-sqsd/cli/sqsd


# Use baseimage-docker's init system.
# Reference: https://github.com/phusion/baseimage-docker#using-baseimage-docker-as-base-image
# ***  NOTE  ***
# This requires an *executable* 'run' script which has to be copied to '/etc/service/<YOUR-SERVICE>/run'
# At container startup time this 'run' script will be picked up by the runit supervise service which is
# already part of phusion/baseimage-docker.

CMD ["/sbin/my_init"]


COPY logpruner /logpruner

COPY docker-resources/supervise/go-sqsd/go-sqsd_service.sh /etc/service/go-sqsd/run
COPY docker-resources/supervise/logpruner/logpruner_service.sh /etc/service/logpruner/run