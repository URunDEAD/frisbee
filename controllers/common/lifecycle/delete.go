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

package lifecycle

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/******************************************************
			Delete Managed objects
/******************************************************/

// Delete is a wrapper that addresses a circular dependency issue with the lifecycle monitoring.
// By default, Kubernetes deletes Children before the parent. When a Child is removed,
// the lifecycle watchdog detects that a child is deleted (failed) and updates the parent. However,
// the parent used in the lifecycle is a stalled copy of the actual parent object. Hence, the update
// causes a conflict between the stalled and the actual object.
//
// This deletion method addresses this issue by first deleting the parent, and then the children.
func Delete(ctx context.Context, c client.Client, obj client.Object) error {
	// There are three different options for the deletion propagation policy:
	//
	//    Foreground: Children are deleted before the parent (post-order)
	//    Background: Parent is deleted before the children (pre-order)
	//    Orphan: Owner references are ignored
	deletePolicy := metav1.DeletePropagationBackground

	if err := c.Delete(ctx, obj, &client.DeleteOptions{PropagationPolicy: &deletePolicy}); err != nil {
		return errors.Wrapf(err, "unable to delete object %s", obj.GetName())
	}

	return nil
}