/*
Copyright 2023 The Radius Authors.

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

package v20231001preview

import (
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"

	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned Kubernetes Plane resource to version-agnostic datamodel.
func (src *KubernetesPlaneResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.KubernetesPlane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},

		Properties: datamodel.KubernetesPlaneProperties{
			Server:                   to.String(src.Properties.Server),
			CertificateAuthorityData: to.String(src.Properties.CertificateAuthorityData),
			Auth: datamodel.KubernetesAuthentication{
				Kind: *src.Properties.Auth.GetKubernetesAuthenticationConfiguration().Kind,
			},
		},
	}

	switch auth := src.Properties.Auth.(type) {
	case *KubernetesInClusterConfiguration:
		converted.Properties.Auth.InCluster = &datamodel.KubernetesInClusterAuthentication{}
	case *KubernetesServiceAccountTokenConfiguration:
		converted.Properties.Auth.ServiceAccountToken = &datamodel.KubernetesServiceAccountTokenAuthentication{
			TokenData: to.String(auth.TokenData),
		}

	default:
		return nil, &v1.ErrModelConversion{PropertyName: "$.properties.auth.kind", ValidValue: strings.Join([]string{"InCluster", "ServiceAccountToken"}, ", ")}
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Kubernetes Plane resource.
func (dst *KubernetesPlaneResource) ConvertFrom(src v1.DataModelInterface) error {
	plane, ok := src.(*datamodel.KubernetesPlane)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &plane.ID
	dst.Name = &plane.Name
	dst.Type = &plane.Type
	dst.Location = &plane.Location
	dst.Tags = *to.StringMapPtr(plane.Tags)
	dst.SystemData = fromSystemDataModel(plane.SystemData)

	dst.Properties = &KubernetesPlaneResourceProperties{
		ProvisioningState:        fromProvisioningStateDataModel(plane.InternalMetadata.AsyncProvisioningState),
		Server:                   to.Ptr(plane.Properties.Server),
		CertificateAuthorityData: to.Ptr(plane.Properties.CertificateAuthorityData),
	}

	switch plane.Properties.Auth.Kind {
	case "InCluster":
		dst.Properties.Auth = &KubernetesInClusterConfiguration{
			Kind: to.Ptr(plane.Properties.Auth.Kind),
		}
	case "ServiceAccountToken":
		dst.Properties.Auth = &KubernetesServiceAccountTokenConfiguration{
			Kind: to.Ptr(plane.Properties.Auth.Kind),

			// OMIT the actual token data.
		}
	}

	return nil
}
