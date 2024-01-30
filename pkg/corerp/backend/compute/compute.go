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

package compute

import "context"

// Deployment represents the desired state of a compute resource deployment.
type Deployment struct {
	// Compute contains the compute configuration for the deployment.
	Compute CoreResource

	// Identity contains the identity configuration for the deployment.
	Identity Component

	// Network contains the network configuration for the deployment.
	Network Component

	// Storage contains the storage configuration for the deployment.
	Secrets Component

	// Storage contains the storage configuration for the deployment.
	Storage Component
}

// Deployable represents a component that can be deployed.
type Deployable interface {
	// Deploy deploys the component.
	Deploy(ctx context.Context) error
}

// CoreResource represents a deployable compute resource. The "core" resource such as a VM or container.
type CoreResource interface {
	Deployable
}

// Component represents a deployable component of a compute resource deployment.
//
// After the component is deployed, it will be given a chance to modify the core compute resource.
type Component interface {
	Deployable

	// Update updates the core compute resource.
	Update(resource CoreResource) error
}
