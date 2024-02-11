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
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	servicediscoverytypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/radius-project/radius/pkg/sdk"
)

type AWSServiceDiscoveryServiceHandler struct {
	Connection sdk.Connection
}

func (handler *AWSServiceDiscoveryServiceHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	// TODO: Load AWS credentials from UCP.
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	cfg.Region = options.Resource.ID.FindScope("regions")

	input := options.Resource.CreateResource.Data.(*servicediscovery.CreateServiceInput)
	client := servicediscovery.NewFromConfig(cfg)

	exists, existing, err := handler.exists(ctx, client, input)
	if err != nil {
		return nil, err
	}

	if exists {
		return handler.update(ctx, client, input, existing)
	}

	return handler.create(ctx, client, input)
}

func (handler *AWSServiceDiscoveryServiceHandler) exists(ctx context.Context, client *servicediscovery.Client, input *servicediscovery.CreateServiceInput) (bool, *servicediscoverytypes.ServiceSummary, error) {
	params := &servicediscovery.ListServicesInput{
		Filters: []servicediscoverytypes.ServiceFilter{
			{
				Name:      "NAMESPACE_ID",
				Values:    []string{*input.NamespaceId},
				Condition: servicediscoverytypes.FilterConditionEq,
			},
		},
	}
	output, err := client.ListServices(ctx, params)
	if err != nil {
		return false, nil, err
	}

	for _, service := range output.Services {
		copy := service
		if *service.Name == *input.Name {
			return true, &copy, nil
		}

	}

	return false, nil, nil
}

func (handler *AWSServiceDiscoveryServiceHandler) create(ctx context.Context, client *servicediscovery.Client, input *servicediscovery.CreateServiceInput) (map[string]string, error) {
	output, err := client.CreateService(ctx, input)
	if err != nil {
		return nil, err
	}

	return map[string]string{"ID": *output.Service.Id, "ARN": *output.Service.Arn}, nil
}

func (handler *AWSServiceDiscoveryServiceHandler) update(ctx context.Context, client *servicediscovery.Client, input *servicediscovery.CreateServiceInput, existing *servicediscoverytypes.ServiceSummary) (map[string]string, error) {
	updateInput := &servicediscovery.UpdateServiceInput{
		Id: existing.Id,
		Service: &servicediscoverytypes.ServiceChange{
			Description: input.Description,
			DnsConfig: &servicediscoverytypes.DnsConfigChange{
				DnsRecords: input.DnsConfig.DnsRecords,
			},
			HealthCheckConfig: input.HealthCheckConfig,
		},
	}

	_, err := client.UpdateService(ctx, updateInput)
	if err != nil {
		return nil, err
	}

	return map[string]string{"ID": *existing.Id, "ARN": *existing.Arn}, nil
}

func (handler *AWSServiceDiscoveryServiceHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
