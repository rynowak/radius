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

package worker

import (
	"context"

	manager "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	queue "github.com/radius-project/radius/pkg/ucp/queue/client"
	qprovider "github.com/radius-project/radius/pkg/ucp/queue/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Service is the base worker service implementation to initialize and start worker.
type Service struct {
	// ProviderName is the name of provider namespace.
	ProviderName string
	// Config is the configuration. This will be provided to controllers executed by the worker.
	Config *hostoptions.ProviderConfig
	// QueueProvider is the queue provider. Will be initialized from config if not provided.
	QueueProvider *qprovider.QueueProvider
	// StorageProvider is the provider of storage client. Will be initialized from config if not provided.
	StorageProvider dataprovider.DataStorageProvider
	// OperationStatusManager is the manager of the operation status.
	OperationStatusManager manager.StatusManager
	// Controllers is the registry of the async operation controllers.
	Controllers *ControllerRegistry
	// RequestQueue is the queue client for async operation request message.
	RequestQueue queue.Client
}

// Init initializes worker service - it initializes the StorageProvider, RequestQueue, OperationStatusManager, Controllers, KubeClient and
// returns an error if any of these operations fail.
func (s *Service) Init(ctx context.Context) error {
	if s.StorageProvider == nil {
		s.StorageProvider = dataprovider.NewStorageProvider(s.Config.StorageProvider)
	}

	if s.QueueProvider == nil {
		s.QueueProvider = qprovider.New(s.Config.QueueProvider)
	}

	var err error
	s.RequestQueue, err = s.QueueProvider.GetClient(ctx)
	if err != nil {
		return err
	}
	s.OperationStatusManager = manager.New(s.StorageProvider, s.RequestQueue, s.Config.Env.RoleLocation)
	s.Controllers = NewControllerRegistry(s.StorageProvider)
	return nil
}

// Start creates and starts a worker, and logs any errors that occur while starting the worker.
func (s *Service) Start(ctx context.Context, opt Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	ctx = hostoptions.WithContext(ctx, s.Config)

	// Create and start worker.
	worker := New(opt, s.OperationStatusManager, s.RequestQueue, s.Controllers)

	logger.Info("Start Worker...")
	if err := worker.Start(ctx); err != nil {
		logger.Error(err, "failed to start worker...")
		return err
	}

	logger.Info("Worker stopped...")
	return nil
}
