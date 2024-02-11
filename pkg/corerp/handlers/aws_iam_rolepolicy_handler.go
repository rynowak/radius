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
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/radius-project/radius/pkg/sdk"
)

type AWSIAMRolePolicyHandler struct {
	Connection sdk.Connection
}

func (handler *AWSIAMRolePolicyHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	// TODO: Load AWS credentials from UCP.
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	cfg.Region = options.Resource.ID.FindScope("regions")

	input := options.Resource.CreateResource.Data.(*iam.AttachRolePolicyInput)
	client := iam.NewFromConfig(cfg)

	_, err = client.AttachRolePolicy(ctx, input)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *AWSIAMRolePolicyHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
