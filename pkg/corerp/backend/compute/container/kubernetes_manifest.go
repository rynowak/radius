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

package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

var errDeploymentNotFound = errors.New("deployment resource must be in outputResources")

// fetchBaseManifest fetches the base manifest from the container resource.
func fetchBaseManifest(r *datamodel.ContainerResource) (kubeutil.ObjectManifest, error) {
	runtimes := r.Properties.Runtimes
	if runtimes != nil && runtimes.Kubernetes != nil && runtimes.Kubernetes.Base != "" {
		return kubeutil.ParseManifest([]byte(runtimes.Kubernetes.Base))
	}

	return kubeutil.ObjectManifest{}, nil
}

// baseDeployment returns the deployment resource based on the given base manifest.
// If the container has a base manifest, get the deployment resource from the base manifest.
// Otherwise, populate default resources.
func baseDeployment(manifest kubeutil.ObjectManifest, applicationName string, resourceName string, resourceType string, options *Options) *appsv1.Deployment {
	name := kubernetes.NormalizeResourceName(resourceName)

	defaultDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: name,
						},
					},

					// See: https://github.com/kubernetes/kubernetes/issues/92226 and
					// https://github.com/radius-project/radius/issues/3002
					//
					// Service links are a flawed and Kubernetes-only feature that we don't
					// want to leak into Radius containers. Users can override this by
					// providing their own base.
					EnableServiceLinks: to.Ptr(false),
				},
			},
		},
	}

	if d := manifest.GetFirst(appsv1.SchemeGroupVersion.WithKind("Deployment")); d != nil {
		defaultDeployment = d.(*appsv1.Deployment)
	}

	defaultDeployment.ObjectMeta = objectMeta(defaultDeployment.ObjectMeta, applicationName, resourceName, resourceType, *options)
	if defaultDeployment.Spec.Selector == nil {
		defaultDeployment.Spec.Selector = &metav1.LabelSelector{}
	}

	podTemplate := &defaultDeployment.Spec.Template
	if podTemplate.ObjectMeta.Labels == nil {
		podTemplate.ObjectMeta.Labels = map[string]string{}
	}

	if podTemplate.ObjectMeta.Annotations == nil {
		podTemplate.ObjectMeta.Annotations = map[string]string{}
	}

	if len(podTemplate.Spec.Containers) == 0 {
		podTemplate.Spec.Containers = []corev1.Container{}
	}

	found := false
	for _, container := range podTemplate.Spec.Containers {
		if strings.EqualFold(container.Name, name) {
			found = true
			break
		}
	}
	if !found {
		podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, corev1.Container{Name: name})
	}

	return defaultDeployment
}

// baseService returns the service resource based on the given base manifest.
// If the service has a base manifest, get the service resource from the base manifest.
// Otherwise, populate default resources.
func baseService(manifest kubeutil.ObjectManifest, applicationName string, resource *datamodel.ContainerResource, options *Options) *corev1.Service {
	defaultService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	if resource := manifest.GetFirst(corev1.SchemeGroupVersion.WithKind("Service")); resource != nil {
		defaultService = resource.(*corev1.Service)
	}
	defaultService.ObjectMeta = objectMeta(defaultService.ObjectMeta, applicationName, resource.Name, resource.ResourceTypeName(), *options)
	return defaultService
}

// getServiceAccountBase returns the service account resource based on the given base manifest.
// If the service account has a base manifest, get the service account resource from the base manifest.
// Otherwise, populate default resources.
func getServiceAccountBase(manifest kubeutil.ObjectManifest, appName string, r *datamodel.ContainerResource, options *renderers.RenderOptions) *corev1.ServiceAccount {
	defaultAccount := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
	}

	if resource := manifest.GetFirst(corev1.SchemeGroupVersion.WithKind("ServiceAccount")); resource != nil {
		defaultAccount = resource.(*corev1.ServiceAccount)
	}

	defaultAccount.ObjectMeta = objectMeta(defaultAccount.ObjectMeta, appName, r.Name, r.ResourceTypeName(), *options)

	return defaultAccount
}

