package quality

import (
	"context"
	"fmt"
	"time"

	adaptivev1alpha1 "github.com/EdgeCDN-X/edgecdnx-plugin/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	client.Client
	Provider MetricsProvider
	Interval time.Duration
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var nq adaptivev1alpha1.NodeQuality
	if err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, &nq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	now := metav1.Now()
	if !nq.Spec.Enabled {
		nq.Status.State = "Disabled"
		nq.Status.Reason = "spec.enabled is false"
		nq.Status.EffectiveWeight = 0
		meta.SetStatusCondition(&nq.Status.Conditions, metav1.Condition{Type: "MetricsAvailable", Status: metav1.ConditionFalse, Reason: "Disabled", Message: "metrics are not queried for disabled nodes", ObservedGeneration: nq.Generation})
		return ctrl.Result{}, r.Status().Update(ctx, &nq)
	}
	sample, err := r.Provider.QueryNode(ctx, nq.Spec.NodeName, now.Time)
	if err != nil {
		meta.SetStatusCondition(&nq.Status.Conditions, metav1.Condition{Type: "MetricsAvailable", Status: metav1.ConditionFalse, Reason: "QueryFailed", Message: err.Error(), ObservedGeneration: nq.Generation})
		if updateErr := r.Status().Update(ctx, &nq); updateErr != nil && !apierrors.IsConflict(updateErr) {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}
	previous := nq.Status.State
	nq.Status.ObservedAt = &now
	nq.Status.SampleCount = int64(sample.RequestCount)
	nq.Status.ActiveRequests = int32(sample.ActiveRequests)
	nq.Status.State = "Healthy"
	nq.Status.Reason = "Prometheus metrics available"
	nq.Status.ConcurrencyLimit = nq.Spec.StaticCapacity
	if previous != nq.Status.State || nq.Status.StateSince == nil {
		nq.Status.StateSince = &now
	}
	meta.SetStatusCondition(&nq.Status.Conditions, metav1.Condition{Type: "MetricsAvailable", Status: metav1.ConditionTrue, Reason: "QuerySucceeded", Message: "real Prometheus samples were read", ObservedGeneration: nq.Generation})
	meta.SetStatusCondition(&nq.Status.Conditions, metav1.Condition{Type: "QualityComputed", Status: metav1.ConditionFalse, Reason: "PendingAdaptiveLogic", Message: "EWMA and weight calculation are implemented in Day 4", ObservedGeneration: nq.Generation})
	meta.SetStatusCondition(&nq.Status.Conditions, metav1.Condition{Type: "RoutingEligible", Status: metav1.ConditionFalse, Reason: "QualityNotComputed", Message: "baseline metrics must not drive routing yet", ObservedGeneration: nq.Generation})
	if err := r.Status().Update(ctx, &nq); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: r.Interval}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&adaptivev1alpha1.NodeQuality{}).Complete(r)
}
