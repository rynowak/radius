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

package resourcegroups

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/store"
)

// NotFoundError is returned when a resource group or plane is not found.
type NotFoundError struct {
	Message string
}

// Error returns the error message.
func (e *NotFoundError) Error() string {
	return e.Message
}

// Is returns true if the error is a NotFoundError.
func (e *NotFoundError) Is(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}

// InvalidError is returned when the data is invalid.
type InvalidError struct {
	Message string
}

// Error returns the error message.
func (e *InvalidError) Error() string {
	return e.Message
}

// Is returns true if the error is a InvalidError.
func (e *InvalidError) Is(err error) bool {
	_, ok := err.(*InvalidError)
	return ok
}

// ValidateRadiusPlane validates that the plane specified in the id exists. Returns NotFoundError if the plane does not exist.
func ValidateRadiusPlane(ctx context.Context, client store.StorageClient, id resources.ID) (*datamodel.RadiusPlane, error) {
	planeID, err := resources.ParseScope(id.PlaneScope())
	if err != nil {
		// Not expected to happen.
		return nil, err
	}

	plane, err := store.GetResource[datamodel.RadiusPlane](ctx, client, planeID.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return nil, &NotFoundError{Message: fmt.Sprintf("plane %q not found", planeID.String())}
	} else if err != nil {
		return nil, fmt.Errorf("failed to find plane %q: %w", planeID.String(), err)
	}

	return plane, nil
}

// ValidateResourceGroup validates that the resource group specified in the id exists (if applicable).
// Returns NotFoundError if the resource group does not exist.
func ValidateResourceGroup(ctx context.Context, client store.StorageClient, id resources.ID) error {
	// If the ID contains a resource group, validate it now.
	if id.FindScope(resources_radius.ScopeResourceGroups) == "" {
		return nil
	}

	resourceGroupID, err := resources.ParseScope(id.RootScope())
	if err != nil {
		// Not expected to happen.
		return err
	}

	_, err = store.GetResource[datamodel.ResourceGroup](ctx, client, resourceGroupID.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return &NotFoundError{Message: fmt.Sprintf("resource group %q not found", resourceGroupID.String())}
	} else if err != nil {
		return fmt.Errorf("failed to find resource group %q: %w", resourceGroupID.String(), err)
	}

	return nil
}

// ValidateResourceType performs semantic validation of a proxy request against registered
// resource types.
//
// Returns NotFoundError if the resource type does not exist.
// Returns InvalidError if the request cannot be routed due to an invalid configuration.
func ValidateResourceType(ctx context.Context, client store.StorageClient, id resources.ID, locationName string, apiVersion string) (*url.URL, error) {
	// The strategy is to try and look up the location resource, and validate that it supports
	// the requested resource type and API version.

	providerID, err := datamodel.ResourceProviderIDFromResourceID(id)
	if err != nil {
		return nil, err
	}

	locationID := providerID.Append(resources.TypeSegment{Type: datamodel.LocationUnqualifiedResourceType, Name: locationName})
	location, err := store.GetResource[datamodel.Location](ctx, client, locationID.String())
	if errors.Is(err, &store.ErrNotFound{}) {

		// Return the error as-is to fallback to the legacy routing behavior.
		return nil, err

		// Uncomment this when we remove the legacy routing behavior.
		// return nil, &InvalidError{Message: fmt.Sprintf("location %q not found for resource provider %q", locationName, id.ProviderNamespace())}
	} else if err != nil {
		return nil, fmt.Errorf("failed to find location %q: %w", locationID.String(), err)
	}

	// Check if the location supports the resource type.
	// Resource types are case-intensitive so we have to iterate.
	var resourceType *datamodel.LocationResourceTypeConfiguration
	search := strings.TrimPrefix(strings.ToLower(id.Type()), strings.ToLower(id.ProviderNamespace())+resources.SegmentSeparator)
	for name, rt := range location.Properties.ResourceTypes {
		if strings.EqualFold(name, search) {
			copy := rt
			resourceType = &copy
			break
		}
	}

	if resourceType == nil {
		return nil, &InvalidError{Message: fmt.Sprintf("resource type %q not supported by location %q", id.Type(), locationName)}
	}

	// Now check if the location supports the resource type. If it does, we can return the downstream URL.
	_, ok := resourceType.APIVersions[apiVersion]
	if !ok {
		return nil, &InvalidError{Message: fmt.Sprintf("api version %q is not supported for resource type %q by location %q", apiVersion, id.Type(), locationName)}
	}

	// If we get here, the we're all good.
	//
	// The address might be nil which means that we're using the default address (dynamic RP)
	if location.Properties.Address == nil {
		return nil, nil
	}

	// If the address was provided, then use that instead.
	u, err := url.Parse(*location.Properties.Address)
	if err != nil {
		return nil, &InvalidError{Message: fmt.Sprintf("failed to parse location address: %v", err.Error())}
	}

	return u, nil
}

// ValidateLegacyResourceProvider validates that the resource provider specified in the id exists. Returns InvalidError if the plane
// contains invalid data.
func ValidateLegacyResourceProvider(ctx context.Context, client store.StorageClient, id resources.ID, plane *datamodel.RadiusPlane) (*url.URL, error) {
	downstream := plane.LookupResourceProvider(id.ProviderNamespace())
	if downstream == "" {
		return nil, &InvalidError{Message: fmt.Sprintf("resource provider %s not configured", id.ProviderNamespace())}
	}

	downstreamURL, err := url.Parse(downstream)
	if err != nil {
		return nil, &InvalidError{Message: fmt.Sprintf("failed to parse downstream URL: %v", err.Error())}
	}

	return downstreamURL, nil
}

// ValidateDownstream can be used to find and validate the downstream URL for a resource.
// Returns NotFoundError for the case where the plane or resource group does not exist.
// Returns InvalidError for cases where the data is invalid, like when the resource provider is not configured.
func ValidateDownstream(ctx context.Context, client store.StorageClient, id resources.ID, location string, apiVersion string) (*url.URL, error) {
	// There are a few steps to validation:
	//
	// - The plane exists
	// - The resource group exists
	// - The resource provider is configured
	// 		- As part of the plane (proxy routing)
	// 		- As part of a resource provider manifest (internal or proxy routing)
	//

	// The plane exists.
	plane, err := ValidateRadiusPlane(ctx, client, id)
	if err != nil {
		return nil, err
	}

	// The resource group exists (if applicable).
	err = ValidateResourceGroup(ctx, client, id)
	if err != nil {
		return nil, err
	}

	downstreamURL, err := ValidateResourceType(ctx, client, id, location, apiVersion)
	if errors.Is(err, &store.ErrNotFound{}) {
		// If the resource provider is not found, check if it is a legacy provider.
		downstreamURL, err := ValidateLegacyResourceProvider(ctx, client, id, plane)
		if err != nil {
			return nil, err
		}

		return downstreamURL, nil
	} else if err != nil {
		return nil, err
	}

	return downstreamURL, nil
}
