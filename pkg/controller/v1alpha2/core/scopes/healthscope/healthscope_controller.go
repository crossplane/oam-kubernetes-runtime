/*
Copyright 2020 The Crossplane Authors.

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

package healthscope

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller"
)

const (
	reconcileTimeout = 1 * time.Minute
	shortWait        = 30 * time.Second
	longWait         = 1 * time.Minute
)

// Reconcile error strings.
const (
	errGetHealthScope          = "cannot get health scope"
	errUpdateHealthScopeStatus = "cannot update health scope status"
)

// Reconcile event reasons.
const (
	reasonHealthCheck       = "HealthCheck"
	reasonHealthCheckFailed = "HealthCheckFailed"
)

// Setup adds a controller that reconciles HealthScope.
func Setup(mgr ctrl.Manager, args controller.Args, l logging.Logger) error {
	name := "oam/" + strings.ToLower(v1alpha2.HealthScopeGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha2.HealthScope{}).
		Complete(NewReconciler(mgr,
			WithLogger(l.WithValues("controller", name)),
			WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name)))))
}

// A Reconciler reconciles OAM Scopes by keeping track of the health status of components.
type Reconciler struct {
	client client.Client

	log    logging.Logger
	record event.Recorder
}

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// NewReconciler returns a Reconciler that reconciles HealthScope by keeping track of its healthstatus.
func NewReconciler(m ctrl.Manager, o ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client: m.GetClient(),
		log:    logging.NewNopLogger(),
		record: event.NewNopRecorder(),
	}

	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile an OAM HealthScope by keeping track of its health status.
func (r *Reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	hs := &v1alpha2.HealthScope{}
	if err := r.client.Get(ctx, req.NamespacedName, hs); err != nil {
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetHealthScope)
	}

	interval := longWait
	if hs.Spec.ProbeInterval != nil {
		interval = time.Duration(*hs.Spec.ProbeInterval) * time.Second
	}

	if interval <= 0 {
		interval = longWait
	}

	start := time.Now()

	log = log.WithValues("uid", hs.GetUID(), "version", hs.GetResourceVersion())

	if err := UpdateHealthStatus(ctx, log, r.client, hs); err != nil {
		log.Debug("Could not update health status", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(hs, event.Warning(reasonHealthCheckFailed, err))
		hs.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errUpdateHealthScopeStatus)))
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, hs), errUpdateHealthScopeStatus)
	}

	log.Debug("Successfully ran health check", "scope", hs.Name)
	r.record.Event(hs, event.Normal(reasonHealthCheck, "Successfully ran health check"))

	elapsed := time.Since(start)

	hs.SetConditions(v1alpha1.ReconcileSuccess())
	return reconcile.Result{RequeueAfter: interval - elapsed}, errors.Wrap(r.client.Status().Update(ctx, hs), errUpdateHealthScopeStatus)
}
