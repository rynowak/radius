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

import "@typespec/rest";
import "@typespec/versioning";
import "@typespec/openapi";
import "@azure-tools/typespec-autorest";
import "@azure-tools/typespec-azure-core";
import "@azure-tools/typespec-azure-resource-manager";
import "@azure-tools/typespec-providerhub";

import "../radius/v1/ucprootscope.tsp";
import "../radius/v1/resources.tsp";
import "../radius/v1/trackedresource.tsp";
import "./common.tsp";
import "./planes.tsp";
import "./ucp-operations.tsp";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Versioning;
using Autorest;
using Azure.Core;
using Azure.ResourceManager;
using Azure.ResourceManager.Foundations;
using OpenAPI;

namespace Ucp;

@doc("The Kubernetes plane resource.")
model KubernetesPlaneResource
    is TrackedResourceRequired<
        KubernetesPlaneResourceProperties,
        "System.Planes/kubernetes",
        "kubernetes"
    > {
    @doc("The plane name.")
    @segment("kubernetes")
    @path
    @key("planeName")
    name: ResourceNameString;
}

@doc("The Plane properties.")
model KubernetesPlaneResourceProperties {
    @doc("The status of the asynchronous operation.")
    @visibility("read")
    provisioningState?: ProvisioningState;

    @doc("The fully-qualified URL of the API server. The provided URL must be reachable from UCP's network.")
    server: string;

    @doc("The base64-encoded certificate authority data.")
    certificateAuthorityData: string;

    @visibility("create", "update")
    @extension("x-ms-secret", true)
    @doc("The configuration used to authenticate with the cluster.")
    auth: KubernetesAuthenticationConfiguration;
}

@doc("The configuration for authentication with the Kubernetes cluster.")
@discriminator("kind")
model KubernetesAuthenticationConfiguration {
}

@doc("The configuration for authenticating with the Kubernetes cluster where Radius is deployed.")

model KubernetesInClusterConfiguration extends KubernetesAuthenticationConfiguration {
    @doc("The kind of authentication configuration.")
    kind: "InCluster";
}

@doc("The configuration for authenticating with the Kubernetes cluster using a long-lived service account token.")
model KubernetesServiceAccountTokenConfiguration extends KubernetesAuthenticationConfiguration {
    @doc("The kind of authentication configuration.")
    kind: "ServiceAccountToken";

    @visibility("create", "update")
    @extension("x-ms-secret", true)
    @doc("The base64-encoded token used to authenticate with the cluster.")
    tokenData: string;
} 

@route("/planes")
@armResourceOperations
interface KubernetesPlanes {
    @doc("List kubernetes planes")
    @get
    @route("/kubernetes")
    @armResourceList(KubernetesPlaneResource)
    list(
        ...ApiVersionParameter,
    ): ArmResponse<ResourceListResult<KubernetesPlaneResource>> | ErrorResponse;

    @doc("Get a plane by name")
    get is UcpResourceRead<
        KubernetesPlaneResource,
        PlaneBaseParameters<KubernetesPlaneResource>
    >;

    @doc("Create or update a plane")
    createOrUpdate is UcpResourceCreateOrUpdateAsync<
        KubernetesPlaneResource,
        PlaneBaseParameters<KubernetesPlaneResource>
    >;

    @doc("Update a plane")
    update is UcpCustomPatchAsync<
        KubernetesPlaneResource,
        PlaneBaseParameters<KubernetesPlaneResource>
    >;

    @doc("Delete a plane")
    delete is UcpResourceDeleteAsync<
        KubernetesPlaneResource,
        PlaneBaseParameters<KubernetesPlaneResource>
    >;
}
