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

package backend

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/controller/dynamic"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
)

// Service runs the backend for the dynamic-rp.
type Service struct {
	worker.Service
	options *dynamicrp.Options
	recipes *controllerconfig.RecipeControllerConfig
}

// NewService creates a new service to run the dynamic-rp backend.
func NewService(options *dynamicrp.Options) *Service {
	return &Service{
		options: options,
		Service: worker.Service{
			ProviderName: "dynamic-rp",
			Config: &hostoptions.ProviderConfig{
				Bicep:           options.Config.Bicep,
				Env:             options.Config.Environment,
				Logging:         options.Config.Logging,
				SecretProvider:  options.Config.Secrets,
				QueueProvider:   options.Config.Queue,
				StorageProvider: options.Config.Storage,
				Terraform:       options.Config.Terraform,
				WorkerServer:    &options.Config.Worker,
			},

			OperationStatusManager: options.StatusManager,
			QueueProvider:          options.QueueProvider,
			StorageProvider:        options.StorageProvider,
		},
		recipes: options.Recipes,
	}
}

// Name returns the name of the service used for logging.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", w.Service.ProviderName)
}

// Run runs the service.
func (w *Service) Run(ctx context.Context) error {
	err := w.Init(ctx)
	if err != nil {
		return err
	}

	workerOptions := worker.Options{}
	if w.options.Config.Worker.MaxOperationConcurrency != nil {
		workerOptions.MaxOperationConcurrency = *w.options.Config.Worker.MaxOperationConcurrency
	}
	if w.options.Config.Worker.MaxOperationRetryCount != nil {
		workerOptions.MaxOperationRetryCount = *w.options.Config.Worker.MaxOperationRetryCount
	}

	err = w.registerControllers(ctx)
	if err != nil {
		return err
	}

	return w.Start(ctx, workerOptions)
}

func (w *Service) registerControllers(ctx context.Context) error {
	options := ctrl.Options{
		DataProvider: w.StorageProvider,
	}

	// Register a single controller to handle all resource types.
	err := w.Controllers.Register(ctx, worker.ResourceTypeAny, v1.OperationMethod(worker.OperationMethodAny), func(options ctrl.Options) (ctrl.Controller, error) {
		return dynamic.NewController(options, w.recipes.Engine, w.recipes.ResourceClient, w.recipes.ConfigLoader, *w.recipes.UCPConnection)
	}, options)
	if err != nil {
		return err
	}

	return nil
}
