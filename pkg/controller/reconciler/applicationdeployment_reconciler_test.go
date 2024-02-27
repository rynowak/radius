/*
Copyright 2023.

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

package reconciler

import (
	"errors"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	applicationDeploymentTestWaitDuration            = time.Second * 10
	applicationDeploymentTestWaitInterval            = time.Second * 1
	applicationDeploymentTestControllerDelayInterval = time.Millisecond * 100
)

func SetupApplicationDeploymentTest(t *testing.T) (*mockRadiusClient, client.Client) {
	SkipWithoutEnvironment(t)

	// For debugging, you can set uncomment this to see logs from the controller. This will cause tests to fail
	// because the logging will continue after the test completes.
	//
	// Add runtimelog "sigs.k8s.io/controller-runtime/pkg/log" to imports.
	//
	// runtimelog.SetLogger(ucplog.FromContextOrDiscard(testcontext.New(t)))

	// Shut down the manager when the test exits.
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,

		// Suppress metrics in tests to avoid conflicts.
		MetricsBindAddress: "0",
	})
	require.NoError(t, err)

	radius := NewMockRadiusClient()
	err = (&ApplicationDeploymentReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("applicationdeployment-controller"),
		Radius:        radius,
		DelayInterval: applicationDeploymentTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	return radius, mgr.GetClient()
}

func Test_ApplicationDeploymentReconciler_Basic(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupApplicationDeploymentTest(t)

	name := types.NamespacedName{Namespace: "applicationdeployment-basic", Name: "test-applicationdeployment"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeApplicationDeployment(name, map[string]any{})
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for template to complete provisioning.
	status := waitForApplicationDeploymentStateUpdating(t, client, name, nil)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-applicationdeployment-basic", status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default", status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-applicationdeployment-basic/providers/Applications.Core/applications/applicationdeployment-basic", status.Application)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	status = waitForApplicationDeploymentStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-applicationdeployment-basic/providers/Microsoft.Resources/deployments/test-applicationdeployment", status.Resource)

	resource, err := radius.Resources(status.Scope, "Microsoft.Resources/deployments").Get(ctx, name.Name)
	require.NoError(t, err)

	expectedProperties := map[string]any{
		"mode":       "Incremental",
		"parameters": map[string]map[string]any{},
		"providerConfig": map[string]any{
			"deployments": map[string]any{
				"type": "Microsoft.Resources",
				"value": map[string]any{
					"scope": "/planes/radius/local/resourcegroups/default-applicationdeployment-basic",
				},
			},
			"radius": map[string]any{
				"type": "Radius",
				"value": map[string]any{
					"scope": "/planes/radius/local/resourcegroups/default-applicationdeployment-basic",
				},
			},
		}, "template": map[string]any{},
	}
	require.Equal(t, expectedProperties, resource.Properties)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the ApplicationDeployment is in progress.
	status = waitForApplicationDeploymentStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the ApplicationDeployment object can complete.
	waitForApplicationDeploymentDeleted(t, client, name)
}

func Test_ApplicationDeploymentReconciler_ChangeEnvironmentAndApplication(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupApplicationDeploymentTest(t)

	name := types.NamespacedName{Namespace: "applicationdeployment-change", Name: "test-applicationdeployment-change"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeApplicationDeployment(name, map[string]any{})
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for template to complete provisioning.
	status := waitForApplicationDeploymentStateUpdating(t, client, name, nil)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-applicationdeployment-change", status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default", status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-applicationdeployment-change/providers/Applications.Core/applications/applicationdeployment-change", status.Application)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	status = waitForApplicationDeploymentStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-applicationdeployment-change/providers/Microsoft.Resources/deployments/test-applicationdeployment-change", status.Resource)

	_, err = radius.Resources(status.Scope, "Microsoft.Resources/deployments").Get(ctx, name.Name)
	require.NoError(t, err)

	createEnvironment(radius, "new-environment")

	// Now update the deployment to change the environment and application.
	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)

	deployment.Spec.Environment = "new-environment"
	deployment.Spec.Application = "new-application"

	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// Now the deployment will delete and re-create all of the items

	// Deletion of the deployment is in progress.
	status = waitForApplicationDeploymentStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Resource should be gone.
	_, err = radius.Resources(status.Scope, "Microsoft.Resources/deployments").Get(ctx, name.Name)
	require.Error(t, err)

	// Deployment will be waiting for extender to complete provisioning.
	status = waitForApplicationDeploymentStateUpdating(t, client, name, nil)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application", status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/new-environment/providers/Applications.Core/environments/new-environment", status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application/providers/Applications.Core/applications/new-application", status.Application)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	status = waitForApplicationDeploymentStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application/providers/Microsoft.Resources/deployments/test-applicationdeployment-change", status.Resource)

	// Now delete the deployment.
	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the resource is in progress.
	status = waitForApplicationDeploymentStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForApplicationDeploymentDeleted(t, client, name)
}

func Test_ApplicationDeploymentReconciler_FailureRecovery(t *testing.T) {
	// This test tests our ability to recover from failed operations inside Radius.
	//
	// We use the mock client to simulate the failure of update and delete operations
	// and verify that the controller will (eventually) retry these operations.

	ctx := testcontext.New(t)
	radius, client := SetupApplicationDeploymentTest(t)

	name := types.NamespacedName{Namespace: "applicationdeployment-failure-recovery", Name: "test-applicationdeployment-failure-recovery"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeApplicationDeployment(name, map[string]any{})
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for template to complete provisioning.
	status := waitForApplicationDeploymentStateUpdating(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation := status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("oops")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
		state.value = generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	})

	// Deployment should (eventually) start a new provisioning operation
	status = waitForApplicationDeploymentStateUpdating(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)
	_ = waitForApplicationDeploymentStateReady(t, client, name)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the deployment is in progress.
	status = waitForApplicationDeploymentStateDeleting(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation = status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("oops")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
	})

	// Deployment should (eventually) start a new deletion operation
	status = waitForApplicationDeploymentStateDeleting(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForApplicationDeploymentDeleted(t, client, name)
}

func waitForApplicationDeploymentStateUpdating(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.ApplicationDeploymentStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.ApplicationDeploymentStatus{}
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		logger.Logf("Fetching ApplicationDeployment: %+v", name)
		current := &radappiov1alpha3.ApplicationDeployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("ApplicationDeployment.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.ApplicationDeploymentPhraseUpdating, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}

	}, applicationDeploymentTestWaitDuration, applicationDeploymentTestWaitInterval, "failed to enter updating state")

	return status
}

func waitForApplicationDeploymentStateReady(t *testing.T, client client.Client, name types.NamespacedName) *radappiov1alpha3.ApplicationDeploymentStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.ApplicationDeploymentStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching ApplicationDeployment: %+v", name)
		current := &radappiov1alpha3.ApplicationDeployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("ApplicationDeployment.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.ApplicationDeploymentPhraseReady, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, applicationDeploymentTestWaitDuration, applicationDeploymentTestWaitInterval, "failed to enter updating state")

	return status
}

func waitForApplicationDeploymentStateDeleting(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.ApplicationDeploymentStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.ApplicationDeploymentStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching ApplicationDeployment: %+v", name)
		current := &radappiov1alpha3.ApplicationDeployment{}
		err := client.Get(ctx, name, current)
		assert.NoError(t, err)

		status = &current.Status
		logger.Logf("ApplicationDeployment.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.ApplicationDeploymentPhraseDeleting, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}
	}, applicationDeploymentTestWaitDuration, applicationDeploymentTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForApplicationDeploymentDeleted(t *testing.T, client client.Client, name types.NamespacedName) {
	ctx := testcontext.New(t)

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching ApplicationDeployment: %+v", name)
		current := &radappiov1alpha3.ApplicationDeployment{}
		err := client.Get(ctx, name, current)
		if apierrors.IsNotFound(err) {
			return true
		}

		logger.Logf("ApplicationDeployment.Status: %+v", current.Status)
		return false

	}, applicationDeploymentTestWaitDuration, applicationDeploymentTestWaitInterval, "ApplicationDeployment still exists")
}