// populateAllBaseResources populates all remaining resources from manifest into outputResources.
// These resources must be deployed before Deployment resource by adding them as a dependency.
func populateAllBaseResources(ctx context.Context, base kubeutil.ObjectManifest, outputResources []rpv1.OutputResource, options renderers.RenderOptions) []rpv1.OutputResource {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Find deployment resource from outputResources to add base manifest resources as a dependency.
	var deploymentResource *rpv1.Resource
	for _, r := range outputResources {
		if r.LocalID == rpv1.LocalIDDeployment {
			deploymentResource = r.CreateResource
			break
		}
	}

	// This should not happen because deployment resource is created in the first place.
	if deploymentResource == nil {
		panic(errDeploymentNotFound)
	}

	// Populate the remaining objects in base manifest into outputResources.
	// These resources must be deployed before Deployment resource by adding them as a dependency.
	for k, resources := range base {
		localIDPrefix := ""

		switch k {
		case corev1.SchemeGroupVersion.WithKind("Secret"):
			localIDPrefix = rpv1.LocalIDSecret
		case corev1.SchemeGroupVersion.WithKind("ConfigMap"):
			localIDPrefix = rpv1.LocalIDConfigMap

		default:
			continue
		}

		for _, resource := range resources {
			objMeta := resource.(metav1.ObjectMetaAccessor).GetObjectMeta().(*metav1.ObjectMeta)
			objMeta.Namespace = options.Environment.Namespace
			logger.Info(fmt.Sprintf("Adding base manifest resource, kind: %s, name: %s", k, objMeta.Name))

			localID := rpv1.NewLocalID(localIDPrefix, objMeta.Name)
			o := rpv1.NewKubernetesOutputResource(localID, resource, *objMeta)
			deploymentResource.Dependencies = append(deploymentResource.Dependencies, localID)
			outputResources = append(outputResources, o)
		}
	}

	return outputResources
}

func patchPodSpec(sourceSpec *corev1.PodSpec, patchSpec []byte) (*corev1.PodSpec, error) {
	podSpecJSON, err := json.Marshal(sourceSpec)
	if err != nil {
		return nil, err
	}

	merged, err := strategicpatch.StrategicMergePatch(podSpecJSON, patchSpec, corev1.PodSpec{})
	if err != nil {
		return nil, err
	}

	patched := &corev1.PodSpec{}
	err = json.Unmarshal(merged, patched)
	if err != nil {
		return nil, err
	}

	return patched, nil
}

func mergeLabelSelector(base *metav1.LabelSelector, current *metav1.LabelSelector) *metav1.LabelSelector {
	if base == nil {
		base = &metav1.LabelSelector{}
	}

	return &metav1.LabelSelector{
		MatchLabels:      labels.Merge(base.MatchLabels, current.MatchLabels),
		MatchExpressions: append(base.MatchExpressions, current.MatchExpressions...),
	}
}

func mergeObjectMeta(base metav1.ObjectMeta, current metav1.ObjectMeta) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        current.Name,
		Namespace:   current.Namespace,
		Labels:      labels.Merge(base.Labels, current.Labels),
		Annotations: labels.Merge(base.Annotations, current.Annotations),
	}
}

func objectMeta(base metav1.ObjectMeta, applicationName, resourceName, resourceType string, options Options) metav1.ObjectMeta {
	// TODO: get rid of reference to render options.
	renderOptions := renderers.RenderOptions{
		Application:  options.Application,
		Environment:  options.Environment,
		Dependencies: options.Dependencies,
	}
	current := metav1.ObjectMeta{
		Name:        kubernetes.NormalizeResourceName(resourceName),
		Namespace:   options.Environment.Namespace,
		Labels:      renderers.GetLabels(renderOptions, applicationName, resourceName, resourceType),
		Annotations: renderers.GetAnnotations(renderOptions),
	}

	return mergeObjectMeta(base, current)
}
