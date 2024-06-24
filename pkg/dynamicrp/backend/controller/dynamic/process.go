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
	"errors"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/portableresources/backend/controller"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*Controller)(nil)

// Controller is the async operation controller to perform background processing on tracked resources.
type Controller struct {
	ctrl.BaseController

	opts         ctrl.Options
	engine       engine.Engine
	client       processors.ResourceClient
	configLoader configloader.ConfigurationLoader

	apiVersionsClient apiVersionsClient
}

// NewController creates a new Controller controller which is used to process resources asynchronously.
func NewController(
	opts ctrl.Options,
	engine engine.Engine,
	client processors.ResourceClient,
	configLoader configloader.ConfigurationLoader,
	ucp sdk.Connection) (ctrl.Controller, error) {

	factory, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(ucp))
	if err != nil {
		return nil, err
	}

	return &Controller{
		BaseController: ctrl.NewBaseAsyncController(opts),

		opts:              opts,
		engine:            engine,
		client:            client,
		configLoader:      configLoader,
		apiVersionsClient: factory.NewAPIVersionsClient(),
	}, nil
}

// Run implements the async operation controller to process resources asynchronously.
func (c *Controller) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	apiVersion, err := c.validateResourceType(ctx, id, request.APIVersion, v1.LocationGlobal)
	if errors.Is(err, &InvalidResourceTypeError{}) {
		e := v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: err.Error(),
			Target:  request.ResourceID,
		}
		return ctrl.NewFailedResult(e), nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	operationType, _ := v1.ParseOperationType(request.OperationType)
	switch operationType.Method {
	case http.MethodPut:
		return c.processPut(ctx, request, apiVersion)
	case http.MethodDelete:
		return c.processDelete(ctx, request, apiVersion)
	default:
		e := v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("Invalid operation type: %q", operationType),
			Target:  request.ResourceID,
		}
		return ctrl.NewFailedResult(e), nil
	}
}

func (c *Controller) validateResourceType(ctx context.Context, id resources.ID, apiVersion string, location string) (*v20231001preview.APIVersionResource, error) {
	response, err := c.apiVersionsClient.Get(
		ctx,
		id.FindScope("radius"),
		id.ProviderNamespace(),
		strings.TrimPrefix(id.Type(),
			id.ProviderNamespace()+resources.SegmentSeparator),
		apiVersion,
		nil)
	if err != nil {
		return nil, err
	}

	return &response.APIVersionResource, nil
}

func (c *Controller) processDelete(ctx context.Context, request *ctrl.Request, resourceType *v20231001preview.APIVersionResource) (ctrl.Result, error) {
	p := &dynamicProcessor{
		APIVersion: resourceType,
	}

	inner, err := controller.NewDeleteResource(c.opts, p, c.engine, c.configLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err := inner.Run(ctx, request)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
}

func (c *Controller) processPut(ctx context.Context, request *ctrl.Request, resourceType *v20231001preview.APIVersionResource) (ctrl.Result, error) {
	p := &dynamicProcessor{
		APIVersion: resourceType,
	}

	inner, err := controller.NewCreateOrUpdateResource(c.opts, p, c.engine, c.client, c.configLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	result, err := inner.Run(ctx, request)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
}
