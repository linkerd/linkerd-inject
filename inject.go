package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yamlDecoder "k8s.io/apimachinery/pkg/util/yaml"

	// https://github.com/kubernetes/client-go/blob/master/INSTALL.md
	"k8s.io/client-go/pkg/api/v1"
	batch "k8s.io/client-go/pkg/apis/batch/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

/*
	Inject in init-container to set up transparent proxying
	via iptables rules which send traffic to daemonset linkerd

	This file based on:
	https://github.com/istio/pilot/blob/pilot-0-2-0-working/platform/kube/inject/inject.go
	and https://github.com/istio/pilot/blob/pilot-0-2-0-working/cmd/istioctl/inject.go

	TODO: actually get this code into istio/pilot or rewrite it

  This code inserts the following into the pod specs:

	spec:
		initContainers:
		- name: init-linkerd
		  image: linkerd/istio-init:v1
		  env:
		  - name: NODE_NAME
		    valueFrom:
		      fieldRef:
		        fieldPath: spec.nodeName
		  args:
		    - -p
		    - "4140" # port of the Daemonset linkerd's incoming router
		    - -s
		    - "L5D" # linkerd Daemonset service name, uppercased
		    - -m
		    - "false" # set to true if running in minikube
		  imagePullPolicy: IfNotPresent
		  securityContext:
		    capabilities:
		      add:
		      - NET_ADMIN
*/

const (
	istioSidecarAnnotationSidecarKey   = "alpha.istio.io/linkerd-daemonset"
	istioSidecarAnnotationSidecarValue = "injected"
	initContainerName                  = "init-linkerd"
	initImage                          = "linkerd/istio-init:v1"
)

type Params struct {
	LinkerdDaemonsetPort    string
	LinkerdDaemonsetService string
	UseServiceVip           bool
}

func dieIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func injectIntoPodTemplateSpec(p *Params, t *v1.PodTemplateSpec) error {
	// from https://github.com/istio/pilot/blob/pilot-0-2-0-working/platform/kube/inject/inject.go

	if t.Annotations == nil {
		t.Annotations = make(map[string]string)
	} else if _, ok := t.Annotations[istioSidecarAnnotationSidecarKey]; ok {
		// Return unmodified resource if init has already happened
		return nil
	}
	t.Annotations[istioSidecarAnnotationSidecarKey] = istioSidecarAnnotationSidecarValue

	// init-container
	var annotations []interface{}
	if initContainer, ok := t.Annotations["pod.beta.kubernetes.io/init-containers"]; ok {
		if err := json.Unmarshal([]byte(initContainer), &annotations); err != nil {
			return err
		}
	}
	initArgs := []string{
		"-p", p.LinkerdDaemonsetPort,
		"-m", strconv.FormatBool(p.UseServiceVip),
		"-s", strings.ToUpper(p.LinkerdDaemonsetService),
	}

	initContainerAnnotations := map[string]interface{}{
		"name":  initContainerName,
		"image": initImage,
		"args":  initArgs,

		"env": []map[string]interface{}{
			map[string]interface{}{
				"name": "NODE_NAME",
				"valueFrom": map[string]interface{}{
					"fieldRef": map[string]interface{}{
						"apiVersion": "v1", // https://github.com/kubernetes/kubernetes/issues/39189
						"fieldPath":  "spec.nodeName",
					},
				},
			},
		},
		"imagePullPolicy": "IfNotPresent",
		"securityContext": map[string]interface{}{
			"capabilities": map[string]interface{}{
				"add": []string{"NET_ADMIN"},
			},
		},
	}

	annotations = append(annotations, initContainerAnnotations)

	initAnnotationValue, err := json.Marshal(&annotations)
	if err != nil {
		return err
	}

	t.Annotations["pod.beta.kubernetes.io/init-containers"] = string(initAnnotationValue)

	return nil
}

func intoResourceFile(p *Params, in io.Reader, out io.Writer) error {
	// from https://github.com/istio/pilot/blob/pilot-0-2-0-working/platform/kube/inject/inject.go
	reader := yamlDecoder.NewYAMLReader(bufio.NewReaderSize(in, 4096))

	for {
		raw, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		kinds := map[string]struct {
			typ    interface{}
			inject func(typ interface{}) error
		}{
			"Job": {
				typ: &batch.Job{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(p, &((typ.(*batch.Job)).Spec.Template))
				},
			},
			"DaemonSet": {
				typ: &v1beta1.DaemonSet{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(p, &((typ.(*v1beta1.DaemonSet)).Spec.Template))
				},
			},
			"ReplicaSet": {
				typ: &v1beta1.ReplicaSet{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(p, &((typ.(*v1beta1.ReplicaSet)).Spec.Template))
				},
			},
			"Deployment": {
				typ: &v1beta1.Deployment{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(p, &((typ.(*v1beta1.Deployment)).Spec.Template))
				},
			},
			"ReplicationController": {
				typ: &v1.ReplicationController{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(p, ((typ.(*v1.ReplicationController)).Spec.Template))
				},
			},
		}
		var updated []byte
		var meta metav1.TypeMeta
		if err = yaml.Unmarshal(raw, &meta); err != nil {
			return err
		}
		if kind, ok := kinds[meta.Kind]; ok {
			if err = yaml.Unmarshal(raw, kind.typ); err != nil {
				return err
			}
			if err = kind.inject(kind.typ); err != nil {
				return err
			}
			if updated, err = yaml.Marshal(kind.typ); err != nil {
				return err
			}
		} else {
			updated = raw // unchanged
		}

		if _, err = out.Write(updated); err != nil {
			return err
		}
		if _, err = fmt.Fprint(out, "---\n"); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	// from https://github.com/istio/pilot/blob/pilot-0-2-0-working/cmd/istioctl/inject.go
	inputFile := flag.String("f", "", "Input Kubernetes resource filename")
	outputFile := flag.String("o", "", "Modified output Kubernetes resource filename")
	linkerdPort := flag.String("linkerdPort", "4140", "linkerd daemonset port which will handle outgoing requests")
	useServiceVip := flag.Bool("useServiceVip", false, "for use in k8s envs without downward api access")
	linkerdSvcName := flag.String("linkerdSvcName", "l5d", "linkerd daemonset service name")

	flag.Parse()
	var err error

	if *inputFile == "" {
		err = errors.New("Please supply an unmodified Kubernetes resource filename with -f")
	}

	dieIf(err)

	var reader io.Reader
	if *inputFile == "-" {
		reader = os.Stdin
	} else {
		if reader, err = os.Open(*inputFile); err != nil {
			dieIf(err)
		}
	}

	var writer io.Writer
	if *outputFile == "" {
		writer = os.Stdout
	} else {
		var file *os.File
		if file, err = os.Create(*outputFile); err != nil {
			dieIf(err)
		}
		writer = file
		defer func() { err = file.Close() }()
	}

	params := &Params{
		LinkerdDaemonsetPort:    *linkerdPort,
		LinkerdDaemonsetService: *linkerdSvcName,
		UseServiceVip:           *useServiceVip,
	}

	err = intoResourceFile(params, reader, writer)
	dieIf(err)
}
