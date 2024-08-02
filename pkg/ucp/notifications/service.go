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

	"github.com/dapr/go-sdk/service/common"
	daprd "github.com/dapr/go-sdk/service/http"
	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Service is a service to listen to resource notifications in the background.
type Service struct {
	// Options is the host options for the service.
	Options     hostoptions.HostOptions
	ServiceName string

	logger logr.Logger
	filter Filter
}

// Name returns the service name.
func (w *Service) Name() string {
	return w.ServiceName
}

// Run starts the service.
func (w *Service) Run(ctx context.Context) error {
	w.logger = ucplog.FromContextOrDiscard(ctx)

	w.filter = &DeclarativeFilter{
		ucp:   w.Options.UCPConnection,
		data:  w.Options.Config.StorageProvider,
		queue: w.Options.Config.QueueProvider,
	}

	service := daprd.NewService(":7009")
	subscription := common.Subscription{
		PubsubName: "pubsub",
		Topic:      "ucp-notifications",
		Route:      "/ucp-notifications",
	}
	err := service.AddTopicEventHandler(&subscription, w.eventHandler)
	if err != nil {
		return fmt.Errorf("failed to add topic event handler: %w", err)
	}

	errChan := make(chan error)
	go func() {
		err = service.Start()
		if err != nil {
			errChan <- fmt.Errorf("failed to start service: %w", err)
			return
		}

		errChan <- nil
	}()

	// Wait for shutdown.
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Fallthrough and continue with graceful stop.
	}

	err = service.GracefulStop()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Drain the error channel to prevent goroutine leak.
	<-errChan

	return nil
}

func (w *Service) eventHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	w.logger.Info("Received event", "event", e)

	if w.filter == nil {
		return false, nil
	}

	n := Notification{}
	err = json.Unmarshal(e.RawData, &n)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	err = w.filter.Send(ctx, n)
	if err != nil {
		return true, fmt.Errorf("failed to send notification: %w", err)
	}

	w.logger.Info("Delivered notification", "event", e)

	return false, nil
}
