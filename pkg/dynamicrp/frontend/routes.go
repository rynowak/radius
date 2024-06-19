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

package frontend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/api"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	operationBaseRoute                 = "/planes/radius/{planeName}/providers/{providerNamespace}/locations/{locationName}"
	planeScopedResourceCollectionRoute = "/planes/radius/{planeName}/providers/{providerNamespace}/{resourceType}"
	resourceCollectionRoute            = "/planes/radius/{planeName}/{rg:resource[gG]roups}/{resourceGroupName}/providers/{providerNamespace}/{resourceType}"
	resourceRoute                      = resourceCollectionRoute + "/{resourceName}"
)

func (s *Service) registerRoutes(r *chi.Mux) error {
	ctrlOpts := controller.Options{
		Address:       fmt.Sprintf("%s:%d", s.options.Config.Server.Host, s.options.Config.Server.Port),
		PathBase:      s.options.Config.Server.PathBase,
		DataProvider:  s.options.StorageProvider,
		StatusManager: s.options.StatusManager,

		KubeClient:    nil, // Unused by DynamicRP
		StorageClient: nil, // Set dynamically
		ResourceType:  "",  // Set dynamically
	}

	// Return ARM errors for invalid requests.
	r.NotFound(validator.APINotFoundHandler())
	r.MethodNotAllowed(validator.APIMethodNotAllowedHandler())

	r.Route(s.options.Config.Server.PathBase, func(r chi.Router) {
		register(r, "GET "+operationBaseRoute+"/operationResults/{operationID}", v1.OperationGet, ctrlOpts, func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			return defaultoperation.NewGetOperationResult(opts)
		})

		register(r, "GET "+operationBaseRoute+"/operationStatuses/{operationID}", v1.OperationGet, ctrlOpts, func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			return defaultoperation.NewGetOperationStatus(opts)
		})

		register(r, "GET "+planeScopedResourceCollectionRoute, v1.OperationPlaneScopeList, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			resourceOpts.ListRecursiveQuery = true
			return defaultoperation.NewListResources[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
		})

		register(r, "GET "+resourceCollectionRoute, v1.OperationList, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			return defaultoperation.NewListResources[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
		})

		register(r, "GET "+resourceRoute, v1.OperationGet, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			return defaultoperation.NewGetResource[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
		})

		register(r, "PUT "+resourceRoute, v1.OperationPut, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			return defaultoperation.NewDefaultSyncPut[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
		})

		register(r, "DELETE "+resourceRoute, v1.OperationDelete, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
			return defaultoperation.NewDefaultSyncDelete[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
		})
	})

	return nil
}

type controllerFactory = func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error)

func register(r chi.Router, pattern string, method v1.OperationMethod, opts controller.Options, factory controllerFactory) {
	r.Handle(pattern, dynamicOperationType(method, opts, factory))
}

func dynamicOperationType(method v1.OperationMethod, opts controller.Options, factory func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := resources.Parse(r.URL.Path)
		if err != nil {
			result := rest.NewBadRequestResponse(err.Error())
			err = result.Apply(r.Context(), w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			return
		}

		operationType := v1.OperationType{Type: strings.ToUpper(id.Type()), Method: method}

		// Copy the options and initalize them dynamically for this type.
		opts := opts
		opts.ResourceType = id.Type()

		client, err := opts.DataProvider.GetStorageClient(r.Context(), id.Type())
		if err != nil {
			result := rest.NewBadRequestResponse(err.Error())
			err = result.Apply(r.Context(), w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			return
		}

		opts.StorageClient = client

		ctrlOpts := controller.ResourceOptions[datamodel.DynamicResource]{
			RequestConverter: func(content []byte, version string) (*datamodel.DynamicResource, error) {
				api := &api.DynamicResource{}

				err := json.Unmarshal(content, api)
				if err != nil {
					return nil, err
				}

				dm, err := api.ConvertTo()
				if err != nil {
					return nil, err
				}

				return dm.(*datamodel.DynamicResource), nil
			},
			ResponseConverter: func(resource *datamodel.DynamicResource, version string) (v1.VersionedModelInterface, error) {
				api := &api.DynamicResource{}
				err = api.ConvertFrom(resource)
				if err != nil {
					return nil, err
				}

				return api, nil
			},
		}

		ctrl, err := factory(opts, ctrlOpts)
		if err != nil {
			result := rest.NewBadRequestResponse(err.Error())
			err = result.Apply(r.Context(), w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			return
		}

		handler := server.HandlerForController(ctrl, operationType)
		handler.ServeHTTP(w, r)
	})
}
