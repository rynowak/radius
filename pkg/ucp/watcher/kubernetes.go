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
	"fmt"
	"slices"
	"strings"
	"sync"

	daprclient "github.com/dapr/go-sdk/client"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtimescheme "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clientcache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	runtime "sigs.k8s.io/controller-runtime"
	controllerbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	runtimemetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	pubSubComponent = "pubsub"
	pubSubTopic     = "ucp-notifications"
)

type kubernetesWatcher struct {
	restConfig *rest.Config

	dapr    daprclient.Client
	manager runtime.Manager

	mutex *sync.Mutex
	state map[schema.GroupVersionKind]kindState
}

type kindState struct {
	registration clientcache.ResourceEventHandlerRegistration
}

func (w *kubernetesWatcher) Run(ctx context.Context) error {
	dapr, err := daprclient.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create Dapr client: %w", err)
	}
	w.dapr = dapr

	scheme := runtimescheme.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))

	manager, err := runtime.NewManager(w.restConfig, runtime.Options{
		BaseContext: func() context.Context {
			return ctx
		},
		Logger:                 ucplog.FromContextOrDiscard(ctx).WithName("watcher.kubernetes"),
		HealthProbeBindAddress: "0", // Disable
		Metrics: runtimemetrics.Options{
			BindAddress: "0", // Disable
		},
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("failed to create controller-runtime manager: %w", err)
	}
	w.manager = manager

	// Handles changes CRD types and starts watching for each one.
	err = controllerbuilder.ControllerManagedBy(manager).
		Watches(&apiextv1.CustomResourceDefinition{}, &crdEventHandler{watcher: w}).
		Named("crd").
		Complete(&crdReconciler{})
	if err != nil {
		return fmt.Errorf("failed to create register CRD controller: %w", err)
	}

	// Handles built-in resource types.
	err = w.watchBuiltInTypes(ctx)
	if err != nil {
		return fmt.Errorf("failed to watch built-in types: %w", err)
	}

	// Blocks until cancellation despite the function name.
	return manager.Start(ctx)
}

func (w *kubernetesWatcher) watchBuiltInTypes(ctx context.Context) error {
	dc, err := discovery.NewDiscoveryClientForConfig(w.restConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Getting server groups and resources")
	_, resourceLists, err := dc.ServerGroupsAndResources()
	if err != nil {
		return fmt.Errorf("failed to get server groups and resources: %w", err)
	}

	for _, resourceList := range resourceLists {
		for _, resource := range resourceList.APIResources {
			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				return fmt.Errorf("failed to parse group version: %w", err)
			}

			// Skip subresources like deployments/scale
			if strings.Contains(resource.Name, "/") {
				continue
			}

			// Skip unwatchable resources like authentication.k8s.io/v1, Kind=TokenReview
			if !slices.Contains(resource.Verbs, "watch") {
				continue
			}

			w.watch(ctx, gv.WithKind(resource.Kind))
		}
	}

	logger.Info("Watching built-in resources")
	return nil
}

func (w *kubernetesWatcher) shouldWatch(gvk schema.GroupVersionKind) bool {
	// Skip resources that are very chatty, and unlikely to be useful.
	//
	// This includes some built-in Kubernetes functionality, as well as built-in Radius functionality.

	if gvk.Group == "events.k8s.io" && gvk.Kind == "Event" {
		return false
	}

	if gvk.Group == "coordination.k8s.io" && gvk.Kind == "Lease" {
		return false
	}

	if gvk.Group == "ucp.dev" && gvk.Kind == "QueueMessage" {
		return false
	}

	if gvk.Group == "ucp.dev" && gvk.Kind == "Resource" {
		return false
	}

	return true
}

func (w *kubernetesWatcher) watch(ctx context.Context, gvk schema.GroupVersionKind) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.state == nil {
		w.state = map[schema.GroupVersionKind]kindState{}
	}

	if _, ok := w.state[gvk]; ok {
		return
	}

	if !w.shouldWatch(gvk) {
		return
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Watching resource", "kind", gvk)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	informer, err := w.manager.GetCache().GetInformer(ctx, obj)
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to get informer for kind", "kind", gvk)
		return
	}

	registration, err := informer.AddEventHandler(&resourceEventHandler{baseContext: ctx, dapr: w.dapr})
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to add event handler for kind", "kind", gvk)
		return
	}

	w.state[gvk] = kindState{registration: registration}
}

