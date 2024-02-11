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

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
)

type AWSECSServiceHandler struct {
	Connection sdk.Connection
}

func (handler *AWSECSServiceHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	// TODO: Load AWS credentials from UCP.
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	cfg.Region = options.Resource.ID.FindScope("regions")

	input := options.Resource.CreateResource.Data.(*ecs.CreateServiceInput)
	client := ecs.NewFromConfig(cfg)

	exists, existing, err := handler.serviceExists(ctx, client, input)
	if err != nil {
		return nil, err
	}

	if exists {
		return handler.update(ctx, client, input, existing, options.DependencyProperties)
	}

	// Service does not exist, create it.
	return handler.create(ctx, client, input, options.DependencyProperties)
}

func (handler *AWSECSServiceHandler) serviceExists(ctx context.Context, client *ecs.Client, input *ecs.CreateServiceInput) (bool, *ecstypes.Service, error) {
	services, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Services: []string{*input.ServiceName}, Cluster: input.Cluster})
	if err != nil {
		return false, nil, err
	}

	if len(services.Services) == 0 {
		return false, nil, nil
	}

	return true, &services.Services[0], nil
}

func (handler *AWSECSServiceHandler) create(ctx context.Context, client *ecs.Client, input *ecs.CreateServiceInput, dependencies map[string]map[string]string) (map[string]string, error) {
	serviceArn := ""
	sdService, ok := dependencies["ServiceDiscoveryService"]
	if ok {
		serviceArn = sdService["ARN"]
	}

	for i := range input.ServiceRegistries {
		input.ServiceRegistries[i].RegistryArn = to.Ptr(serviceArn)
	}

	_, err := client.CreateService(ctx, input)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *AWSECSServiceHandler) update(ctx context.Context, client *ecs.Client, input *ecs.CreateServiceInput, existing *ecstypes.Service, dependencies map[string]map[string]string) (map[string]string, error) {

	serviceArn := ""
	sdService, ok := dependencies["ServiceDiscoveryService"]
	if ok {
		serviceArn = sdService["ARN"]
	}

	for i := range input.ServiceRegistries {
		input.ServiceRegistries[i].RegistryArn = to.Ptr(serviceArn)
	}

	updateInput := &ecs.UpdateServiceInput{
		Service:                       input.ServiceName,
		CapacityProviderStrategy:      input.CapacityProviderStrategy,
		Cluster:                       input.Cluster,
		DeploymentConfiguration:       input.DeploymentConfiguration,
		DesiredCount:                  input.DesiredCount,
		EnableECSManagedTags:          to.Ptr(input.EnableECSManagedTags),
		EnableExecuteCommand:          to.Ptr(input.EnableExecuteCommand),
		ForceNewDeployment:            false,
		HealthCheckGracePeriodSeconds: input.HealthCheckGracePeriodSeconds,
		LoadBalancers:                 input.LoadBalancers,
		NetworkConfiguration:          input.NetworkConfiguration,
		PlacementConstraints:          input.PlacementConstraints,
		PlacementStrategy:             input.PlacementStrategy,
		PlatformVersion:               input.PlatformVersion,
		PropagateTags:                 input.PropagateTags,
		ServiceConnectConfiguration:   input.ServiceConnectConfiguration,
		ServiceRegistries:             input.ServiceRegistries,
		TaskDefinition:                input.TaskDefinition,
		VolumeConfigurations:          input.VolumeConfigurations,
	}

	_, err := client.UpdateService(ctx, updateInput)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *AWSECSServiceHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
