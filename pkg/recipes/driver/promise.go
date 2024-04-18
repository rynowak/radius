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

package driver

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/ucp/resources"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Driver = (*PromiseDriver)(nil)

type PromiseDriver struct {
	RuntimeClient runtime_client.WithWatch
}

func (p *PromiseDriver) Delete(ctx context.Context, opts DeleteOptions) error {
	gvr := definitionToGVR(opts.Definition)

	obj := makePromiseObject(gvr, opts.BaseOptions)
	err := p.RuntimeClient.Delete(ctx, obj)
	if err != nil {
		return recipes.NewRecipeError(v1.CodeInternal, fmt.Sprintf("failed to delete promise %s: %w", opts.Recipe.Name, err.Error()), util.ExecutionError)
	}

	return nil
}

func (p *PromiseDriver) Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error) {
	gvr := definitionToGVR(opts.Definition)

	obj := makePromiseObject(gvr, opts.BaseOptions)
	obj.Object["spec"] = makePromiseSpec(opts)

	err := p.RuntimeClient.Patch(ctx, obj, runtime_client.Apply, &runtime_client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return nil, recipes.NewRecipeError(v1.CodeInternal, fmt.Sprintf("failed to update promise %s: %w", opts.Recipe.Name, err.Error()), util.ExecutionError)
	}

	output, err := p.watchPromise(ctx, obj)
	if err != nil {
		return nil, recipes.NewRecipeError(v1.CodeInternal, fmt.Sprintf("failed to watch promise %s: %w", opts.Recipe.Name, err.Error()), util.ExecutionError)
	}

	output.Resources = append(output.Resources, resourceIdForPromiseObject(gvr, obj))
	return output, nil
}

func (p *PromiseDriver) watchPromise(ctx context.Context, original *unstructured.Unstructured) (*recipes.RecipeOutput, error) {
	// Wait for the promise to complete processing.
	objs := unstructured.UnstructuredList{
		Object: map[string]any{
			"apiVersion": original.GetAPIVersion(),
			"kind":       original.GetKind(),
		},
	}

	ww, err := p.RuntimeClient.Watch(ctx, &objs, runtime_client.InNamespace(original.GetNamespace()))
	if err != nil {
		return nil, fmt.Errorf("failed to watch promise %s: %w", original.GetName(), err)
	}

	defer ww.Stop()
	for {
		select {
		case event, ok := <-ww.ResultChan():
			if !ok {
				return nil, fmt.Errorf("watch for promise %s closed unexpectedly", original.GetName())
			}

			if event.Type == watch.Deleted {
				return nil, fmt.Errorf("promise %s was deleted", original.GetName())
			} else if event.Type != watch.Modified && event.Type != watch.Added {
				// Make sure to process BOTH modified and added. Added is needed to observe
				// the initial state of the object.
				continue
			}

			obj := event.Object.(*unstructured.Unstructured)
			if obj.GetName() != original.GetName() {
				continue
			}

			conditions, err := convertConditions(obj)
			if err != nil {
				return nil, fmt.Errorf("failed to convert conditions: %w", err)
			}

			ready := false
			for _, c := range conditions {
				if c.Type == "PipelineCompleted" && c.Status == metav1.ConditionTrue {
					ready = true
				}
			}

			if !ready {
				continue
			}

			return convertStatus(obj)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (p *PromiseDriver) GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error) {
	gvr := definitionToGVR(opts.Definition)
	crd := &apiextv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
	}
	err := p.RuntimeClient.Get(ctx, types.NamespacedName{Name: gvr.GroupResource().String()}, crd)
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", gvr.GroupResource().String(), err)
	}

	parameters := map[string]any{}
	metadata := map[string]any{
		"parameters": parameters,
	}

	found := false
	for _, version := range crd.Spec.Versions {
		if version.Name != gvr.Version {
			continue
		}

		found = true

		spec := version.Schema.OpenAPIV3Schema.Properties["spec"]
		for name, schema := range spec.Properties {
			parameters[name] = map[string]any{
				"type":        schema.Type,
				"description": schema.Description,
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("version %s not found in CRD %s", gvr.Version, gvr.GroupResource().String())
	}

	return metadata, nil
}

func definitionToGVR(definition recipes.EnvironmentDefinition) schema.GroupVersionResource {
	return schema.ParseGroupResource(definition.TemplatePath).WithVersion(definition.TemplateVersion)
}

func makePromiseObject(gvr schema.GroupVersionResource, opts BaseOptions) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvr.GroupVersion().String(),
			"kind":       gvr.Resource, // For promises the resource and kind are the same
			"metadata": map[string]any{
				"name":      resources.MustParse(opts.Recipe.ResourceID).Name(),
				"namespace": opts.Configuration.Runtime.Kubernetes.Namespace,
			},
		},
	}
}

func makePromiseSpec(opts ExecuteOptions) map[string]any {
	spec := map[string]any{}

	// Merge parameters from the environment and resource
	for name, value := range opts.Definition.Parameters {
		spec[name] = value
	}
	for name, value := range opts.Recipe.Parameters {
		spec[name] = value
	}

	// Apply Recipe conventions for the promise spec.
	//
	// TODO: make this conditional on the promise definition having these fields.
	spec["name"] = resources.MustParse(opts.Recipe.ResourceID).Name()
	spec["namespace"] = opts.Configuration.Runtime.Kubernetes.Namespace
	spec["application"] = resources.MustParse(opts.Recipe.ApplicationID).Name()
	spec["environment"] = resources.MustParse(opts.Recipe.EnvironmentID).Name()

	return spec
}

func resourceIdForPromiseObject(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) string {
	return "/planes/kubernetes/local/namespace/" + obj.GetNamespace() + "/providers/" + gvr.Group + "/" + gvr.Resource + "/" + obj.GetName()
}

func convertStatus(obj *unstructured.Unstructured) (*recipes.RecipeOutput, error) {
	// Read fields set by the promise.
	o := obj.Object["status"]
	if o == nil {
		return nil, nil
	}

	status := o.(map[string]any)

	ro := &recipes.RecipeOutput{
		Resources: []string{},
		Values:    status["values"].(map[string]any),
		Secrets:   map[string]any{}, // Ignoring secrets for now.
	}

	resources := status["resources"].([]any)
	for _, r := range resources {
		ro.Resources = append(ro.Resources, r.(string))
	}

	return ro, nil
}

func convertConditions(obj *unstructured.Unstructured) ([]metav1.Condition, error) {
	// It doesn't implement observed generation :(
	if obj.Object["status"] == nil {
		return nil, nil
	}

	status := obj.Object["status"].(map[string]any)
	if status["conditions"] == nil {
		return nil, nil
	}

	db, err := json.Marshal(status["conditions"])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal conditions: %w", err)
	}

	cc := []metav1.Condition{}
	err = json.Unmarshal(db, &cc)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal conditions: %w", err)
	}

	return cc, nil
}
