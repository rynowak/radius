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

package handlers

import (
	"context"
	"encoding/json"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

type AWSHandler struct {
	Connection sdk.Connection
}

func (handler *AWSHandler) createClient() (*armresources.Client, error) {
	opts := &clientv2.Options{
		Cred:    &tokencredentials.AnonymousCredential{},
		BaseURI: handler.Connection.Endpoint(),
	}

	client, err := clientv2.NewGenericResourceClient(uuid.NewString(), opts, sdk.NewClientOptions(handler.Connection))
	if err != nil {
		return nil, err
	}

	return client, nil
}

// Put validates that the resource exists. It returns an error if the resource does not exist.
func (handler *AWSHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	client, err := handler.createClient()
	if err != nil {
		return nil, err
	}

	resource := armresources.GenericResource{
		Name:       to.Ptr(options.Resource.ID.Name()),
		Properties: options.Resource.CreateResource.Data,
	}

	b, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Creating resource", "resource", string(b))

	poller, err := client.BeginCreateOrUpdateByID(ctx, options.Resource.ID.String(), "default", resource, nil)
	if err != nil {
		return nil, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

// No-op - just returns nil.
func (handler *AWSHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
