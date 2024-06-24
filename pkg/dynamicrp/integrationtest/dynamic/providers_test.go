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
	"net/http"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp/testhost"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const (
	radiusPlaneName           = "testing"
	resourceProviderNamespace = "Applications.Test"
	resourceTypeName          = "exampleResources"
	locationName              = v1.LocationGlobal
	apiVersion                = "2024-01-01"

	resourceGroupName   = "test-group"
	exampleResourceName = "my-example"

	exampleResourcePlaneID            = "/planes/radius/" + radiusPlaneName
	exampleResourceGroupID            = exampleResourcePlaneID + "/resourceGroups/test-group"
	exampleResourcePlaneCollectionURL = exampleResourcePlaneID + "/providers/Applications.Test/exampleResources?api-version=" + apiVersion
	exampleResourceCollectionURL      = exampleResourceGroupID + "/providers/Applications.Test/exampleResources?api-version=" + apiVersion

	exampleResourceID  = exampleResourceGroupID + "/providers/Applications.Test/exampleResources/" + exampleResourceName
	exampleResourceURL = exampleResourceID + "?api-version=" + apiVersion

	exampleResourceEmptyListResponseFixture = "testdata/exampleresource_v20240101preview_emptylist_responsebody.json"
	exampleResourceListResponseFixture      = "testdata/exampleresource_v20240101preview_list_responsebody.json"

	exampleResourceRequestFixture          = "testdata/exampleresource_v20240101preview_requestbody.json"
	exampleResourceResponseFixture         = "testdata/exampleresource_v20240101preview_responsebody.json"
	exampleResourceAcceptedResponseFixture = "testdata/exampleresource_v20240101preview_accepted_responsebody.json"
)

// This test covers the lifecycle of a dynamic resource.
func Test_Dynamic_Resource_Lifecycle(t *testing.T) {
	_, server := testhost.Start(t)

	// Setup a resource provider (Applications.Test/exampleResources)
	createRadiusPlane(server)
	createResourceProvider(server)
	createResourceType(server)
	createAPIVersion(server)
	createLocation(server)

	// Setup a resource group where we can interact with the new resource type.
	createResourceGroup(server)

	// List should start empty
	response := server.MakeRequest(http.MethodGet, exampleResourceCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceEmptyListResponseFixture)

	// Getting a specific resource should return 404.
	response = server.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsErrorCode(404, "NotFound")

	// Create a resource
	response = server.MakeFixtureRequest(http.MethodPut, exampleResourceURL, exampleResourceRequestFixture)
	response.EqualsFixture(201, exampleResourceAcceptedResponseFixture)

	// Verify async operations
	operationStatusResponse := server.MakeRequest(http.MethodGet, response.Raw.Header.Get("Azure-AsyncOperation"), nil)
	operationStatusResponse.EqualsStatusCode(200)

	operationStatus := v1.AsyncOperationStatus{}
	operationStatusResponse.ReadAs(&operationStatus)

	require.Equal(t, v1.ProvisioningStateAccepted, operationStatus.Status)
	require.NotNil(t, operationStatus.StartTime)

	statusID, err := resources.ParseResource(operationStatus.ID)
	require.NoError(t, err)
	require.Equal(t, "applications.test/locations/operationstatuses", statusID.Type())
	require.Equal(t, statusID.Name(), operationStatus.Name)

	operationResultResponse := server.MakeRequest(http.MethodGet, response.Raw.Header.Get("Location"), nil)
	require.Truef(t, operationResultResponse.Raw.StatusCode == http.StatusAccepted || operationResultResponse.Raw.StatusCode == http.StatusNoContent, "Expected 202 or 204 response")

	response = response.WaitForOperationComplete(nil)
	response.EqualsStatusCode(200)

	// List at plane scope should now contain the resource
	response = server.MakeRequest(http.MethodGet, exampleResourcePlaneCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceListResponseFixture)

	// List at resource group should now contain the resource
	response = server.MakeRequest(http.MethodGet, exampleResourceCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceListResponseFixture)

	// Getting the resource should return 200
	response = server.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsFixture(200, exampleResourceResponseFixture)

	// Deleting a resource should return 202
	response = server.MakeRequest(http.MethodDelete, exampleResourceURL, nil)
	response.EqualsStatusCode(202)

	response = response.WaitForOperationComplete(nil)
	response.EqualsStatusCode(200)

	// Now the resource is gone
	response = server.MakeRequest(http.MethodGet, exampleResourceCollectionURL, nil)
	response.EqualsFixture(200, exampleResourceEmptyListResponseFixture)
	response = server.MakeRequest(http.MethodGet, exampleResourceURL, nil)
	response.EqualsErrorCode(404, "NotFound")
}

func createRadiusPlane(server *testserver.TestServer) v20231001preview.RadiusPlanesClientCreateOrUpdateResponse {
	ctx := context.Background()

	plane := v20231001preview.RadiusPlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.RadiusPlaneResourceProperties{
			// Note: this is a workaround. Properties is marked as a required field in
			// the API. Without passing *something* here the body will be rejected.
			ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
			ResourceProviders: map[string]*string{},
		},
	}

	client := server.UCP().NewRadiusPlanesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, plane, nil)
	require.NoError(server.T(), err)

	response, err := poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)

	return response
}

func createResourceProvider(server *testserver.TestServer) {
	ctx := context.Background()

	resourceProvider := v20231001preview.ResourceProviderResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceProviderProperties{},
	}

	client := server.UCP().NewResourceProvidersClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceProvider, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createResourceType(server *testserver.TestServer) {
	ctx := context.Background()

	resourceType := v20231001preview.ResourceTypeResource{
		Properties: &v20231001preview.ResourceTypeProperties{
			DefaultAPIVersion: to.Ptr(apiVersion),
		},
	}

	client := server.UCP().NewResourceTypesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceTypeName, resourceType, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createAPIVersion(server *testserver.TestServer) {
	ctx := context.Background()

	apiVersionResource := v20231001preview.APIVersionResource{
		Properties: &v20231001preview.APIVersionProperties{},
	}

	client := server.UCP().NewAPIVersionsClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceTypeName, apiVersion, apiVersionResource, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createLocation(server *testserver.TestServer) {
	ctx := context.Background()

	location := v20231001preview.LocationResource{
		Properties: &v20231001preview.LocationProperties{
			ResourceTypes: map[string]*v20231001preview.LocationResourceType{
				resourceTypeName: {
					APIVersions: map[string]map[string]any{
						apiVersion: {},
					},
				},
			},
		},
	}

	client := server.UCP().NewLocationsClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, locationName, location, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createResourceGroup(server *testserver.TestServer) {
	ctx := context.Background()

	resourceGroup := v20231001preview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceGroupProperties{},
	}

	client := server.UCP().NewResourceGroupsClient()
	_, err := client.CreateOrUpdate(ctx, radiusPlaneName, resourceGroupName, resourceGroup, nil)
	require.NoError(server.T(), err)
}
