/*
Copyright 2021 ICS-FORTH.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"github.com/carv-ics-forth/frisbee/controllers/common"
	serviceutils "github.com/carv-ics-forth/frisbee/controllers/service/utils"
	"github.com/carv-ics-forth/frisbee/pkg/configuration"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
)

func (r *Controller) runJob(ctx context.Context, service *v1alpha1.Service) error {
	setDefaultValues(service)

	if err := handleRequirements(ctx, r, service); err != nil {
		return errors.Wrapf(err, "cannot satisfy requirements")
	}

	if err := decoratePod(ctx, r, service); err != nil {
		return errors.Wrapf(err, "cannot set pod decorators")
	}

	discovery, err := constructDiscoveryService(service)
	if err != nil {
		return errors.Wrapf(err, "cannot build DNS service")
	}

	if err := common.Create(ctx, r, service, discovery); err != nil {
		return errors.Wrapf(err, "cannot create DNS service")
	}

	// finally, create the pod
	var pod corev1.Pod

	pod.SetName(service.GetName())
	v1alpha1.PropagateLabels(&pod, service)
	pod.SetAnnotations(service.GetAnnotations())

	service.Spec.PodSpec.DeepCopyInto(&pod.Spec)

	if err := common.Create(ctx, r, service, &pod); err != nil {
		return errors.Wrapf(err, "cannot create pod")
	}

	return nil
}

func setDefaultValues(cr *v1alpha1.Service) {
	// Set the restart policy
	cr.Spec.RestartPolicy = corev1.RestartPolicyNever

	// Set the pre/post execution hooks
	for i := 0; i < len(cr.Spec.Containers); i++ {
		// Use this for the telemetry sidecar to be able to enter the cgroup of the main container
		/*
			cr.Spec.Containers[i].Lifecycle = &corev1.Lifecycle{
				PostStart: &corev1.Handler{
					Exec: &corev1.ExecAction{
						Command: []string{
							// "/bin/sh", "-c", "|", "cut -d ' ' -f 4 /proc/self/stats > /dev/shm/app",
						},
					},
				},
				PreStop: nil,
			}

		*/
	}
}

var pathType = netv1.PathTypePrefix

func handleRequirements(ctx context.Context, r *Controller, cr *v1alpha1.Service) error {
	if cr.Spec.Requirements == nil {
		return nil
	}

	// Ingress
	if req := cr.Spec.Requirements.Ingress; req != nil {
		var ingress netv1.Ingress

		ingressClassName := configuration.Global.IngressClassName

		ingress.SetName(cr.GetName())
		v1alpha1.PropagateLabels(&ingress, cr)

		ingress.Spec = netv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []netv1.IngressRule{
				{
					Host: common.ExternalEndpoint(cr.GetName(), cr.GetNamespace()),
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: cr.GetName(),
											Port: req.Service.Port,
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if err := common.Create(ctx, r, cr, &ingress); err != nil {
			return errors.Wrapf(err, "cannot create ingress")
		}
	}

	return nil
}

func SetField(service *v1alpha1.Service, val v1alpha1.SetField) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("cannot set field [%s]. err: %s", val.Field, r)
		}
	}()

	fieldRef := reflect.ValueOf(&service.Spec).Elem()

	index := func(path reflect.Value, idx string) reflect.Value {
		if i, err := strconv.Atoi(idx); err == nil {
			return path.Index(i)
		}

		// reflect.Value.FieldByName cannot be used on map Value
		if path.Kind() == reflect.Map {
			return reflect.Indirect(path)
		}

		return reflect.Indirect(path).FieldByName(idx)
	}

	for _, s := range strings.Split(val.Field, ".") {
		fieldRef = index(fieldRef, s)
	}

	var conv interface{}

	// Convert src value to something that may fit to the dst.
	switch fieldRef.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		toInt, err := strconv.Atoi(val.Value)
		if err != nil {
			return errors.Wrapf(err, "convert to Int error")
		}

		conv = toInt

	case reflect.Bool:
		toBool, err := strconv.ParseBool(val.Value)
		if err != nil {
			return errors.Wrapf(err, "convert to Bool error")
		}

		conv = toBool

	case reflect.Map:
		// TODO: Needs to be improved because the map can be of various types
		logrus.Warn("THIS FUNCTION IS NOT WORKING, BUT WE DO NOT WANT TO FAIL EITHER")

		return nil

	default:
		conv = val.Value
	}

	fieldRef.Set(reflect.ValueOf(conv).Convert(fieldRef.Type()))

	return nil
}

