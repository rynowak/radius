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

package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	queue "github.com/radius-project/radius/pkg/ucp/queue/client"
	queueprovider "github.com/radius-project/radius/pkg/ucp/queue/provider"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
)

type Notification struct {
	ID     resources.ID `json:"id"`
	Reason Reason       `json:"reason"`
}

type Reason string

const (
	NotificationReasonUpdated Reason = "updated"
	NotificationReasonDeleted Reason = "deleted"
)

type Filter interface {
	Send(ctx context.Context, notification Notification) error
}

type DeclarativeFilter struct {
	UCP   sdk.Connection
	Data  dataprovider.StorageProviderOptions
	Queue queueprovider.QueueProviderOptions
}

func (f *DeclarativeFilter) Send(ctx context.Context, notification Notification) error {
	recipeTypes, err := f.recipeTypes(ctx)
	if err != nil {
		return err
	}

	for _, recipeType := range recipeTypes {
		impacted, err := f.impactedResources(ctx, recipeType, notification)
		if err != nil {
			return err
		}

		for _, resource := range impacted {
			err := f.notify(ctx, resource)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *DeclarativeFilter) recipeTypes(ctx context.Context) ([]string, error) {
	client, err := v20231001preview.NewResourceProvidersClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(f.UCP))
	if err != nil {
		return nil, err
	}

	results := []string{}

	pager := client.NewListPager("local", nil)
	for pager.More() {
		// response, err := pager.NextPage(ctx)
		// if err != nil {
		// 	return nil, err
		// }

		// for _, provider := range response.Value {
		// 	declarative := true
		// 	// for _, location := range provider.Properties.Locations {
		// 	// 	if location.Address == nil {
		// 	// 		// Not a declarative resource type.
		// 	// 		continue
		// 	// 	} else if *location.Address != "internal" {
		// 	// 		// Not a declarative resource type.
		// 	// 		continue
		// 	// 	} else {
		// 	// 		declarative = true
		// 	// 		break
		// 	// 	}
		// 	// }

		// 	// if !declarative {
		// 	// 	continue // Not a declarative resource type.
		// 	// }

		// 	// for _, resourceType := range provider.Properties.ResourceTypes {
		// 	// 	for _, capability := range resourceType.Capabilities {
		// 	// 		if *capability == "Recipe" {
		// 	// 			results = append(results, *provider.Name+"/"+*resourceType.ResourceType)
		// 	// 		}
		// 	// 	}
		// 	// }
		// }
	}

	return results, nil
}

func (f *DeclarativeFilter) impactedResources(ctx context.Context, resourceType string, notification Notification) ([]resources.ID, error) {
	client, err := generated.NewGenericResourcesClient("/planes/radius/local", resourceType, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(f.UCP))
	if err != nil {
		return nil, err
	}

	results := []resources.ID{}

	pager := client.NewListByRootScopePager(nil)
	for pager.More() {
		response, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, resource := range response.Value {
			infrastructure := f.infrastructure(resource)

			matched := false
			for _, id := range infrastructure {
				if strings.EqualFold(id.String(), notification.ID.String()) {
					results = append(results, resources.MustParse(*resource.ID))
					matched = true
				}

				// Avoid duplicates if a resource mentioned an infrastructure resource twice.
				if matched {
					break
				}
			}
		}
	}

	return results, nil
}

func (f *DeclarativeFilter) infrastructure(resource *generated.GenericResource) []resources.ID {
	// extract $properties.status.outputResources[*].id
	obj, ok := resource.Properties["status"]
	if !ok {
		return nil
	}

	status, ok := obj.(map[string]interface{})
	if !ok {
		return nil
	}

	obj, ok = status["outputResources"]
	if !ok {
		return nil
	}

	outputResources, ok := obj.([]interface{})
	if !ok {
		return nil
	}

	results := []resources.ID{}
	for _, outputResource := range outputResources {
		resource, ok := outputResource.(map[string]interface{})
		if !ok {
			continue
		}

		obj, ok = resource["id"]
		if !ok {
			continue
		}

		id, ok := obj.(string)
		if !ok {
			continue
		}

		results = append(results, resources.MustParse(id))
	}

	return results
}

func (f *DeclarativeFilter) notify(ctx context.Context, id resources.ID) error {
	storage, err := f.storageClient(ctx, id.Type())
	if err != nil {
		return err
	}

	obj, err := storage.Get(ctx, id.String(), nil)
	if err != nil {
		return err
	}

	ps := f.provisioningState(obj)
	if !ps.IsTerminal() {
		return fmt.Errorf("resource %s is not in a terminal state: %s", id, ps)
	}

	f.setProvisioningState(obj, v1.ProvisioningStateUpdating)

	err = storage.Save(ctx, obj, store.WithETag(obj.ETag))
	if err != nil {
		return err
	}

	options := statusmanager.QueueOperationOptions{
		OperationTimeout: time.Minute * 60, // TODO
		RetryAfter:       v1.DefaultRetryAfterDuration,
	}

	sCtx := v1.ARMRequestContext{
		ResourceID:    id,
		OperationID:   uuid.New(),
		OperationType: v1.OperationType{Type: strings.ToUpper(id.Type()), Method: "PUT"},
	}

	err = f.statusManager(ctx).QueueAsyncOperation(ctx, &sCtx, options)

	if err != nil {
		f.setProvisioningState(obj, v1.ProvisioningStateFailed)
		saveErr := storage.Save(ctx, obj, store.WithETag(obj.ETag))
		if saveErr != nil {
			return saveErr
		}

		return err
	}

	return nil
}

func (f *DeclarativeFilter) provisioningState(resource any) v1.ProvisioningState {
	b, err := json.Marshal(resource)
	if err != nil {
		panic(err)
	}

	data := map[string]any{}
	err = json.Unmarshal(b, &data)
	if err != nil {
		panic(err)
	}

	obj, ok := data["properties"]
	if !ok {
		obj = map[string]any{}
		data["properties"] = obj
	}

	properties, ok := obj.(map[string]any)
	if !ok {
		return v1.ProvisioningStateSucceeded
	}

	obj, ok = properties["provisioningState"]
	if !ok {
		return v1.ProvisioningStateSucceeded
	}

	ps, ok := obj.(string)
	if !ok {
		return v1.ProvisioningStateSucceeded
	}

	return v1.ProvisioningState(ps)
}

func (f *DeclarativeFilter) setProvisioningState(resource any, ps v1.ProvisioningState) {
	b, err := json.Marshal(resource)
	if err != nil {
		panic(err)
	}

	data := map[string]any{}
	err = json.Unmarshal(b, &data)
	if err != nil {
		panic(err)
	}

	obj, ok := data["properties"]
	if !ok {
		obj = map[string]any{}
		data["properties"] = obj
	}

	properties, ok := obj.(map[string]any)
	if !ok {
		return
	}

	properties["provisioningState"] = string(ps)
}

func (f *DeclarativeFilter) storageClient(ctx context.Context, resourceType string) (store.StorageClient, error) {
	return dataprovider.NewStorageProvider(f.Data).GetStorageClient(ctx, resourceType)
}

func (f *DeclarativeFilter) queueClient(ctx context.Context) (queue.Client, error) {
	return queueprovider.New(f.Queue).GetClient(ctx)
}

func (f *DeclarativeFilter) statusManager(ctx context.Context) statusmanager.StatusManager {
	queueClient, err := f.queueClient(ctx)
	if err != nil {
		return nil
	}

	return statusmanager.New(dataprovider.NewStorageProvider(f.Data), queueClient, "global")
}
