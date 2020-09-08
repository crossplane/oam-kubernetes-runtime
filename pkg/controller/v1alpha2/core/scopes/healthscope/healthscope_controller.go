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
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller"
)

const (
	reconcileTimeout = 1 * time.Minute
	// shortWait        = 30 * time.Second
	longWait = 1 * time.Minute
)

// Reconcile error strings.
const (
	errGetHealthScope          = "cannot get health scope"
	errMarshalScopeDiagnosiis  = "cannot marshal diagnosis of the scope"
	errUpdateHealthScopeStatus = "cannot update health scope status"
)

// Reconcile event reasons.
const (
	reasonHealthCheck = "HealthCheck"
)

// Setup adds a controller that reconciles HealthScope.
func Setup(mgr ctrl.Manager, args controller.Args, l logging.Logger) error {
	name := "oam/" + strings.ToLower(v1alpha2.HealthScopeGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha2.HealthScope{}).
		Complete(NewReconciler(mgr,
			WithLogger(l.WithValues("controller", name)),
			WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		))
}

// A Reconciler reconciles OAM Scopes by keeping track of the health status of components.
type Reconciler struct {
	client client.Client

	log      logging.Logger
	record   event.Recorder
	checkers []WorloadHealthChecker
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

// WithChecker adds workload health checker
func WithChecker(c WorloadHealthChecker) ReconcilerOption {
	return func(r *Reconciler) {
		if r.checkers == nil {
			r.checkers = make([]WorloadHealthChecker, 0)
		}
		r.checkers = append(r.checkers, c)
	}
}

// NewReconciler returns a Reconciler that reconciles HealthScope by keeping track of its healthstatus.
func NewReconciler(m ctrl.Manager, o ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client: m.GetClient(),
		log:    logging.NewNopLogger(),
		record: event.NewNopRecorder(),
		checkers: []WorloadHealthChecker{
			WorkloadHealthCheckFn(CheckContainerziedWorkloadHealth),
			WorkloadHealthCheckFn(CheckDeploymentHealth),
			WorkloadHealthCheckFn(CheckStatefulsetHealth),
			WorkloadHealthCheckFn(CheckDaemonsetHealth),
		},
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

	hsStatus := r.GetScopeHealthStatus(ctx, hs)
	log.Debug("Successfully ran health check", "scope", hs.Name)
	r.record.Event(hs, event.Normal(reasonHealthCheck, "Successfully ran health check"))

	elapsed := time.Since(start)

	if hsStatus.IsHealthy {
		hs.Status.Health = "healthy"
	} else {
		hs.Status.Health = "unhealthy"
	}

	diagnosis, err := json.Marshal(hsStatus)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, errMarshalScopeDiagnosiis)
	}

	// sava diagnosis into status.conditions
	//TODO(roywang) is there a better way to show diagnosis
	hs.SetConditions(v1alpha1.ReconcileSuccess().
		WithMessage(string(diagnosis)))

	return reconcile.Result{RequeueAfter: interval - elapsed}, errors.Wrap(r.client.Status().Update(ctx, hs), errUpdateHealthScopeStatus)
}

// GetScopeHealthStatus get the status of the healthscope based on workload resources.
func (r *Reconciler) GetScopeHealthStatus(ctx context.Context, healthScope *v1alpha2.HealthScope) HealthCondition {
	hsStatus := HealthCondition{
		Target: runtimev1alpha1.TypedReference{
			APIVersion: healthScope.APIVersion,
			Kind:       healthScope.Kind,
			Name:       healthScope.Name,
			UID:        healthScope.UID,
		},
		IsHealthy:     true, //if no workload referenced, scope is healthy by default
		SubConditions: []*HealthCondition{},
	}
	timeout := defaultTimeout
	if healthScope.Spec.ProbeTimeout != nil {
		timeout = time.Duration(*healthScope.Spec.ProbeTimeout) * time.Second
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scopeWLRefs := healthScope.Spec.WorkloadReferences
	// process workloads concurrently
	wlStatusCs := make(chan *HealthCondition, len(scopeWLRefs))
	var wg sync.WaitGroup
	wg.Add(len(scopeWLRefs))

	for _, workloadRef := range scopeWLRefs {
		go func(resRef runtimev1alpha1.TypedReference) {
			defer wg.Done()
			var wlStatusC *HealthCondition
			for _, checker := range r.checkers {
				wlStatusC = checker.Check(ctxWithTimeout, r.client, resRef, healthScope.GetNamespace())
				if wlStatusC != nil {
					// found matched checker and get health status
					wlStatusCs <- wlStatusC
					return
				}
			}
			// unsupportted workload
			wlStatusCs <- &HealthCondition{
				Target:    resRef,
				IsHealthy: false,
				Diagnosis: fmt.Sprintf(errFmtUnsupportWorkload, resRef.APIVersion, resRef.Kind),
			}
		}(workloadRef)
	}

	go func() {
		wg.Wait()
		close(wlStatusCs)
	}()

	for wlStatus := range wlStatusCs {
		// any unhealthy workload makes the scope unhealthy
		hsStatus.IsHealthy = hsStatus.IsHealthy && wlStatus.IsHealthy
		hsStatus.SubConditions = append(hsStatus.SubConditions, wlStatus)
	}
	return hsStatus
}
