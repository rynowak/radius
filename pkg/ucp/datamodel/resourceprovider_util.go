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

package datamodel

import "github.com/radius-project/radius/pkg/ucp/resources"

// ResourceProviderIDFromResourceID converts an inbound resource id to the resource ID
// of the resource provider.
func ResourceProviderIDFromResourceID(id resources.ID) (resources.ID, error) {
	// Ex:
	// /planes/radius/local/providers/Applications.Test/testResources/foo
	// => /planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test
	return resources.ParseResource(
		id.PlaneScope() +
			resources.SegmentSeparator + resources.ProvidersSegment +
			resources.SegmentSeparator + ResourceProviderResourceType +
			resources.SegmentSeparator + id.ProviderNamespace())
}
