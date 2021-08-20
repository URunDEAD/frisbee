package lifecycle

import (
	"strings"

	"github.com/fnikolai/frisbee/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pod translates the Pod's Lifecycle to Frisbee Lifecycle
func Pod() GetLifecycleFunc {
	return func(obj interface{}) []*v1alpha1.Lifecycle {
		pod := obj.(*corev1.Pod)

		switch pod.Status.Phase {
		case corev1.PodPending:
			return []*v1alpha1.Lifecycle{{
				Kind:      "Pod",
				Name:      pod.GetName(),
				Phase:     v1alpha1.PhasePending,
				Reason:    pod.Status.Reason,
				StartTime: &metav1.Time{Time: pod.GetCreationTimestamp().Time},
				EndTime:   nil,
			}}

		case corev1.PodRunning:
			return []*v1alpha1.Lifecycle{{
				Kind:      "Pod",
				Name:      pod.GetName(),
				Phase:     v1alpha1.PhaseRunning,
				Reason:    pod.Status.Reason,
				StartTime: &metav1.Time{Time: pod.GetCreationTimestamp().Time},
				EndTime:   nil,
			}}

		case corev1.PodSucceeded:
			return []*v1alpha1.Lifecycle{{
				Kind:      "Pod",
				Name:      pod.GetName(),
				Phase:     v1alpha1.PhaseSuccess,
				Reason:    pod.Status.Reason,
				StartTime: &metav1.Time{Time: pod.GetCreationTimestamp().Time},
				EndTime:   pod.GetDeletionTimestamp(),
			}}

		case corev1.PodFailed:
			return []*v1alpha1.Lifecycle{{
				Kind:      "Pod",
				Name:      pod.GetName(),
				Phase:     v1alpha1.PhaseFailed,
				Reason:    pod.Status.Reason,
				StartTime: &metav1.Time{Time: pod.GetCreationTimestamp().Time},
				EndTime:   pod.GetDeletionTimestamp(),
			}}

		case corev1.PodUnknown:
			return []*v1alpha1.Lifecycle{{
				Kind:      "Pod",
				Name:      pod.GetName(),
				Phase:     v1alpha1.PhaseFailed,
				Reason:    "unknown state",
				StartTime: &metav1.Time{Time: pod.GetCreationTimestamp().Time},
				EndTime:   pod.GetDeletionTimestamp(),
			}}
		default:
			return []*v1alpha1.Lifecycle{}
		}
	}
}

// Containers translates the Container's Lifecycle to Frisbee Lifecycle.
func Containers() GetLifecycleFunc {
	return func(obj interface{}) []*v1alpha1.Lifecycle {
		var lifecycles []*v1alpha1.Lifecycle

		pod := obj.(*corev1.Pod)

		for _, container := range pod.Status.ContainerStatuses {
			// todo: to go on, we currently ignore the status of sidecars.
			// find a way to overcome this limitation
			if strings.Contains(container.Name, "-") {
				continue
			}

			switch {
			case container.State.Waiting != nil:
				state := container.State.Waiting

				lifecycles = append(lifecycles, &v1alpha1.Lifecycle{
					Kind:      "Container",
					Name:      container.Name,
					Phase:     v1alpha1.PhasePending,
					Reason:    state.Reason,
					StartTime: nil,
					EndTime:   nil,
				})

			case container.State.Running != nil:
				state := container.State.Running

				lifecycles = append(lifecycles, &v1alpha1.Lifecycle{
					Kind:      "Container",
					Name:      container.Name,
					Phase:     v1alpha1.PhaseRunning,
					Reason:    "container is started",
					StartTime: &state.StartedAt,
					EndTime:   nil,
				})

			case container.State.Terminated != nil:
				state := container.State.Terminated

				if state.ExitCode == 0 {
					lifecycles = append(lifecycles, &v1alpha1.Lifecycle{
						Kind:      "Container",
						Name:      container.Name,
						Phase:     v1alpha1.PhaseSuccess,
						Reason:    state.Reason,
						StartTime: &state.StartedAt,
						EndTime:   &state.FinishedAt,
					})
				} else {
					lifecycles = append(lifecycles, &v1alpha1.Lifecycle{
						Kind:      "Container",
						Name:      container.Name,
						Phase:     v1alpha1.PhaseFailed,
						Reason:    state.Reason,
						StartTime: &state.StartedAt,
						EndTime:   &state.FinishedAt,
					})
				}
			}
		}

		return lifecycles
	}
}
