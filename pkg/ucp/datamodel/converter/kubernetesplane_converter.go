/*
Copyright 2023 The Kubernetes Authors.

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

package converter

import (
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// KubernetesPlaneDataModelToVersioned converts version agnostic Kubernetes plane datamodel to versioned model.
func KubernetesPlaneDataModelToVersioned(model *datamodel.KubernetesPlane, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.KubernetesPlaneResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// KubernetesPlaneDataModelFromVersioned converts versioned Kubernetes plane model to datamodel.
func KubernetesPlaneDataModelFromVersioned(content []byte, version string) (*datamodel.KubernetesPlane, error) {
	switch version {
	case v20231001preview.Version:
		vm := &v20231001preview.KubernetesPlaneResource{}
		if err := json.Unmarshal(content, vm); err != nil {
			return nil, err
		}
		dm, err := vm.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.KubernetesPlane), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
