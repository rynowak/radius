// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/clients"
	k8s "k8s.io/client-go/kubernetes"
)

func RequireAzureCloud(e Environment) (*AzureCloudEnvironment, error) {
	az, ok := e.(*AzureCloudEnvironment)
	if !ok {
		return nil, fmt.Errorf("an '%v' environment is required but the kind was '%v'", KindAzureCloud, e.GetKind())
	}

	return az, nil
}

// AzureCloudEnvironment represents an Azure Cloud Radius environment.
type AzureCloudEnvironment struct {
	Name                      string `mapstructure:"name" validate:"required" yaml:",omitempty"`
	Kind                      string `mapstructure:"kind" validate:"required" yaml:",omitempty"`
	SubscriptionID            string `mapstructure:"subscriptionid" validate:"required" yaml:",omitempty"`
	ResourceGroup             string `mapstructure:"resourcegroup" validate:"required" yaml:",omitempty"`
	ControlPlaneResourceGroup string `mapstring:"controlplaneresourcegroup" validate:"required" yaml:",omitempty"`
	ClusterName               string `mapstructure:"clustername" validate:"required" yaml:",omitempty"`
	DefaultApplication        string `mapstructure:"defaultapplication" yaml:",omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain" yaml:",omitempty"`
}

func (e *AzureCloudEnvironment) GetName() string {
	return e.Name
}

func (e *AzureCloudEnvironment) GetKind() string {
	return e.Kind
}

func (e *AzureCloudEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *AzureCloudEnvironment) GetStatusLink() string {
	// If there's a problem generating the status link, we don't want to fail noisily, just skip the link.
	url, err := azure.GenerateAzureEnvUrl(e.SubscriptionID, e.ResourceGroup)
	if err != nil {
		return ""
	}

	return url
}

func (e *AzureCloudEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	dc := resources.NewDeploymentsClient(e.SubscriptionID)
	armauth, err := azure.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return nil, err
	}

	dc.Authorizer = armauth

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second

	// Don't timeout, let the user cancel
	dc.PollingDuration = 0

	return &azure.ARMDeploymentClient{
		Client:         dc,
		SubscriptionID: e.SubscriptionID,
		ResourceGroup:  e.ResourceGroup,
	}, nil
}

func (e *AzureCloudEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	config, err := azure.GetAKSMonitoringCredentials(ctx, e.SubscriptionID, e.ControlPlaneResourceGroup, e.ClusterName)
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &azure.ARMDiagnosticsClient{
		Client:     k8sClient,
		RestConfig: config,
	}, nil
}

func (e *AzureCloudEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a Azure credentials: %w", err)
	}

	con := armcore.NewDefaultConnection(azcred, nil)

	return &azure.ARMManagementClient{
		Connection:     con,
		ResourceGroup:  e.ResourceGroup,
		SubscriptionID: e.SubscriptionID}, nil
}
