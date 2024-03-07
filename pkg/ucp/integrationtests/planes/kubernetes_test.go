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

package planes

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/frontend/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
)

const (
	kubernetesPlaneCollectionURL          = "/planes/kubernetes?api-version=2023-10-01-preview"
	kubernetesPlaneResourceURL            = "/planes/kubernetes/local?api-version=2023-10-01-preview"
	kubernetesPlaneRequestFixture         = "testdata/kubernetesplane_v20231001preview_requestbody.json"
	kubernetesPlaneResponseFixture        = "testdata/kubernetesplane_v20231001preview_responsebody.json"
	kubernetesPlaneListResponseFixture    = "testdata/kubernetesplane_v20231001preview_list_responsebody.json"
	kubernetesPlaneUpdatedRequestFixture  = "testdata/kubernetesplane_updated_v20231001preview_requestbody.json"
	kubernetesPlaneUpdatedResponseFixture = "testdata/kubernetesplane_updated_v20231001preview_responsebody.json"
)

func configureKubernetesModule(options modules.Options) []modules.Initializer {
	return []modules.Initializer{
		kubernetes.NewModule(options),
	}
}

func Test_KubernetesPlane_PUT_Create(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", kubernetesPlaneResourceURL, kubernetesPlaneRequestFixture)
	response.EqualsFixture(200, kubernetesPlaneResponseFixture)
}

func Test_KubernetesPlane_PUT_Update(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", kubernetesPlaneResourceURL, kubernetesPlaneRequestFixture)
	response.EqualsFixture(200, kubernetesPlaneResponseFixture)

	response = server.MakeFixtureRequest("PUT", kubernetesPlaneResourceURL, kubernetesPlaneUpdatedRequestFixture)
	response.EqualsFixture(200, kubernetesPlaneUpdatedResponseFixture)
}

func Test_KubernetesPlane_GET_Empty(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	response := server.MakeRequest("GET", kubernetesPlaneResourceURL, nil)
	response.EqualsErrorCode(404, v1.CodeNotFound)
}

func Test_KubernetesPlane_GET_Found(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", kubernetesPlaneResourceURL, kubernetesPlaneRequestFixture)
	response.EqualsFixture(200, kubernetesPlaneResponseFixture)

	response = server.MakeRequest("GET", kubernetesPlaneResourceURL, nil)
	response.EqualsFixture(200, kubernetesPlaneResponseFixture)
}

func Test_KubernetesPlane_LIST(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	// Add a kubernetes plane
	response := server.MakeFixtureRequest("PUT", kubernetesPlaneResourceURL, kubernetesPlaneRequestFixture)
	response.EqualsFixture(200, kubernetesPlaneResponseFixture)

	// Verify that /planes/kubernetes URL returns planes only with the kubernetes plane type.
	response = server.MakeRequest("GET", kubernetesPlaneCollectionURL, nil)
	response.EqualsFixture(200, kubernetesPlaneListResponseFixture)
}

func Test_KubernetesPlane_DELETE_DoesNotExist(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	response := server.MakeRequest("DELETE", kubernetesPlaneResourceURL, nil)
	response.EqualsResponse(204, nil)
}

func Test_KubernetesPlane_DELETE_Found(t *testing.T) {
	server := testserver.StartWithETCD(t, configureKubernetesModule)
	defer server.Close()

	response := server.MakeFixtureRequest("PUT", kubernetesPlaneResourceURL, kubernetesPlaneRequestFixture)
	response.EqualsFixture(200, kubernetesPlaneResponseFixture)

	response = server.MakeRequest("DELETE", kubernetesPlaneResourceURL, nil)
	response.EqualsResponse(200, nil)
}
