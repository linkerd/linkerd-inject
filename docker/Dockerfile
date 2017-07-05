# Based on https://github.com/istio/pilot/blob/pilot-0-2-0-working/docker/debug/Dockerfile
# Build linkerd inject image
#
#  docker build -t linkerd/istio-init:v1 .
#  docker push linkerd/istio-init:v1

FROM ubuntu:xenial
RUN apt-get update && \
      apt-get install -y \
      curl \
      iptables

ADD prepare_proxy.sh /usr/local/bin/prepare_proxy.sh
RUN chmod +x /usr/local/bin/prepare_proxy.sh

ENTRYPOINT ["/usr/local/bin/prepare_proxy.sh"]
