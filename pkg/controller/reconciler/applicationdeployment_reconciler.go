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
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
)

const (
	deploymentResourceType = "Microsoft.Resources/deployments"
)

// ApplicationDeploymentReconciler reconciles a ApplicationDeployment object.
type ApplicationDeploymentReconciler struct {
	// Client is the Kubernetes client.
	Client client.Client

	// Scheme is the Kubernetes scheme.
	Scheme *runtime.Scheme

	// EventRecorder is the Kubernetes event recorder.
	EventRecorder record.EventRecorder

	// Radius is the Radius client.
	Radius RadiusClient

	// DelayInterval is the amount of time to wait between operations.
	DelayInterval time.Duration
}

// Reconcile is the main reconciliation loop for the ApplicationDeployment resource.
func (r *ApplicationDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "ApplicationDeployment", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	deployment := radappiov1alpha3.ApplicationDeployment{}
	err := r.Client.Get(ctx, req.NamespacedName, &deployment)
	if apierrors.IsNotFound(err) {
		// This can happen due to a data-race if the application deployment is created and then deleted before we can
		// reconcile it. There's nothing to do here.
		logger.Info("ApplicationDeployment is being deleted.")
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Unable to fetch resource.")
		return ctrl.Result{}, err
	}

	// Our algorithm is as follows:
	//
	// 1. Check if we have an "operation" in progress. If so, check it's status.
	//   a. If the operation is still in progress, then queue another reconcile (polling).
	//   b. If the operation completed successfully then update the status and continue processing (happy-path).
	//   c. If the operation failed then update the status and continue processing (retry).
	// 2. If the ApplicationDeployment is being deleted then process deletion.
	//   a. This may require us to start a DELETE operation. After that we can continue polling.
	// 3. If the ApplicationDeployment is not being deleted then process this as a creation or update.
	//   a. This may require us to start a PUT operation. After that we can continue polling.
	//
	// We do it this way because it guarantees that we only have one operation going at a time.

	if deployment.Status.Operation != nil {
		// NOTE: if reconcileOperation completes successfully, then it will return a "zero" result,
		// this means the operation has completed and we should continue processing.
		result, err := r.reconcileOperation(ctx, &deployment)
		if err != nil {
			logger.Error(err, "Unable to reconcile in-progress operation.")
			return ctrl.Result{}, err
		} else if result.IsZero() {
			// NOTE: if reconcileOperation completes successfully, then it will return a "zero" result,
			// this means the operation has completed and we should continue processing.
			logger.Info("Operation completed successfully.")
		} else {
			logger.Info("Requeueing to continue operation.")
			return result, nil
		}
	}

	if deployment.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, &deployment)
	}

	return r.reconcileUpdate(ctx, &deployment)
}

