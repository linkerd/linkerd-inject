# Helloworld example

Example deployment using Daemonset linkerd and transparent proxying using our
[hello world example](https://github.com/linkerd/linkerd-examples/tree/master/docker/helloworld).
For [minikube](https://github.com/kubernetes/minikube) instructions, see below.

You'll notice that unlike our previous
[hello world](https://github.com/linkerd/linkerd-examples/blob/master/k8s-daemonset/k8s/hello-world.yml) or
[hello world legacy](https://github.com/linkerd/linkerd-examples/blob/master/k8s-daemonset/k8s/hello-world-legacy.yml),
we don't have `http_proxy` set. Also note that the service name has been changed
from `world-v1` to `world`.

(Note that the inject example assumes an incoming linkerd router on port `4140` and
that the Daemonset has a service name of `l5d`. If you've changed these, please
use the `-linkerdPort` and `-linkerdSvcName` flags.)

Deploy linkerd as a Daemonset:
```
$ kubectl apply -f https://raw.githubusercontent.com/linkerd/linkerd-examples/master/k8s-daemonset/k8s/linkerd.yml
```

Use linkerd-inject to modify the hello world config, and deploy it:
```
$ LINKERD_PORT=4140
$ kubectl apply -f <(inject -f hello-world.yml -linkerdPort $LINKERD_PORT)
```

Test it out! Our services now talk to each other using linkerd.
```
$ INGRESS_LB=$(kubectl get svc l5d -o jsonpath="{.status.loadBalancer.ingress[0].*}")
$ curl $INGRESS_LB:$LINKERD_PORT -H "Host: hello"
Hello (10.196.2.94) world (10.196.0.26)!!
```

## Running in minikube

Deploy the linkerd Daemonset as above, and then use the following commands.

```
$ LINKERD_PORT=4140
$ kubectl apply -f <(inject -f hello-world.yml -useServiceVip -linkerdPort $LINKERD_PORT)

$ L5D_NODE_PORT=$(kubectl get svc l5d -o jsonpath="{.spec.ports[0].nodePort}")
$ curl -v $(minikube ip):$L5D_NODE_PORT -H "Host: hello"
```
