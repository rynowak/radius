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
	"errors"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/radius-project/radius/pkg/sdk"
)

type AWSIAMRoleHandler struct {
	Connection sdk.Connection
}

func (handler *AWSIAMRoleHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	// TODO: Load AWS credentials from UCP.
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	cfg.Region = options.Resource.ID.FindScope("regions")

	input := options.Resource.CreateResource.Data.(*iam.CreateRoleInput)
	client := iam.NewFromConfig(cfg)

	exists, err := handler.exists(ctx, client, input)
	if err != nil {
		return nil, err
	}

	if exists {
		return handler.update(ctx, client, input)
	}

	return handler.create(ctx, client, input)
}

func (handler *AWSIAMRoleHandler) exists(ctx context.Context, client *iam.Client, input *iam.CreateRoleInput) (bool, error) {
	var notFound *iamtypes.NoSuchEntityException
	_, err := client.GetRole(ctx, &iam.GetRoleInput{RoleName: input.RoleName})
	if errors.As(err, &notFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func (handler *AWSIAMRoleHandler) create(ctx context.Context, client *iam.Client, input *iam.CreateRoleInput) (map[string]string, error) {
	_, err := client.CreateRole(ctx, input)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *AWSIAMRoleHandler) update(ctx context.Context, client *iam.Client, input *iam.CreateRoleInput) (map[string]string, error) {
	updateInput := &iam.UpdateRoleInput{
		RoleName:           input.RoleName,
		Description:        input.Description,
		MaxSessionDuration: input.MaxSessionDuration,
	}

	_, err := client.UpdateRole(ctx, updateInput)
	if err != nil {
		return nil, err
	}

	return map[string]string{}, nil
}

func (handler *AWSIAMRoleHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