func (w *kubernetesWatcher) unwatch(ctx context.Context, gvk schema.GroupVersionKind) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.state == nil {
		w.state = map[schema.GroupVersionKind]kindState{}
	}

	state, ok := w.state[gvk]
	if !ok {
		return
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Unwatching resource", "kind", gvk)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	informer, err := w.manager.GetCache().GetInformer(ctx, obj)
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to get informer for kind", "kind", gvk)
		return
	}

	err = informer.RemoveEventHandler(state.registration)
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "failed to add remove event handler for kind", "kind", gvk)
		return
	}

	delete(w.state, gvk)
}

var _ handler.EventHandler = &crdEventHandler{}

type crdEventHandler struct {
	watcher *kubernetesWatcher
}

func (c *crdEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[runtimeclient.Object], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.Object.(*apiextv1.CustomResourceDefinition)

	gvk := schema.GroupVersionKind{
		Group:   obj.Spec.Group,
		Kind:    obj.Spec.Names.Kind,
		Version: obj.Spec.Versions[0].Name,
	}

	c.watcher.watch(ctx, gvk)
}

func (c *crdEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[runtimeclient.Object], _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.Object.(*apiextv1.CustomResourceDefinition)

	gvk := schema.GroupVersionKind{
		Group:   obj.Spec.Group,
		Kind:    obj.Spec.Names.Kind,
		Version: obj.Spec.Versions[0].Name,
	}

	c.watcher.unwatch(ctx, gvk)
}

func (c *crdEventHandler) Generic(context.Context, event.TypedGenericEvent[runtimeclient.Object], workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (c *crdEventHandler) Update(context.Context, event.TypedUpdateEvent[runtimeclient.Object], workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

type crdReconciler struct {
}

func (c *crdReconciler) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

var _ clientcache.ResourceEventHandler = &resourceEventHandler{}

type resourceEventHandler struct {
	baseContext context.Context
	dapr        daprclient.Client
}

func (r *resourceEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	resource := obj.(client.Object)
	gvk := resource.GetObjectKind().GroupVersionKind()

	id := resources_kubernetes.IDFromParts("local", gvk.Group, gvk.Kind, resource.GetNamespace(), resource.GetName())
	notification := &Notification{
		ID:     id.String(),
		Reason: NotificationReasonCreated,
	}

	err := r.dapr.PublishEvent(r.baseContext, pubSubComponent, pubSubTopic, notification)
	if err != nil {
		ucplog.FromContextOrDiscard(r.baseContext).Error(err, "failed to publish event")
	}
}

func (r *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	resource := newObj.(client.Object)
	gvk := resource.GetObjectKind().GroupVersionKind()

	id := resources_kubernetes.IDFromParts("local", gvk.Group, gvk.Kind, resource.GetNamespace(), resource.GetName())
	notification := &Notification{
		ID:     id.String(),
		Reason: NotificationReasonUpdated,
	}

	err := r.dapr.PublishEvent(r.baseContext, pubSubComponent, pubSubTopic, notification)
	if err != nil {
		ucplog.FromContextOrDiscard(r.baseContext).Error(err, "failed to publish event")
	}
}

func (r *resourceEventHandler) OnDelete(obj interface{}) {
	resource := obj.(client.Object)
	gvk := resource.GetObjectKind().GroupVersionKind()

	id := resources_kubernetes.IDFromParts("local", gvk.Group, gvk.Kind, resource.GetNamespace(), resource.GetName())
	notification := &Notification{
		ID:     id.String(),
		Reason: NotificationReasonDeleted,
	}

	err := r.dapr.PublishEvent(r.baseContext, pubSubComponent, pubSubTopic, notification)
	if err != nil {
		ucplog.FromContextOrDiscard(r.baseContext).Error(err, "failed to publish event")
	}
}
