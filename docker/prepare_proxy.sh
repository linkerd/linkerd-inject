#!/bin/bash
# Linkerd initialization script responsible for setting up port forwarding.
# Based on: https://github.com/istio/pilot/blob/pilot-0-2-0-working/docker/prepare_proxy.sh

set -o errexit
set -o nounset
set -o pipefail

usage() {
  echo "${0} -p PORT"
  echo ''
  echo '  -p: Specify the linkerd daemonset port to which redirect all TCP traffic'
  echo ''
}

while getopts ":p:m:" opt; do
  case ${opt} in
    p)
      LINKERD_PORT=${OPTARG}
      ;;
    m)
      RUN_IN_MINIKUBE=${OPTARG}
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${LINKERD_PORT-}" ]]; then
  echo "Please set linkerd port -p"
  usage
  exit 1
fi

# Don't forward local traffic
iptables -t nat -A OUTPUT -d 127.0.0.1/32 -j RETURN                                         -m comment --comment "istio/bypass-explicit-loopback"

if [ "$RUN_IN_MINIKUBE" = true ]; then
  # Forward traffic to the linkerd service VIP on the single minikube node
  iptables -t nat -A OUTPUT -p tcp -j DNAT --to ${L5D_SERVICE_HOST}:${LINKERD_PORT}         -m comment --comment "istio/dnat-to-minikube-l5d"
else
  # iptables doesn't like hostnames with dashes, resolve the host ip here
  # in kubernetes 1.7, can get hostIp via downward api
  HOST_IP=$(getent hosts $NODE_NAME | awk '{ print $1 }')

  # Forward traffic to the daemonset linkerd router
  iptables -t nat -A OUTPUT -p tcp -j DNAT --to ${HOST_IP}:${LINKERD_PORT}                  -m comment --comment "istio/dnat-to-daemonset-l5d"
 fi

# list iptables rules
iptables -t nat --list


