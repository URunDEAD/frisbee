package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase is the current status of an object
type Phase string

// These are the valid statuses of services. The following lifecycles are valid:
// Uninitialized -> Failed
// Uninitialized -> Running* -> Completed
// Uninitialized -> Running* -> Failed
// Uninitialized -> Chaos* -> Completed
// Uninitialized -> Running* -> Chaos -> Completed
// The asterix (*) Indicate that the same phase may appear recursively.
const (
	// Uninitialized means that the service has been accepted by the system, but one or more of the containers
	// has not been started. This includes time before being bound to a node, as well as time spent
	// pulling images onto the host.
	Uninitialized Phase = ""

	// Running means the services has been bound to a node and all of the containers have been started.
	// At least one container is still running or is in the process of being restarted.
	Running Phase = "Running"

	// Complete means that all containers in the pod have voluntarily terminated
	// with a container exit code of 0, and the system is not going to restart any of these containers.
	Complete Phase = "Complete"

	// Failed means that all containers in the pod have terminated, and at least one container has
	// terminated in a failure (exited with a non-zero exit code or was stopped by the system).
	Failed Phase = "Failed"

	// Chaos indicates a managed abnormal condition such STOP or KILL. In this phase, the controller ignores
	// any subsequent failures and let the system under evaluation to progress as it can.
	Chaos Phase = "Chaos"
)

type EtherStatus struct {
	// +kubebuilder:validation:Enum=Running;Complete;Failed;Chaos
	Phase Phase `json:"phase,omitempty"`

	// A brief CamelCase message indicating details about why the service is in this Phase.
	// e.g. 'Evicted'
	// +optional
	Reason string `json:"reason,omitempty"`

	// RFC 3339 date and time at which the object was acknowledged by the Kubelet.
	// This is before the Kubelet pulled the container image(s) for the pod.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Most recently observed status of the object
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`
}