// ReconileOperation reconciles a ApplicationDeployment that has an operation in progress.
func (r *ApplicationDeploymentReconciler) reconcileOperation(ctx context.Context, deployment *radappiov1alpha3.ApplicationDeployment) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// NOTE: the pollers are actually different types, so we have to duplicate the code
	// for the PUT and DELETE handling. This makes me sad :( but there isn't a great
	// solution besides duplicating the code.
	//
	// The only difference between these two codepaths is how they handle success.
	if deployment.Status.Operation.OperationKind == radappiov1alpha3.OperationKindPut {
		poller, err := r.Radius.Resources(deployment.Status.Scope, deploymentResourceType).ContinueCreateOperation(ctx, deployment.Status.Operation.ResumeToken)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to continue PUT operation: %w", err)
		}

		_, err = poller.Poll(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to poll operation status: %w", err)
		}

		if !poller.Done() {
			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation is complete.
		_, err = poller.Result(ctx)
		if err != nil {
			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Update failed.")

			deployment.Status.Operation = nil
			deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseFailed

			err = r.Client.Status().Update(ctx, deployment)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		deployment.Status.Operation = nil
		deployment.Status.Resource = deployment.Status.Scope + "/providers/" + deploymentResourceType + "/" + deployment.Name
		return ctrl.Result{}, nil

	} else if deployment.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete {
		poller, err := r.Radius.Resources(deployment.Status.Scope, deploymentResourceType).ContinueDeleteOperation(ctx, deployment.Status.Operation.ResumeToken)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to continue DELETE operation: %w", err)
		}

		_, err = poller.Poll(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to poll operation status: %w", err)
		}

		if !poller.Done() {
			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation is complete.
		_, err = poller.Result(ctx)
		if err != nil {
			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Delete failed.")

			deployment.Status.Operation = nil
			deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseFailed

			err = r.Client.Status().Update(ctx, deployment)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		deployment.Status.Operation = nil
		deployment.Status.Resource = ""
		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	logger.Error(fmt.Errorf("unknown operation kind: %s", deployment.Status.Operation.OperationKind), "Unknown operation kind.")

	deployment.Status.Operation = nil
	deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseFailed

	err := r.Client.Status().Update(ctx, deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ApplicationDeploymentReconciler) reconcileUpdate(ctx context.Context, deployment *radappiov1alpha3.ApplicationDeployment) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Ensure that our finalizer is present before we start any operations.
	if controllerutil.AddFinalizer(deployment, ApplicationDeploymentFinalizer) {
		err := r.Client.Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deployment.Status.ObservedGeneration = deployment.Generation

	environmentName := "default"
	if deployment.Spec.Environment != "" {
		environmentName = deployment.Spec.Environment
	}

	applicationName := deployment.Namespace
	if deployment.Spec.Application != "" {
		applicationName = deployment.Spec.Application
	}

	labels := map[string]string{}
	if deployment.ObjectMeta.Labels != nil {
		argoLabel, ok := deployment.ObjectMeta.Labels["argocd.argoproj.io/instance"]
		if ok {
			labels["argocd.argoproj.io/instance"] = argoLabel
		}
	}

	resourceGroupID, environmentID, applicationID, err := resolveDependencies(ctx, r.Radius, "/planes/radius/local", environmentName, applicationName, labels)
	if err != nil {
		r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "DependencyError", err.Error())
		logger.Error(err, "Unable to resolve dependencies.")
		return ctrl.Result{}, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	deployment.Status.Scope = resourceGroupID
	deployment.Status.Environment = environmentID
	deployment.Status.Application = applicationID

	updatePoller, deletePoller, err := r.startPutOrDeleteOperationIfNeeded(ctx, deployment)
	if err != nil {
		logger.Error(err, "Unable to create or update resource.")
		r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if updatePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := updatePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deployment.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindPut}
		deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseUpdating
		err = r.Client.Status().Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	} else if deletePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := deletePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deployment.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseDeleting
		err = r.Client.Status().Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	// If we get here then it means we can process the result of the operation.
	logger.Info("Resource is in desired state.", "resourceId", deployment.Status.Resource)

	deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseReady
	err = r.Client.Status().Update(ctx, deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(deployment, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *ApplicationDeploymentReconciler) reconcileDelete(ctx context.Context, deployment *radappiov1alpha3.ApplicationDeployment) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deployment.Status.ObservedGeneration = deployment.Generation

	poller, err := r.startDeleteOperationIfNeeded(ctx, deployment)
	if err != nil {
		logger.Error(err, "Unable to delete resource.")
		r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if poller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := poller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deployment.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseDeleting
		err = r.Client.Status().Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
	// ApplicationDeployment
	if controllerutil.RemoveFinalizer(deployment, ApplicationDeploymentFinalizer) {
		err := r.Client.Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		deployment.Status.ObservedGeneration = deployment.Generation
	}

	deployment.Status.Phrase = radappiov1alpha3.ApplicationDeploymentPhraseDeleted
	err = r.Client.Status().Update(ctx, deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(deployment, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *ApplicationDeploymentReconciler) startPutOrDeleteOperationIfNeeded(ctx context.Context, deployment *radappiov1alpha3.ApplicationDeployment) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceID := deployment.Status.Scope + "/providers/" + deploymentResourceType + "/" + deployment.Name
	if deployment.Status.Resource != "" && !strings.EqualFold(deployment.Status.Resource, resourceID) {
		// If we get here it means that the environment or application changed, so we should delete
		// the old resource and create a new one.
		logger.Info("Resource is already created but is out-of-date")

		logger.Info("Starting DELETE operation.")
		poller, err := deleteResource(ctx, r.Radius, deployment.Status.Resource)
		if err != nil {
			return nil, nil, err
		} else if poller != nil {
			return nil, poller, nil
		}

		// Deletion was synchronous
		deployment.Status.Resource = ""
	}

	// Note: we separate this check from the previous block, because it could complete synchronously.
	if deployment.Status.Resource != "" {
		logger.Info("Resource is already created and is up-to-date.")
		return nil, nil, nil
	}

	template := map[string]any{}
	err := json.Unmarshal([]byte(deployment.Spec.Template), &template)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	logger.Info("Starting PUT operation.")
	properties := map[string]any{
		"mode": "Incremental",
		"providerConfig": map[string]any{
			"deployments": map[string]any{
				"type": "Microsoft.Resources",
				"value": map[string]any{
					"scope": deployment.Status.Scope,
				},
			},
			"radius": map[string]any{
				"type": "Radius",
				"value": map[string]any{
					"scope": deployment.Status.Scope,
				},
			},
		}, // TODO other providers
		"template": template,
		"parameters": map[string]map[string]any{
			"application": {
				"value": deployment.Status.Application,
			},
			"environment": {
				"value": deployment.Status.Environment,
			},
		}, // TODO
	}

	poller, err := createOrUpdateResource(ctx, r.Radius, resourceID, properties)
	if err != nil {
		return nil, nil, err
	} else if poller != nil {
		return poller, nil, nil
	}

	// Update was synchronous
	deployment.Status.Resource = resourceID
	return nil, nil, nil
}

func (r *ApplicationDeploymentReconciler) startDeleteOperationIfNeeded(ctx context.Context, deployment *radappiov1alpha3.ApplicationDeployment) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if deployment.Status.Resource == "" {
		logger.Info("Resource is already deleted (or was never created).")
		return nil, nil
	}

	logger.Info("Starting DELETE operation.")
	poller, err := deleteResource(ctx, r.Radius, deployment.Status.Resource)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, err
	}

	// Deletion was synchronous

	deployment.Status.Resource = ""
	return nil, nil
}

func (r *ApplicationDeploymentReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.ApplicationDeployment{}).
		Complete(r)
}
