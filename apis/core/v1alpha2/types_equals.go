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

package v1alpha2

import (
	"bytes"
	"reflect"
)

// Equal tests for equality between two ComponentSpec types
func (c1 *ComponentSpec) Equal(c2 *ComponentSpec) bool {
	if !reflect.DeepEqual(c1.Parameters, c2.Parameters) {
		return false
	}

	if c1.Workload.Object != nil && c2.Workload.Object != nil {
		return reflect.DeepEqual(c1.Workload.Object, c2.Workload.Object)
	}

	c1data, err := c1.Workload.MarshalJSON()
	if err != nil {
		return false
	}
	c2data, err := c2.Workload.MarshalJSON()
	if err != nil {
		return false
	}
	return bytes.Equal(c1data, c2data)
}
