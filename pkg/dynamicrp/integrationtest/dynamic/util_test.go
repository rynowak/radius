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

package dynamic

import (
	"context"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

func PutPlane(t *testing.T, ts *testserver.TestServer) {
	connection, err := sdk.NewDirectConnection(ts.BaseURL)
	require.NoError(t, err)

	clientOptions := sdk.NewClientOptions(connection)

	client, err := v20231001preview.NewRadiusPlanesClient(&aztoken.AnonymousCredential{}, clientOptions)
	require.NoError(t, err)

	plane := v20231001preview.RadiusPlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.RadiusPlaneResourceProperties{
			// Note: this is a workaround. Properties is marked as a required field in
			// the API. Without passing *something* here the body will be rejected.
			ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
			ResourceProviders: map[string]*string{},
		},
	}

	poller, err := client.BeginCreateOrUpdate(context.Background(), "local", plane, nil)
	require.NoError(t, err)

	_, err = poller.PollUntilDone(context.Background(), nil)
	require.NoError(t, err)
}
