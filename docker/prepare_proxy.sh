#!/bin/bash
# Linkerd initialization script responsible for setting up port forwarding.
# Based on: https://github.com/istio/pilot/blob/pilot-0-2-0-working/docker/prepare_proxy.sh

set -o errexit
set -o nounset
set -o pipefail

usage() {
  echo "${0} -p PORT"
  echo ''
  echo '  -p: Specify the linkerd Daemonset port to which redirect all TCP traffic'
  echo '  -m: Run in a single node environment'
  echo '  -s: Specify the linkerd Daemonset service name'
  echo ''
}

while getopts ":p:m:s:" opt; do
  case ${opt} in
    p)
      INJ_LINKERD_PORT=${OPTARG}
      ;;
    m)
      INJ_RUN_IN_MINIKUBE=${OPTARG}
      ;;
    s)
      INJ_L5D_SERVICE_NAME=${OPTARG}
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${INJ_LINKERD_PORT-}" ]]; then
  echo "Please set linkerd port -p"
  usage
  exit 1
fi

# Don't forward local traffic
iptables -t nat -A OUTPUT -d 127.0.0.1/32 -j RETURN                                         -m comment --comment "istio/bypass-explicit-loopback"

if [ "$INJ_RUN_IN_MINIKUBE" = true ]; then
  # Forward traffic to the linkerd service VIP on the single node
  INJ_L5D_SERVICE_VIP="${INJ_L5D_SERVICE_NAME}_SERVICE_HOST"
  iptables -t nat -A OUTPUT -p tcp -j DNAT --to ${!INJ_L5D_SERVICE_VIP}:${INJ_LINKERD_PORT}         -m comment --comment "istio/dnat-to-minikube-l5d"
else
  # iptables doesn't like hostnames with dashes, resolve the host ip here
  # in kubernetes 1.7, can get hostIp via downward api
  INJ_HOST_IP=$(getent hosts $NODE_NAME | awk '{ print $1 }')

  # Forward traffic to the daemonset linkerd router
  iptables -t nat -A OUTPUT -p tcp -j DNAT --to ${INJ_HOST_IP}:${INJ_LINKERD_PORT}                  -m comment --comment "istio/dnat-to-daemonset-l5d"
 fi

# list iptables rules
iptables -t nat --list