func decoratePod(ctx context.Context, r *Controller, cr *v1alpha1.Service) error {
	// set labels
	if req := cr.Spec.Decorators.Labels; req != nil {
		cr.SetLabels(labels.Merge(cr.GetLabels(), req))
	}

	// set annotations
	if req := cr.Spec.Decorators.Annotations; req != nil {
		cr.SetAnnotations(labels.Merge(cr.GetAnnotations(), req))
	}

	// set dynamically evaluated fields
	if req := cr.Spec.Decorators.SetFields; req != nil {
		for _, val := range req {
			if err := SetField(cr, val); err != nil {
				return errors.Wrapf(err, "cannot set field [%v]", val)
			}
		}
	}

	// set resources, to the first container only
	if req := cr.Spec.Decorators.Resources; req != nil {
		if len(cr.Spec.Containers) != 1 {
			return errors.New("Decoration resources are not applicable for multiple containers")
		}

		resources := make(map[corev1.ResourceName]resource.Quantity)

		var resourceError *multierror.Error

		if len(req.CPU) > 0 {
			q, err := resource.ParseQuantity(req.CPU)
			if err != nil {
				resourceError = multierror.Append(resourceError, errors.Wrapf(err, "CPU resource error"))
			}

			resources[corev1.ResourceCPU] = q
		}

		if len(req.Memory) > 0 {
			q, err := resource.ParseQuantity(req.Memory)
			if err != nil {
				resourceError = multierror.Append(resourceError, errors.Wrapf(err, "Memory resource error"))
			}

			resources[corev1.ResourceMemory] = q
		}

		if resourceError != nil {
			return errors.Wrapf(resourceError, "Resource error")
		}

		cr.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Limits:   resources,
			Requests: resources,
		}
	}

	if len(cr.Spec.Decorators.Telemetry) > 0 {
		//  The sidecar makes use of the shareProcessNamespace option to access the host cgroup metrics.
		share := true
		cr.Spec.ShareProcessNamespace = &share
	}

	// import telemetry agents
	if req := cr.Spec.Decorators.Telemetry; req != nil {
		// import dashboards for monitoring agents to the service
		for _, monRef := range req {
			monSpec, err := serviceutils.GetServiceSpec(ctx, r.GetClient(), cr, v1alpha1.GenerateObjectFromTemplate{TemplateRef: monRef})
			if err != nil {
				return errors.Wrapf(err, "cannot get monitor")
			}

			if len(monSpec.Containers) != 1 {
				return errors.Wrapf(err, "telemetry sidecar '%s' expected 1 container but got %d",
					monRef, len(monSpec.Containers))
			}

			cr.Spec.Containers = append(cr.Spec.Containers, monSpec.Containers[0])
			cr.Spec.Volumes = append(cr.Spec.Volumes, monSpec.Volumes...)
			cr.Spec.Volumes = append(cr.Spec.Volumes, monSpec.Volumes...)
		}
	}

	return nil
}

func constructDiscoveryService(service *v1alpha1.Service) (*corev1.Service, error) {
	// register ports from containers and sidecars
	var allPorts []corev1.ServicePort

	for ci, container := range service.Spec.Containers {
		for pi, port := range container.Ports {
			if port.ContainerPort == 0 {
				return nil, errors.Errorf("port is 0 for container[%d].port[%d]", ci, pi)
			}

			allPorts = append(allPorts, corev1.ServicePort{
				Name: port.Name,
				Port: port.ContainerPort,
			})
		}
	}

	// clusterIP should be specified only with ports
	var clusterIP string

	if len(allPorts) == 0 {
		clusterIP = "None"
	}

	var kubeService corev1.Service

	kubeService.SetName(service.GetName())

	// make labels visible to the dns service
	v1alpha1.PropagateLabels(&kubeService, service)

	kubeService.Spec.Ports = allPorts
	kubeService.Spec.ClusterIP = clusterIP

	// select pods that are created by the same v1alpha1.Service as this corev1.Service
	kubeService.Spec.Selector = map[string]string{
		v1alpha1.LabelCreatedBy: service.GetName(),
	}

	return &kubeService, nil
}
