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
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func Test_KubernetesPlane_ConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.KubernetesPlane
		err      error
	}{
		{
			filename: "kubernetesplane-resource-incluster.json",
			expected: &datamodel.KubernetesPlane{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/kubernetes/abcd",
						Name:     "abcd",
						Type:     "System.Planes/kubernetes",
						Location: "global",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.KubernetesPlaneProperties{
					Server:                   "https://mycluster.example.com:9443",
					CertificateAuthorityData: "AAAA===",
					Auth: datamodel.KubernetesAuthentication{
						Kind:      "InCluster",
						InCluster: &datamodel.KubernetesInClusterAuthentication{},
					},
				},
			},
		},
		{
			filename: "kubernetesplane-resource-serviceaccounttoken.json",
			expected: &datamodel.KubernetesPlane{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/kubernetes/abcd",
						Name:     "abcd",
						Type:     "System.Planes/kubernetes",
						Location: "global",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.KubernetesPlaneProperties{
					Server:                   "https://mycluster.example.com:9443",
					CertificateAuthorityData: "AAAA===",
					Auth: datamodel.KubernetesAuthentication{
						Kind: "ServiceAccountToken",
						ServiceAccountToken: &datamodel.KubernetesServiceAccountTokenAuthentication{
							TokenData: "BBBB===",
						},
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &KubernetesPlaneResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.KubernetesPlane)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func Test_KubernetesPlane_ConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *KubernetesPlaneResource
		err      error
	}{
		{
			filename: "kubernetesplane-datamodel-incluster.json",
			expected: &KubernetesPlaneResource{
				ID:       to.Ptr("/planes/kubernetes/abcd"),
				Name:     to.Ptr("abcd"),
				Type:     to.Ptr("System.Planes/kubernetes"),
				Location: to.Ptr("global"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &KubernetesPlaneResourceProperties{
					ProvisioningState:        fromProvisioningStateDataModel(v1.ProvisioningStateSucceeded),
					Server:                   to.Ptr("https://mycluster.example.com:9443"),
					CertificateAuthorityData: to.Ptr("AAAA==="),
					Auth: &KubernetesInClusterConfiguration{
						Kind: to.Ptr("InCluster"),
					},
				},
			},
		},
		{
			filename: "kubernetesplane-datamodel-serviceaccounttoken.json",
			expected: &KubernetesPlaneResource{
				ID:       to.Ptr("/planes/kubernetes/abcd"),
				Name:     to.Ptr("abcd"),
				Type:     to.Ptr("System.Planes/kubernetes"),
				Location: to.Ptr("global"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &KubernetesPlaneResourceProperties{
					ProvisioningState:        fromProvisioningStateDataModel(v1.ProvisioningStateSucceeded),
					Server:                   to.Ptr("https://mycluster.example.com:9443"),
					CertificateAuthorityData: to.Ptr("AAAA==="),
					Auth: &KubernetesServiceAccountTokenConfiguration{
						Kind: to.Ptr("ServiceAccountToken"),
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			dm := &datamodel.KubernetesPlane{}
			err := json.Unmarshal(rawPayload, dm)
			require.NoError(t, err)

			resource := &KubernetesPlaneResource{}
			err = resource.ConvertFrom(dm)

			// Avoid hardcoding the SystemData field in tests.
			tt.expected.SystemData = fromSystemDataModel(dm.SystemData)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, resource)
			}
		})
	}
}
