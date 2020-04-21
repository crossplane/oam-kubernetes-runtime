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

package workload

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
)

const (
	reconcileTimeout = 1 * time.Minute
	shortWait        = 30 * time.Second
	longWait         = 1 * time.Minute
)

// Reconcile error strings.
const (
	errGetWorkload              = "cannot get workload"
	errUpdateWorkloadStatus     = "cannot update workload status"
	errTranslateWorkload        = "cannot translate workload"
	errApplyWorkloadTranslation = "cannot apply workload translation"
)

// Reconcile event reasons.
const (
	reasonTranslateWorkload = "WorkloadTranslated"

	reasonCannotTranslateWorkload        = "CannotTranslateWorkload"
	reasonCannotApplyWorkloadTranslation = "CannotApplyWorkloadTranslation"
)

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

// WithTranslator specifies how the Reconciler should translate the workload.
func WithTranslator(t Translator) ReconcilerOption {
	return func(r *Reconciler) {
		r.workload = t
	}
}

// WithApplicator specifies how the Reconciler should apply the workload
// translation.
func WithApplicator(a resource.Applicator) ReconcilerOption {
	return func(r *Reconciler) {
		r.applicator = a
	}
}

// WithApplyOptions specifies options to pass to the applicator. These options
// are applied in addition to MustBeControllableBy, which is always applied and
// cannot be overridden.
func WithApplyOptions(a ...resource.ApplyOption) ReconcilerOption {
	return func(r *Reconciler) {
		r.applyOpts = a
	}
}

// A Reconciler reconciles an OAM workload type by packaging it into a
// KubernetesApplication.
type Reconciler struct {
	client      client.Client
	newWorkload func() oam.Workload
	workload    Translator
	applicator  resource.Applicator
	applyOpts   []resource.ApplyOption

	log    logging.Logger
	record event.Recorder
}

// NewReconciler returns a Reconciler that reconciles an OAM workload type by
// packaging it into a KubernetesApplication.
func NewReconciler(m ctrl.Manager, workload oam.WorkloadKind, o ...ReconcilerOption) *Reconciler {
	nw := func() oam.Workload {
		return resource.MustCreateObject(schema.GroupVersionKind(workload), m.GetScheme()).(oam.Workload)
	}

	r := &Reconciler{
		client:      m.GetClient(),
		newWorkload: nw,
		workload:    TranslateFn(NoopTranslate),
		applicator:  resource.NewAPIPatchingApplicator(m.GetClient()),
		applyOpts:   []resource.ApplyOption{},
		log:         logging.NewNopLogger(),
		record:      event.NewNopRecorder(),
	}

	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile an OAM workload type by packaging it into a KubernetesApplication.
func (r *Reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	workload := r.newWorkload()
	if err := r.client.Get(ctx, req.NamespacedName, workload); err != nil {
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetWorkload)
	}

	log = log.WithValues("uid", workload.GetUID(), "version", workload.GetResourceVersion())

	objs, err := r.workload.Translate(ctx, workload)
	if err != nil {
		log.Debug("Cannot translate workload", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(workload, event.Warning(reasonCannotTranslateWorkload, err))
		workload.SetConditions(runtimev1alpha1.ReconcileError(errors.Wrap(err, errTranslateWorkload)))
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, workload), errUpdateWorkloadStatus)
	}

	for _, o := range objs {
		// A workload's translation must be controlled by the workload.
		meta.AddOwnerReference(o, *metav1.NewControllerRef(workload, workload.GetObjectKind().GroupVersionKind()))

		// All top-level objects must be in the same namespace as the workload.
		o.SetNamespace(workload.GetNamespace())

		// All top-level objects must have the same name as the workload.
		// TODO(hasheddan): this restriction means that you can only have one
		// top-level object of a given kind per workload translation. In the
		// future, it would be ideal to allow for multiple instances of a single
		// object kind per workload translation. At that time, this naming
		// restriction should be removed, and the trait reconciler should list
		// objects by labels added below.
		o.SetName(workload.GetName())

		// All top-level objects must have the workload label so that they can
		// be listed by traits.
		// TODO(hasheddan): currently the trait controller only gets one object
		// of the given kind that has the same name as its referenced workload,
		// so this label is not being used.
		meta.AddLabels(o, map[string]string{lowerGroupKind(workload.GetObjectKind()): string(workload.GetUID())})

		opts := append([]resource.ApplyOption{resource.MustBeControllableBy(workload.GetUID())}, r.applyOpts...)
		if err := r.applicator.Apply(ctx, o, opts...); err != nil {
			log.Debug("Cannot apply workload translation", "error", err, "requeue-after", time.Now().Add(shortWait))
			r.record.Event(workload, event.Warning(reasonCannotApplyWorkloadTranslation, err))
			workload.SetConditions(runtimev1alpha1.ReconcileError(errors.Wrap(err, errApplyWorkloadTranslation)))
			return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, workload), errUpdateWorkloadStatus)
		}
	}

	r.record.Event(workload, event.Normal(reasonTranslateWorkload, "Successfully translated workload"))
	log.Debug("Successfully translated workload", "kind", workload.GetObjectKind().GroupVersionKind().String())

	workload.SetConditions(runtimev1alpha1.ReconcileSuccess())
	return reconcile.Result{RequeueAfter: longWait}, errors.Wrap(r.client.Status().Update(ctx, workload), errUpdateWorkloadStatus)
}

func lowerGroupKind(gk schema.ObjectKind) string {
	return strings.ToLower(gk.GroupVersionKind().GroupKind().String())
}
