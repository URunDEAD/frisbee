// Licensed to FORTH/ICS under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. FORTH/ICS licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package workflow

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	pet "github.com/dustinkirkland/golang-petname"
	"github.com/fnikolai/frisbee/api/v1alpha1"
	"github.com/fnikolai/frisbee/controllers/template/helpers"
	"github.com/fnikolai/frisbee/controllers/utils"
	"github.com/fnikolai/frisbee/controllers/utils/grafana"
	"github.com/fnikolai/frisbee/controllers/utils/lifecycle"
	"github.com/fnikolai/frisbee/pkg/netutils"
	notifier "github.com/golanghelper/grafana-webhook"
	"github.com/grafana-tools/sdk"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// {{{ Internal types

const (
	// grafana specific.
	grafanaDashboards  = "/etc/grafana/provisioning/dashboards"
	prometheusTemplate = "observability/prometheus"
	grafanaTemplate    = "observability/grafana"
)

func (r *Controller) newMonitoringStack(ctx context.Context, obj *v1alpha1.Workflow) error {
	if len(obj.Spec.ImportMonitors) == 0 {
		return nil
	}

	prometheus, err := r.installPrometheus(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "prometheus error")
	}

	grafana, err := r.installGrafana(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "grafana error")
	}

	// Make Prometheus and Grafana accessible from outside the ByCluster
	if obj.Spec.Ingress != nil {
		if err := r.installIngress(ctx, obj, prometheus, grafana); err != nil {
			return errors.Wrapf(err, "ingress error")
		}

		r.Logger.Info("Ingress is installed")

		// use the public Grafana address (via Ingress) because the controller runs outside the cluster
		grafanaPublicURI := fmt.Sprintf("http://%s", virtualhost(grafana.GetName(), obj.Spec.Ingress.Host))

		if err := r.initGrafana(ctx, grafanaPublicURI); err != nil {
			return errors.Wrapf(err, "grafana client error")
		}
	}

	r.Logger.Info("Monitoring stack is ready")

	return nil
}

func (r *Controller) installPrometheus(ctx context.Context, w *v1alpha1.Workflow) (*v1alpha1.Service, error) {
	var prom v1alpha1.Service

	{ // metadata
		utils.SetOwner(r, w, &prom)
		prom.SetName("prometheus")
	}

	{ // spec
		ts := thelpers.ParseRef(w.GetNamespace(), prometheusTemplate)

		genSpec, err := thelpers.GetDefaultSpec(ctx, r, ts)
		if err != nil {
			return nil, errors.Wrapf(err, "scheme retrieval")
		}

		spec, err := genSpec.ToServiceSpec()
		if err != nil {
			return nil, errors.Wrapf(err, "scheme decoding")
		}

		spec.DeepCopyInto(&prom.Spec)
	}

	{ // deployment
		err := r.GetClient().Create(ctx, &prom)

		switch {
		case k8errors.IsAlreadyExists(err):
			return nil, errors.Wrapf(err, "a previous prometheus instance is running")

		case err != nil:
			return nil, errors.Wrapf(err, "creation failed")
		default:
			logrus.Warnf("Waiting for prometheus to become ready ...")
			if err := lifecycle.WaitUntil(r.prometheus, v1alpha1.PhaseRunning); err != nil {
				return nil, errors.Wrapf(err, "prometheus is not running")
			}
			close(r.prometheus)
		}
	}
	r.Logger.Info("Prometheus is installed")

	return &prom, nil
}

func (r *Controller) installGrafana(ctx context.Context, w *v1alpha1.Workflow) (*v1alpha1.Service, error) {
	grafana := v1alpha1.Service{}

	{ // metadata
		utils.SetOwner(r, w, &grafana)
		grafana.SetName("grafana")
	}

	{ // spec
		// to perform the necessary automations, we load the spec locally and push the modified version for creation.
		ts := thelpers.ParseRef(w.GetNamespace(), grafanaTemplate)

		genSpec, err := thelpers.GetDefaultSpec(ctx, r, ts)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get scheme")
		}

		spec, err := genSpec.ToServiceSpec()
		if err != nil {
			return nil, errors.Wrapf(err, "spec failed")
		}

		if err := r.importDashboards(ctx, w, &spec); err != nil {
			return nil, errors.Wrapf(err, "import dashboards")
		}

		spec.DeepCopyInto(&grafana.Spec)
	}

	{ // deployment
		err := r.GetClient().Create(ctx, &grafana)

		switch {
		case k8errors.IsAlreadyExists(err):
			return nil, errors.Wrapf(err, "a previous grafana instance is running")

		case err != nil:
			return nil, errors.Wrapf(err, "creation failed")
		default:
			logrus.Warnf("Waiting for grafana to become ready ...")

			if err := lifecycle.WaitUntil(r.grafana, v1alpha1.PhaseRunning); err != nil {
				return nil, errors.Wrapf(err, "grafana is not running")
			}

			close(r.grafana)
		}
	}

	r.Logger.Info("Grafana is installed")

	return &grafana, nil
}

