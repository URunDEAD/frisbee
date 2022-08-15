/*
Copyright 2022 ICS-FORTH.

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

package client

import (
	"context"
	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"github.com/carv-ics-forth/frisbee/pkg/manifest"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/yaml"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

var (
	ManagedNamespace = map[string]string{"app.kubernetes.io/managed-by": "Frisbee"}
	yamlSeparator    = regexp.MustCompile(`\n---`)
)

// NewTestManagementClient creates new Test client
func NewTestManagementClient(client client.Client, options Options) TestManagementClient {
	return TestManagementClient{
		client:  client,
		options: options,
	}
}

type TestManagementClient struct {
	client  client.Client
	options Options
}

// GetTest returns single test by id
func (c TestManagementClient) GetTest(id string) (scenario *v1alpha1.Scenario, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filters := &client.ListOptions{
		Namespace: id,
	}

	var scenarios v1alpha1.ScenarioList

	if err := c.client.List(ctx, &scenarios, filters); err != nil {
		return nil, errors.Wrapf(err, "cannot list resources")
	}

	switch {
	case len(scenarios.Items) == 0:
		return nil, nil

	case len(scenarios.Items) != 1:
		return nil, errors.Errorf("test '%s' has %d scenarios", id, len(scenarios.Items))
	}

	return &scenarios.Items[0], nil
}

// ListTests list all tests
func (c TestManagementClient) ListTests(selector string) (tests v1alpha1.ScenarioList, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	set, err := labels.ConvertSelectorToLabelsMap(selector)
	if err != nil {
		return tests, errors.Wrapf(err, "invalid selector")
	}

	// find namespaces where tests are running
	filters := &client.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Merge(ManagedNamespace, set)),
	}

	var namespaces corev1.NamespaceList

	if err := c.client.List(ctx, &namespaces, filters); err != nil {
		return tests, errors.Wrapf(err, "cannot list resource")
	}

	// extract scenarios from the namespaces
	for _, nm := range namespaces.Items {
		var scenarios v1alpha1.ScenarioList

		if err := c.client.List(ctx, &scenarios, &client.ListOptions{Namespace: nm.GetName()}); err != nil {
			return tests, errors.Wrapf(err, "cannot list resources")
		}

		if len(scenarios.Items) != 1 {
			return tests, errors.Errorf("test '%s' has %d scenarios", nm.GetName(), len(scenarios.Items))
		}

		tests.Items = append(tests.Items, scenarios.Items[0])
	}

	return tests, nil
}

// DeleteTests deletes all tests
func (c TestManagementClient) DeleteTests(selector string) (testNames []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	set, err := labels.ConvertSelectorToLabelsMap(selector)
	if err != nil {
		return testNames, errors.Wrapf(err, "invalid selector")
	}

	// find namespaces where tests are running
	filters := &client.ListOptions{
		LabelSelector: labels.SelectorFromValidatedSet(labels.Merge(ManagedNamespace, set)),
	}

	var namespaces corev1.NamespaceList

	if err := c.client.List(ctx, &namespaces, filters); err != nil {
		return testNames, errors.Wrapf(err, "cannot list resource")
	}

	// remove namespaces
	propagation := metav1.DeletePropagationForeground
	// propagation := metav1.DeletePropagationBackground

	for _, nm := range namespaces.Items {
		if err := c.client.Delete(ctx, &nm, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return testNames, errors.Wrapf(err, "cannot remove namespace '%s", nm.GetName())
		}

		testNames = append(testNames, nm.GetName())
	}

	return testNames, nil
}

// DeleteTest deletes single test by name
func (c TestManagementClient) DeleteTest(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if id == "" {
		return errors.Errorf("test id '%s' is not valid", id)
	}

	propagation := metav1.DeletePropagationForeground
	// propagation := metav1.DeletePropagationBackground

	var namespace corev1.Namespace
	namespace.SetName(id)
	namespace.SetLabels(ManagedNamespace)

	return c.client.Delete(ctx, &namespace, &client.DeleteOptions{PropagationPolicy: &propagation})
}

// SubmitTestFromFile applies the scenario from the given file.
func (c TestManagementClient) SubmitTestFromFile(id string, manifestPath string) (resourceNames []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// read the raw content from disk
	fileContents, err := manifest.ReadManifest(manifestPath)
	if err != nil {
		return resourceNames, errors.Wrapf(err, "cannot read manifest '%s'", manifestPath)
	}

	// parse the manifest into resources
	var resources []unstructured.Unstructured

	for i, text := range yamlSeparator.Split(string(fileContents[0]), -1) {
		if strings.TrimSpace(text) == "" {
			continue
		}

		var resource unstructured.Unstructured

		if err := yaml.Unmarshal([]byte(text), &resource); err != nil {
			// Only return an error if this is a kubernetes object, otherwise, print the error
			if resource.GetKind() != "" {
				return resourceNames, errors.Errorf("SKATAKIA ?")
			} else {
				return resourceNames, errors.Errorf("yaml file at index %d is not valid", i)
			}
		}

		resource.SetNamespace(id)
		resources = append(resources, resource)
		resourceNames = append(resourceNames, resource.GetNamespace()+"/"+resource.GetName())
	}

	// create namespace for hosting the scenario
	{
		var namespace corev1.Namespace
		namespace.SetName(id)
		namespace.SetLabels(ManagedNamespace)

		if err := c.client.Create(ctx, &namespace); err != nil {
			return resourceNames, errors.Wrapf(err, "create namespace %s", id)
		}
	}

	// create the resources. if a resource with similar name exists, it is deleted.
	for i, resource := range resources {
		if err := c.client.Delete(ctx, &resources[i]); client.IgnoreNotFound(err) != nil {
			return resourceNames, errors.Wrapf(err, "delete resource %s", resource.GetName())
		}

		if err := c.client.Create(ctx, &resources[i]); err != nil {
			return resourceNames, errors.Wrapf(err, "create resource %s", resource.GetName())
		}
	}

	return resourceNames, nil
}
