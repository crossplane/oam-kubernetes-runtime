// +build integration

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

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller/oam"
)

func TestAppConfigControllerWithManifests(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		test   func(c client.Client) error
	}{
		{
			name:   "ApplicationConfigurationRendersWorkloads",
			reason: "An ApplicationConfiguration should render its workloads.",
			test: func(c client.Client) error {
				d := &v1alpha2.WorkloadDefinition{}
				if err := unmarshalFromFile("../testdata/workloaddefinition.yaml", d); err != nil {
					return err
				}

				if err := c.Create(context.Background(), d); err != nil {
					return err
				}

				co := &v1alpha2.Component{}
				if err := unmarshalFromFile("../testdata/component.yaml", co); err != nil {
					return err
				}

				if err := c.Create(context.Background(), co); err != nil {
					return err
				}

				a := &v1alpha2.ApplicationConfiguration{}
				if err := unmarshalFromFile("../testdata/appconfig.yaml", a); err != nil {
					return err
				}

				if err := c.Create(context.Background(), a); err != nil {
					return err
				}

				if err := waitFor(context.Background(), 3*time.Second, func() (bool, error) {
					cw := &v1alpha2.ContainerizedWorkload{}
					if err := c.Get(context.Background(), types.NamespacedName{Name: cwName, Namespace: defaultNS}, cw); err != nil {
						if kerrors.IsNotFound(err) {
							return false, nil
						}
						return false, err
					}

					if len(cw.Spec.Containers) != 1 {
						return true, errors.New(errUnexpectedContainers)
					}
					for i, e := range cw.Spec.Containers[0].Environment {
						if e.Name != envVars[i] {
							return true, errors.New(errUnexpectedSubstitution)
						}
						if e.Value != nil && *e.Value != paramVals[i] {
							return true, errors.New(errUnexpectedSubstitution)
						}
					}

					return true, nil
				}); err != nil {
					return err
				}

				if err := c.Delete(context.Background(), a); err != nil {
					return err
				}

				err := waitFor(context.Background(), 3*time.Second, func() (bool, error) {
					cw := &v1alpha2.ContainerizedWorkload{}
					if err := c.Get(context.Background(), types.NamespacedName{Name: cwName, Namespace: defaultNS}, cw); err != nil {
						if kerrors.IsNotFound(err) {
							return true, nil
						}
						return false, err
					}

					return false, nil
				})

				return err
			},
		},
	}

	cfg, err := ctrl.GetConfig()
	if err != nil {
		t.Fatal(err)
	}

	i, err := integration.New(cfg,
		integration.WithCRDPaths("../../crds"),
		integration.WithCleaners(
			integration.NewCRDCleaner(),
			integration.NewCRDDirCleaner()),
	)

	if err != nil {
		t.Fatal(err)
	}

	if err := core.AddToScheme(i.GetScheme()); err != nil {
		t.Fatal(err)
	}

	if err := corev1.AddToScheme(i.GetScheme()); err != nil {
		t.Fatal(err)
	}

	if err := apiextensionsv1beta1.AddToScheme(i.GetScheme()); err != nil {
		t.Fatal(err)
	}

	zl := zap.New(zap.UseDevMode(true))
	log := logging.NewLogrLogger(zl.WithName("app-config"))
	if err := oam.Setup(i, log); err != nil {
		t.Fatal(err)
	}

	i.Run()

	defer func() {
		if err := i.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.test(i.GetClient())
			if err != nil {
				t.Error(err)
			}
		})
	}
}
