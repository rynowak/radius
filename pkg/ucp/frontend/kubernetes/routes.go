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

package kubernetes

import (
	"context"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	planes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/planes"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	planeCollectionPath = "/planes/kubernetes"
	planeResourcePath   = "/planes/kubernetes/{planeName}"
)

func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	baseRouter := server.NewSubrouter(m.router, m.options.PathBase)

	apiValidator := validator.APIValidator(validator.Options{
		SpecLoader:         m.options.SpecLoader,
		ResourceTypeGetter: validator.UCPResourceTypeGetter,
	})

	planeResourceOptions := controller.ResourceOptions[datamodel.KubernetesPlane]{
		RequestConverter:  converter.KubernetesPlaneDataModelFromVersioned,
		ResponseConverter: converter.KubernetesPlaneDataModelToVersioned,
	}

	// URLs for lifecycle of planes
	planeResourceType := "System.Planes/kubernetes"
	planeCollectionRouter := server.NewSubrouter(baseRouter, planeCollectionPath, apiValidator)
	planeResourceRouter := server.NewSubrouter(baseRouter, planeResourcePath, apiValidator)

	handlerOptions := []server.HandlerOptions{
		{
			// This is a scope query so we can't use the default operation.
			ParentRouter:  planeCollectionRouter,
			Method:        v1.OperationList,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationList},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return &planes_ctrl.ListPlanesByType[*datamodel.KubernetesPlane, datamodel.KubernetesPlane]{
					Operation: controller.NewOperation(opts, planeResourceOptions),
				}, nil
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationGet,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationGet},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationPut,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationPut},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationDelete,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationDelete},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opts, planeResourceOptions)
			},
		},
	}

	ctrlOptions := controller.Options{
		Address:       m.options.Address,
		PathBase:      m.options.PathBase,
		DataProvider:  m.options.DataProvider,
		StatusManager: m.options.StatusManager,
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOptions); err != nil {
			return nil, err
		}
	}

	return m.router, nil
}
