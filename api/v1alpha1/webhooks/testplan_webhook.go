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

package webhooks

import (
	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var testplanlog = logf.Log.WithName("testplan-resource")

func (r *v1alpha1.TestPlan) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-testplan,mutating=true,failurePolicy=fail,sideEffects=None,groups=frisbee.io,resources=testplans,verbs=create;update,versions=v1alpha1,name=mtestplan.kb.io,admissionReviewVersions={v1,v1alpha1}

var _ webhook.Defaulter = &v1alpha1.TestPlan{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *v1alpha1.TestPlan) Default() {
	testplanlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-testplan,mutating=false,failurePolicy=fail,sideEffects=None,groups=frisbee.io,resources=testplans,verbs=create;update,versions=v1alpha1,name=vtestplan.kb.io,admissionReviewVersions={v1,v1alpha1}

var _ webhook.Validator = &v1alpha1.TestPlan{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *v1alpha1.TestPlan) ValidateCreate() error {
	testplanlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *v1alpha1.TestPlan) ValidateUpdate(old runtime.Object) error {
	testplanlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *v1alpha1.TestPlan) ValidateDelete() error {
	testplanlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}