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

package watcher

import (
	"context"
	"sync"

	daprclient "github.com/dapr/go-sdk/client"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
)

// Service is a service to watch resources in the background.
type Service struct {
	// Options is the host options for the service.
	Options hostoptions.HostOptions

	kubernetes kubernetesWatcher
}

// Name returns the service name.
func (w *Service) Name() string {
	return "UCP resource watcher"
}

// Run starts the service.
func (w *Service) Run(ctx context.Context) error {
	var dapr daprclient.Client
	var err error

	if w.Options.Config.Dapr.GRPCPort != 0 {
		dapr, err = daprclient.NewClient()
		if err != nil {
			return err
		}
	}

	w.kubernetes = kubernetesWatcher{
		restConfig: w.Options.K8sConfig,
		mutex:      &sync.Mutex{},
		dapr:       dapr,
	}

	// Run until cancelled.
	if dapr != nil {
		w.kubernetes.Run(ctx)
	}

	return nil
}