func (r *Controller) importDashboards(ctx context.Context, obj *v1alpha1.Workflow, spec *v1alpha1.ServiceSpec) error {
	// iterate monitoring services
	for _, monRef := range obj.Spec.ImportMonitors {
		ts := thelpers.ParseRef(obj.GetNamespace(), monRef)

		genSpec, err := thelpers.GetDefaultSpec(ctx, r, ts)
		if err != nil {
			return errors.Wrapf(err, "cannot get scheme for %s", monRef)
		}

		monSpec, err := genSpec.ToMonitorSpec()
		if err != nil {
			return errors.Wrapf(err, "spec error for %s", monRef)
		}

		// get the configmap which contains our desired dashboard
		configMapKey := client.ObjectKey{Namespace: obj.GetNamespace(), Name: monSpec.Dashboard.FromConfigMap}
		configMap := corev1.ConfigMap{}

		if err := r.GetClient().Get(ctx, configMapKey, &configMap); err != nil {
			return errors.Wrapf(err, "cannot get configmap %s", configMapKey)
		}

		// create volume from the configmap
		volume := corev1.Volume{}
		volume.Name = pet.Name() // generate random name

		volume.VolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMap.GetName()},
			},
		}

		// Create mountpoints
		mounts := make([]corev1.VolumeMount, 0, len(configMap.Data))

		for file := range configMap.Data {
			if file == monSpec.Dashboard.File {
				mounts = append(mounts, corev1.VolumeMount{
					Name:      volume.Name, // Name of a Volume.
					ReadOnly:  true,
					MountPath: filepath.Join(grafanaDashboards, file), // Path within the container
					SubPath:   file,                                   //  Path within the volume
				})
			}
		}

		// associate mounts to grafana container
		spec.Volumes = append(spec.Volumes, volume)
		spec.Container.VolumeMounts = append(spec.Container.VolumeMounts, mounts...)
	}

	return nil
}

func (r *Controller) installIngress(ctx context.Context, obj *v1alpha1.Workflow, services ...*v1alpha1.Service) error {
	ingress := netv1.Ingress{}

	{ // metadata
		utils.SetOwner(r, obj, &ingress)
		ingress.SetName("frisbee")

		if obj.Spec.Ingress.UseAmbassador {
			ingress.SetAnnotations(map[string]string{
				"kubernetes.io/ingress.class": "ambassador",
			})
		}
	}

	{ // spec
		pathtype := netv1.PathTypePrefix

		rules := make([]netv1.IngressRule, 0, len(services))

		for _, service := range services {
			// we now that prometheus and grafana have a single container
			port := service.Spec.Container.Ports[0]

			rule := netv1.IngressRule{
				Host: virtualhost(service.Name, obj.Spec.Ingress.Host),
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &pathtype,
								Backend: netv1.IngressBackend{
									Service: &netv1.IngressServiceBackend{
										Name: service.Name,
										Port: netv1.ServiceBackendPort{Number: port.ContainerPort},
									},
								},
							},
						},
					},
				},
			}

			rules = append(rules, rule)

			r.Logger.Info("Ingress", "host", rule.Host)
		}

		ingress.Spec.Rules = rules
	}

	{ // deployment
		if err := utils.Create(ctx, r, &ingress); err != nil {
			return errors.Wrapf(err, "unable to create ingress")
		}
	}

	return nil
}

func virtualhost(serviceName, ingress string) string {
	return fmt.Sprintf("%s.%s", serviceName, ingress)
}

func (r *Controller) initGrafana(ctx context.Context, apiURI string) error {
	var healthCheckTimeout = wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   5,
		Jitter:   0.1,
		Steps:    4,
	}

	grafanaClient, err := sdk.NewClient(apiURI, "", sdk.DefaultHTTPClient)
	if err != nil {
		return errors.Wrapf(err, "grafanaClient error")
	}

	// retry until Grafana is ready to receive annotations.
	err = retry.OnError(healthCheckTimeout, func(_ error) bool { return true }, func() error {
		_, err := grafanaClient.GetHealth(ctx)

		return errors.Wrapf(err, "grafana health error")
	})

	if err != nil {
		return errors.Wrapf(err, "grafana is unreachable")
	}

	url, err := r.runNotificationWebhook(ctx, "6666")
	if err != nil {
		return errors.Wrapf(err, "cannot run a notification webhook")
	}

	r.Logger.Info("Grafana webhook is listening on", "url", url)

	// create a feedback alert notification channel
	feedback := sdk.AlertNotification{
		Name:                  "to-frisbee-controller",
		Type:                  "webhook",
		IsDefault:             true,
		DisableResolveMessage: true,
		SendReminder:          false,
		Settings: map[string]string{
			"url": url,
		},
	}

	if _, err := grafanaClient.CreateAlertNotification(ctx, feedback); err != nil {
		return errors.Wrapf(err, "cannot create feedback notification channel")
	}

	grafana.SetAnnotator(ctx, grafanaClient)

	return nil
}

func (r *Controller) runNotificationWebhook(ctx context.Context, port string) (string, error) {
	// get local ip
	ip, err := netutils.GetPublicIP()
	if err != nil {
		return "", errors.Wrapf(err, "cannot get controller's public ip")
	}

	handler := http.DefaultServeMux
	handler.HandleFunc("/", notifier.HandleWebhook(func(w http.ResponseWriter, b *notifier.Body) {

		r.Info("Grafana Alert",
			"title", b.Title,
			"message", b.Message,
			"matches", b.EvalMatches,
			"state", b.State,
		)
	}, 0))

	addr := fmt.Sprintf("%s:%s", ip.String(), port)

	errCh := make(chan error, 1)

	go func() {
		errCh <- http.ListenAndServe(addr, handler)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return "", errors.Wrapf(err, "webhook server failed")
		}
	case <-ctx.Done():
		return "", errors.Wrapf(ctx.Err(), "webhook server failed")
	default:
		url := fmt.Sprintf("http://%s", addr)

		return url, nil
	}

	panic("should never happen")
}
