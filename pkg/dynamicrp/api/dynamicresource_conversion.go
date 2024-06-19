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

package api

import (
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/to"
)

const (
	// TODO
	Version = "2023-01-01"
)

func (d *DynamicResource) ConvertTo() (v1.DataModelInterface, error) {
	dm := &datamodel.DynamicResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(d.ID),
				Name:     to.String(d.Name),
				Type:     to.String(d.Type),
				Location: to.String(d.Location),
				Tags:     to.StringMap(d.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
		Properties: d.Properties,
	}

	return dm, nil
}

func (d *DynamicResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.DynamicResource)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	d.ID = &dm.ID
	d.Name = &dm.Name
	d.Type = &dm.Type
	d.Location = &dm.Location
	d.Tags = *to.StringMapPtr(dm.Tags)
	d.SystemData = fromSystemDataDataModel(dm.SystemData)
	d.Properties = dm.Properties
	if d.Properties == nil {
		d.Properties = map[string]any{}
	}
	d.Properties["provisioningState"] = fromProvisioningStateDataModel(dm.AsyncProvisioningState)

	return nil
}

func fromSystemDataDataModel(input v1.SystemData) map[string]any {
	bs, err := json.Marshal(input)
	if err != nil {
		// This should never fail. We've designed the SystemData type to be serializable.
		panic("marshalling system data failed: " + err.Error())
	}

	result := map[string]any{}
	err = json.Unmarshal(bs, &result)
	if err != nil {
		// This should never fail. We've designed the SystemData type to be serializable.
		panic("unmarshalling system data failed: " + err.Error())
	}

	return result
}

func fromProvisioningStateDataModel(input v1.ProvisioningState) string {
	if input == "" {
		return string(v1.ProvisioningStateSucceeded)
	}

	return string(input)
}
