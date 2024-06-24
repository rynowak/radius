package dynamic

import (
	"context"

	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// We define interfaces so we can mock the interactions with the Radius API. These
// mock interfaces describe the APIs called by the dynamicrp.
//
// These interfaces match the generated API clients so that we can use them to mock
// the generated clients in our tests.
//
// Because these interfaces are non-exported, they MUST be defined in their own file
// and we MUST use -source on mockgen to generate mocks for them.

//go:generate mockgen -typed -source=./clients.go -destination=./mock_clients.go -package=dynamic -self_package github.com/radius-project/radius/pkg/dynamicrp/backend/controller/dynamic apiVersionsClient

type apiVersionsClient interface {
	Get(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (v20231001preview.APIVersionsClientGetResponse, error)
}
