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

package common

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// SetOwner is a helper method to make sure the given object contains an object reference to the object provided.
// This behavior is instrumental to guarantee correct garbage collection of resources especially when resources
// control other resources in a multilevel hierarchy (think deployment-> repilcaset->pod).
func SetOwner(parent, child metav1.Object) {
	child.SetNamespace(parent.GetNamespace())

	// SetControllerReference sets owner as a Controller OwnerReference on controlled.
	// This is used for garbage collection of the controlled object and for
	// reconciling the owner object on changes to controlled (with a Watch + EnqueueRequestForOwner).
	// Since only one OwnerReference can be a controller, it returns an error if
	// there is another OwnerReference with Controller flag set.
	if err := controllerutil.SetControllerReference(parent, child, Globals.Client.Scheme()); err != nil {
		panic(errors.Wrapf(err, "set controller reference"))
	}

	/*
		if err := controllerutil.SetOwnerReference(parent, child, Globals.Client.Scheme()); err != nil {
			panic(errors.Wrapf(err, "set owner reference"))
		}
	*/

	// owner labels are used by the selectors
	child.SetLabels(labels.Merge(child.GetLabels(), map[string]string{
		"owner": parent.GetName(),
	}))
}
