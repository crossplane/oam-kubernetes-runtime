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

	"github.com/crossplane/oam-runtime/pkg/oam"
)

// A Translator is responsible for packaging workloads into other objects.
type Translator interface {
	Translate(context.Context, oam.Workload) ([]oam.Object, error)
}

// An ObjectTranslator is a concrete implementation of a Translator.
type ObjectTranslator struct {
	TranslateFn
}

// Translate a workload into other objects.
func (p *ObjectTranslator) Translate(ctx context.Context, w oam.Workload) ([]oam.Object, error) {
	return p.TranslateFn(ctx, w)
}

// NewObjectTranslatorWithWrappers returns a Translator that translates and wraps
// a workload.
func NewObjectTranslatorWithWrappers(t TranslateFn, wp ...TranslationWrapper) Translator {
	return &ObjectTranslator{
		TranslateFn: func(ctx context.Context, w oam.Workload) ([]oam.Object, error) {
			objs, err := t(ctx, w)
			if err != nil {
				return nil, err
			}
			for _, wrap := range wp {
				if objs, err = wrap(ctx, w, objs); err != nil {
					return nil, err
				}
			}
			return objs, nil
		},
	}
}

// A TranslateFn translates a workload into an object.
type TranslateFn func(context.Context, oam.Workload) ([]oam.Object, error)

// Translate workload into object or objects with no wrappers.
func (fn TranslateFn) Translate(ctx context.Context, w oam.Workload) ([]oam.Object, error) {
	return fn(ctx, w)
}

var _ Translator = TranslateFn(NoopTranslate)

// NoopTranslate does not translate the workload and does not return error.
func NoopTranslate(ctx context.Context, w oam.Workload) ([]oam.Object, error) {
	return nil, nil
}

// A TranslationWrapper wraps the output of a workload translation in another
// object or adds addition object.
type TranslationWrapper func(context.Context, oam.Workload, []oam.Object) ([]oam.Object, error)

var _ TranslationWrapper = NoopWrapper

// NoopWrapper does not wrap the workload translation and does not return error.
func NoopWrapper(ctx context.Context, w oam.Workload, objs []oam.Object) ([]oam.Object, error) {
	return objs, nil
}
