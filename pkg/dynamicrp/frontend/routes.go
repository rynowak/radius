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
	"time"

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

	pathBase := s.options.Config.Server.PathBase
	if pathBase == "" {
		pathBase = "/"
	}
	r.Route(pathBase, func(r chi.Router) {
		r.Route("/planes/radius/{planeName}", func(r chi.Router) {
			r.Route("/providers/{providerNamespace}", func(r chi.Router) {
				register(r, "GET /{resourceType}", v1.OperationPlaneScopeList, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
					resourceOpts.ListRecursiveQuery = true
					return defaultoperation.NewListResources[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
				})

				r.Route("/locations/{locationName}", func(r chi.Router) {
					r.Get("/{or:operation[Rr]esults}/{operationID}", dynamicOperationHandler(v1.OperationGet, ctrlOpts, func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
						return defaultoperation.NewGetOperationResult(opts)
					}))
					r.Get("/{os:operation[Ss]tatuses}/{operationID}", dynamicOperationHandler(v1.OperationGet, ctrlOpts, func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
						return defaultoperation.NewGetOperationStatus(opts)
					}))
				})
			})

			r.Route("/{rg:resource[gG]roups}/{resourceGroupName}/providers/{providerNamespace}/{resourceType}", func(r chi.Router) {
				register(r, "GET /", v1.OperationList, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
					return defaultoperation.NewListResources[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
				})

				r.Route("/{resourceName}", func(r chi.Router) {
					register(r, "GET /", v1.OperationGet, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
						return defaultoperation.NewGetResource[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
					})

					register(r, "PUT /", v1.OperationPut, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
						resourceOpts.AsyncOperationTimeout = 24 * time.Hour
						resourceOpts.AsyncOperationRetryAfter = 5 * time.Second
						return defaultoperation.NewDefaultAsyncPut[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
					})

					register(r, "DELETE /", v1.OperationDelete, ctrlOpts, func(ctrlOpts controller.Options, resourceOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error) {
						resourceOpts.AsyncOperationTimeout = 24 * time.Hour
						resourceOpts.AsyncOperationRetryAfter = 5 * time.Second
						return defaultoperation.NewDefaultAsyncDelete[*datamodel.DynamicResource, datamodel.DynamicResource](ctrlOpts, resourceOpts)
					})
				})
			})
		})

	})

	return nil
}

type controllerFactory = func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error)

func register(r chi.Router, pattern string, method v1.OperationMethod, opts controller.Options, factory controllerFactory) {
	r.Handle(pattern, dynamicOperationHandler(method, opts, factory))
}

func dynamicOperationHandler(method v1.OperationMethod, opts controller.Options, factory func(opts controller.Options, ctrlOpts controller.ResourceOptions[datamodel.DynamicResource]) (controller.Controller, error)) http.HandlerFunc {
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

		// Special case the operation status and operation result types.
		//
		// This is special-casing that all of our resource providers do to store a single data row for both operation statuses and operation results.
		if strings.HasSuffix(strings.ToLower(opts.ResourceType), "locations/operationstatuses") || strings.HasSuffix(strings.ToLower(opts.ResourceType), "locations/operationresults") {
			opts.ResourceType = id.ProviderNamespace() + "/operationstatuses"
		}

		client, err := opts.DataProvider.GetStorageClient(r.Context(), opts.ResourceType)
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
