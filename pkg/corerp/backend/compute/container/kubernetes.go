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
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/backend/compute"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// Liveness/Readiness constants
	DefaultInitialDelaySeconds = 0
	DefaultFailureThreshold    = 3
	DefaultPeriodSeconds       = 10
	DefaultTimeoutSeconds      = 5
)

// TODO: create a new type for this and stop referencing the renderers package.
type Options = renderers.RenderOptions

func RenderKubernetes(ctx context.Context, resource *datamodel.ContainerResource, opts *Options) (*compute.Deployment, error) {
	// The container can provide a "base manifest" that will act as the "default" for the
	// resources created in Kubernetes.
	manifest, err := fetchBaseManifest(resource)
	if err != nil {
		return nil, err
	}

	applicationID, err := resources.ParseResource(resource.Properties.Application)
	if err != nil {
		return nil, err
	}

	connections, err := processConnections(resource)
	if err != nil {
		return nil, err
	}

	deployment, err := renderKubernetesDeployment(ctx, resource, manifest, applicationID.Name(), connections, opts)
	if err != nil {
		return nil, err
	}

	return &compute.Deployment{
		Compute:  &kubernetesDeployment{Deployment: deployment},
		Identity: &kubernetesServiceAccount{},
	}, nil
}

type connection struct {
	Source string
	ID     *string
	URL    *string
	Roles  []string
}

type routeEntry struct {
	Name string
	Type string
}

func processConnections(resource *datamodel.ContainerResource) (map[string]connection, error) {
	return map[string]connection{}, nil
}

func renderKubernetesDeployment(
	ctx context.Context,
	resource *datamodel.ContainerResource,
	manifest kubeutil.ObjectManifest,
	applicationName string,
	connections map[string]connection,
	opts *Options) (*appsv1.Deployment, error) {

	normalizedName := kubernetes.NormalizeResourceName(resource.Name)

	properties := resource.Properties

	deployment := baseDeployment(manifest, applicationName, resource.Name, resource.ResourceTypeName(), opts)
	podSpec := &deployment.Spec.Template.Spec

	// Identify the primary container. The user can use the "base" to add additional containers.
	container := &podSpec.Containers[0]
	for i, c := range podSpec.Containers {
		if strings.EqualFold(c.Name, normalizedName) {
			container = &podSpec.Containers[i]
			break
		}
	}

	// Keep track of the set of routes "provided" by this container, we will need these to generate labels later
	routes, ports, err := processPortsAndRoutes(resource)
	if err != nil {
		return nil, err
	}

	container.Image = properties.Container.Image
	container.Ports = append(container.Ports, ports...)
	container.Command = properties.Container.Command
	container.Args = properties.Container.Args
	container.WorkingDir = properties.Container.WorkingDir

	// If the user has specified an image pull policy, use it. Else, we will use Kubernetes default.
	if properties.Container.ImagePullPolicy != "" {
		container.ImagePullPolicy = corev1.PullPolicy(properties.Container.ImagePullPolicy)
	}

	if !properties.Container.ReadinessProbe.IsEmpty() {
		var err error
		container.ReadinessProbe, err = makeHealthProbe(properties.Container.ReadinessProbe)
		if err != nil {
			return nil, fmt.Errorf("readiness probe encountered errors: %w ", err)
		}
	}

	if !properties.Container.LivenessProbe.IsEmpty() {
		var err error
		container.LivenessProbe, err = makeHealthProbe(properties.Container.LivenessProbe)
		if err != nil {
			return nil, fmt.Errorf("liveness probe encountered errors: %w ", err)
		}
	}

	// We build the environment variable list in a stable order for testability
	// For the values that come from connections we back them with secretData. We'll extract the values
	// and return them.
	env, _, err := getEnvVarsAndSecretData(resource, applicationName, opts.Dependencies)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain environment variables and secret data: %w", err)
	}

	for k, v := range properties.Container.Env {
		env[k] = corev1.EnvVar{Name: k, Value: v}
	}

	// Append in sorted order
	for _, key := range getSortedKeys(env) {
		container.Env = append(container.Env, env[key])
	}

	// TODO
	// If the container requires azure role, it needs to configure workload identity (aka federated identity).
	// identityRequired := len(roles) > 0

	// 	outputResources := []rpv1.OutputResource{}
	// 	deps := []string{}

	// 	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())

	// 	// Add volumes
	// 	volumes := []corev1.Volume{}

	// 	// Create Kubernetes resource name scoped in Kubernetes namespace
	// 	kubeIdentityName := normalizedName
	// 	podSpec.ServiceAccountName = normalizedName

	// 	// Create Azure resource name for managed/federated identity-scoped in resource group specified by Environment resource.
	// 	// To avoid the naming conflicts, we add the application name prefix to resource name.
	// 	azIdentityName := azrenderer.MakeResourceName(applicationName, resource.Name, azrenderer.Separator)

	// 	for volumeName, volumeProperties := range properties.Container.Volumes {
	// 		// Based on the kind, create a persistent/ephemeral volume
	// 		switch volumeProperties.Kind {
	// 		case datamodel.Ephemeral:
	// 			volumeSpec, volumeMountSpec, err := makeEphemeralVolume(volumeName, volumeProperties.Ephemeral)
	// 			if err != nil {
	// 				return []rpv1.OutputResource{}, nil, fmt.Errorf("unable to create ephemeral volume spec for volume: %s - %w", volumeName, err)
	// 			}
	// 			// Add the volume mount to the Container spec
	// 			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
	// 			// Add the volume to the list of volumes to be added to the Volumes spec
	// 			volumes = append(volumes, volumeSpec)
	// 		case datamodel.Persistent:
	// 			var volumeSpec corev1.Volume
	// 			var volumeMountSpec corev1.VolumeMount

	// 			properties, ok := dependencies[volumeProperties.Persistent.Source]
	// 			if !ok {
	// 				return []rpv1.OutputResource{}, nil, errors.New("volume dependency resource not found")
	// 			}

	// 			vol, ok := properties.Resource.(*datamodel.VolumeResource)
	// 			if !ok {
	// 				return []rpv1.OutputResource{}, nil, errors.New("invalid dependency resource")
	// 			}

	// 			switch vol.Properties.Kind {
	// 			case datamodel.AzureKeyVaultVolume:
	// 				// This will add the required managed identity resources.
	// 				identityRequired = true

	// 				// Prepare role assignments
	// 				roleNames := []string{}
	// 				if len(vol.Properties.AzureKeyVault.Secrets) > 0 {
	// 					roleNames = append(roleNames, AzureKeyVaultSecretsUserRole)
	// 				}
	// 				if len(vol.Properties.AzureKeyVault.Certificates) > 0 || len(vol.Properties.AzureKeyVault.Keys) > 0 {
	// 					roleNames = append(roleNames, AzureKeyVaultCryptoUserRole)
	// 				}

	// 				// Build RoleAssignment output.resource
	// 				kvID := vol.Properties.AzureKeyVault.Resource
	// 				roleAssignments, raDeps := azrenderer.MakeRoleAssignments(kvID, roleNames)
	// 				outputResources = append(outputResources, roleAssignments...)
	// 				deps = append(deps, raDeps...)

	// 				// Create Per-Pod SecretProviderClass for the selected volume
	// 				// csiobjectspec must be generated when volume is updated.
	// 				objectSpec, err := handlers.GetMapValue[string](properties.ComputedValues, azvolrenderer.SPCVolumeObjectSpecKey)
	// 				if err != nil {
	// 					return []rpv1.OutputResource{}, nil, err
	// 				}

	// 				spcName := kubernetes.NormalizeResourceName(vol.Name)
	// 				secretProvider, err := azrenderer.MakeKeyVaultSecretProviderClass(applicationName, spcName, vol, objectSpec, &options.Environment)
	// 				if err != nil {
	// 					return []rpv1.OutputResource{}, nil, err
	// 				}
	// 				outputResources = append(outputResources, *secretProvider)
	// 				deps = append(deps, rpv1.LocalIDSecretProviderClass)

	// 				// Create volume spec which associated with secretProviderClass.
	// 				volumeSpec, volumeMountSpec, err = azrenderer.MakeKeyVaultVolumeSpec(volumeName, volumeProperties.Persistent.MountPath, spcName)
	// 				if err != nil {
	// 					return []rpv1.OutputResource{}, nil, fmt.Errorf("unable to create secretstore volume spec for volume: %s - %w", volumeName, err)
	// 				}
	// 			default:
	// 				return []rpv1.OutputResource{}, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported volume kind: %s for volume: %s. Supported kinds are: %v", vol.Properties.Kind, volumeName, GetSupportedKinds()))
	// 			}

	// 			// Add the volume mount to the Container spec
	// 			container.VolumeMounts = append(container.VolumeMounts, volumeMountSpec)
	// 			// Add the volume to the list of volumes to be added to the Volumes spec
	// 			volumes = append(volumes, volumeSpec)

	// 			// Add azurestorageaccountname and azurestorageaccountkey as secrets
	// 			// These will be added as key-value pairs to the kubernetes secret created for the container
	// 			// The key values are as per: https://docs.microsoft.com/en-us/azure/aks/azure-files-volume
	// 			for key, value := range properties.ComputedValues {
	// 				if value.(string) == rpv1.LocalIDAzureFileShareStorageAccount {
	// 					// The storage account was not created when the computed value was rendered
	// 					// Lookup the actual storage account name from the local id
	// 					id := properties.OutputResources[value.(string)]
	// 					value = id.Name()
	// 				}
	// 				secretData[key] = []byte(value.(string))
	// 			}
	// 		default:
	// 			return []rpv1.OutputResource{}, secretData, v1.NewClientErrInvalidRequest(fmt.Sprintf("Only ephemeral or persistent volumes are supported. Got kind: %v", volumeProperties.Kind))
	// 		}
	// 	}

	// 	// In addition to the descriptive labels, we need to attach labels for each route
	// 	// so that the generated services can find these pods
	// 	for _, routeInfo := range routes {
	// 		routeLabels := kubernetes.MakeRouteSelectorLabels(applicationName, routeInfo.Type, routeInfo.Name)
	// 		podLabels = labels.Merge(routeLabels, podLabels)
	// 	}

	// 	serviceAccountBase := getServiceAccountBase(manifest, applicationName, resource, &options)
	// 	// In order to enable per-container identity, it creates user-assigned managed identity, federated identity, and service account.
	// 	if identityRequired {
	// 		// 1. Create Per-Container managed identity.
	// 		managedIdentity, err := azrenderer.MakeManagedIdentity(azIdentityName, options.Environment.CloudProviders)
	// 		if err != nil {
	// 			return []rpv1.OutputResource{}, nil, err
	// 		}
	// 		outputResources = append(outputResources, *managedIdentity)

	// 		// 2. Create Per-container federated identity resource.
	// 		fedIdentity, err := azrenderer.MakeFederatedIdentity(kubeIdentityName, &options.Environment)
	// 		if err != nil {
	// 			return []rpv1.OutputResource{}, nil, err
	// 		}
	// 		outputResources = append(outputResources, *fedIdentity)

	// 		// 3. Create Per-container service account.
	// 		saAccount := azrenderer.SetWorkloadIdentityServiceAccount(serviceAccountBase)
	// 		outputResources = append(outputResources, *saAccount)
	// 		deps = append(deps, rpv1.LocalIDServiceAccount)

	// 		// This is required to enable workload identity.
	// 		podLabels[azrenderer.AzureWorkloadIdentityUseKey] = "true"

	// 		// 4. Add RBAC resources to the dependencies.
	// 		for _, role := range roles {
	// 			deps = append(deps, role.LocalID)
	// 		}

	// 		computedValues[handlers.IdentityProperties] = rpv1.ComputedValueReference{
	// 			Value: options.Environment.Identity,
	// 			Transformer: func(r v1.DataModelInterface, cv map[string]any) error {
	// 				ei, err := handlers.GetMapValue[*rpv1.IdentitySettings](cv, handlers.IdentityProperties)
	// 				if err != nil {
	// 					return err
	// 				}
	// 				res, ok := r.(*datamodel.ContainerResource)
	// 				if !ok {
	// 					return errors.New("resource must be ContainerResource")
	// 				}
	// 				if res.Properties.Identity == nil {
	// 					res.Properties.Identity = &rpv1.IdentitySettings{}
	// 				}
	// 				res.Properties.Identity.Kind = ei.Kind
	// 				res.Properties.Identity.OIDCIssuer = ei.OIDCIssuer
	// 				return nil
	// 			},
	// 		}

	// 		computedValues[handlers.UserAssignedIdentityIDKey] = rpv1.ComputedValueReference{
	// 			LocalID:           rpv1.LocalIDUserAssignedManagedIdentity,
	// 			PropertyReference: handlers.UserAssignedIdentityIDKey,
	// 			Transformer: func(r v1.DataModelInterface, cv map[string]any) error {
	// 				resourceID, err := handlers.GetMapValue[string](cv, handlers.UserAssignedIdentityIDKey)
	// 				if err != nil {
	// 					return err
	// 				}
	// 				res, ok := r.(*datamodel.ContainerResource)
	// 				if !ok {
	// 					return errors.New("resource must be ContainerResource")
	// 				}
	// 				if res.Properties.Identity == nil {
	// 					res.Properties.Identity = &rpv1.IdentitySettings{}
	// 				}
	// 				res.Properties.Identity.Resource = resourceID
	// 				return nil
	// 			},
	// 		}
	// 	} else {
	// 		// If the container doesn't require identity, we'll use the default service account
	// 		or := rpv1.NewKubernetesOutputResource(rpv1.LocalIDServiceAccount, serviceAccountBase, serviceAccountBase.ObjectMeta)
	// 		outputResources = append(outputResources, or)
	// 		deps = append(deps, rpv1.LocalIDServiceAccount)
	// 	}

	// 	// Create the role and role bindings for SA.
	// 	role := makeRBACRole(applicationName, kubeIdentityName, options.Environment.Namespace, resource)
	// 	outputResources = append(outputResources, *role)
	// 	deps = append(deps, rpv1.LocalIDKubernetesRole)

	// 	roleBinding := makeRBACRoleBinding(applicationName, kubeIdentityName, podSpec.ServiceAccountName, options.Environment.Namespace, resource)
	// 	outputResources = append(outputResources, *roleBinding)
	// 	deps = append(deps, rpv1.LocalIDKubernetesRoleBinding)

	// 	deployment.Spec.Template.ObjectMeta = mergeObjectMeta(deployment.Spec.Template.ObjectMeta, metav1.ObjectMeta{
	// 		Labels: podLabels,
	// 	})

	// 	deployment.Spec.Selector = mergeLabelSelector(deployment.Spec.Selector, &metav1.LabelSelector{
	// 		MatchLabels: kubernetes.MakeSelectorLabels(applicationName, resource.Name),
	// 	})

	// 	podSpec.Volumes = append(podSpec.Volumes, volumes...)

	// 	// If the user has specified a restart policy, use it. Else, it will use the Kubernetes default.
	// 	if properties.RestartPolicy != "" {
	// 		podSpec.RestartPolicy = corev1.RestartPolicy(properties.RestartPolicy)
	// 	}

	// 	// If we have a secret to reference we need to ensure that the deployment will trigger a new revision
	// 	// when the secret changes. Normally referencing an environment variable from a secret will **NOT** cause
	// 	// a new revision when the secret changes.
	// 	//
	// 	// see: https://stackoverflow.com/questions/56711894/does-k8-update-environment-variables-when-secrets-change
	// 	//
	// 	// The solution to this is to embed the hash of the secret as an annotation in the deployment. This way when the
	// 	// secret changes we also change the content of the deployment and thus trigger a new revision. This is a very
	// 	// common solution to this problem, and not a bizarre workaround that we invented.
	// 	if len(secretData) > 0 {
	// 		hash := kubernetes.HashSecretData(secretData)
	// 		deployment.Spec.Template.ObjectMeta.Annotations[kubernetes.AnnotationSecretHash] = hash
	// 		deps = append(deps, rpv1.LocalIDSecret)
	// 	}

	// 	// Patching Runtimes.Kubernetes.Pod to the PodSpec in deployment resource.
	// 	if properties.Runtimes != nil && properties.Runtimes.Kubernetes != nil && properties.Runtimes.Kubernetes.Pod != "" {
	// 		patchedPodSpec, err := patchPodSpec(podSpec, []byte(properties.Runtimes.Kubernetes.Pod))
	// 		if err != nil {
	// 			return []rpv1.OutputResource{}, nil, fmt.Errorf("failed to patch PodSpec: %w", err)
	// 		}
	// 		deployment.Spec.Template.Spec = *patchedPodSpec
	// 	}

	// 	deploymentOutput := rpv1.NewKubernetesOutputResource(rpv1.LocalIDDeployment, deployment, deployment.ObjectMeta)
	// 	deploymentOutput.CreateResource.Dependencies = deps

	// 	outputResources = append(outputResources, deploymentOutput)
	// 	return outputResources, secretData, nil
	// }
	return deployment, nil
}

func processPortsAndRoutes(resource *datamodel.ContainerResource) ([]routeEntry, []corev1.ContainerPort, error) {

	routes := []routeEntry{}
	ports := []corev1.ContainerPort{}
	for _, port := range resource.Properties.Container.Ports {
		if provides := port.Provides; provides != "" {
			resourceId, err := resources.ParseResource(provides)
			if err != nil {
				return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
			}

			routeName := kubernetes.NormalizeResourceName(resourceId.Name())
			routeType := resourceId.TypeSegments()[len(resourceId.TypeSegments())-1].Type
			routeTypeParts := strings.Split(routeType, "/")

			routeTypeSuffix := kubernetes.NormalizeResourceName(routeTypeParts[len(routeTypeParts)-1])

			routes = append(routes, routeEntry{Name: routeName, Type: routeTypeSuffix})

			ports = append(ports, corev1.ContainerPort{
				// Name generation logic has to match the code in HttpRoute
				Name:          kubernetes.GetShortenedTargetPortName(routeTypeSuffix + routeName),
				ContainerPort: port.ContainerPort,
				Protocol:      corev1.ProtocolTCP,
			})
		} else {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: port.ContainerPort,
				Protocol:      corev1.ProtocolTCP,
			})
		}
	}
	return []routeEntry{}, ports, nil
}

func getEnvVarsAndSecretData(resource *datamodel.ContainerResource, applicationName string, dependencies map[string]renderers.RendererDependency) (map[string]corev1.EnvVar, map[string][]byte, error) {
	env := map[string]corev1.EnvVar{}
	secretData := map[string][]byte{}
	properties := resource.Properties

	// Take each connection and create environment variables for each part
	// We'll store each value in a secret named with the same name as the resource.
	// We'll use the environment variable names as keys.
	// Float is used by the JSON serializer
	for name, con := range properties.Connections {
		properties := dependencies[con.Source]
		if !con.GetDisableDefaultEnvVars() {
			source := con.Source
			if source == "" {
				continue
			}

			// handles case where container has source field structured as a URL.
			if isURL(source) {
				// parse source into scheme, hostname, and port.
				scheme, hostname, port, err := parseURL(source)
				if err != nil {
					return map[string]corev1.EnvVar{}, map[string][]byte{}, fmt.Errorf("failed to parse source URL: %w", err)
				}

				schemeKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "SCHEME")
				hostnameKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "HOSTNAME")
				portKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "PORT")

				env[schemeKey] = corev1.EnvVar{Name: schemeKey, Value: scheme}
				env[hostnameKey] = corev1.EnvVar{Name: hostnameKey, Value: hostname}
				env[portKey] = corev1.EnvVar{Name: portKey, Value: port}

				continue
			}

			// handles case where container has source field structured as a resourceID.
			for key, value := range properties.ComputedValues {
				name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))

				source := corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: kubernetes.NormalizeResourceName(resource.Name),
						},
						Key: name,
					},
				}
				switch v := value.(type) {
				case string:
					secretData[name] = []byte(v)
					env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
				case float64:
					secretData[name] = []byte(strconv.Itoa(int(v)))
					env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
				case int:
					secretData[name] = []byte(strconv.Itoa(v))
					env[name] = corev1.EnvVar{Name: name, ValueFrom: &source}
				}
			}
		}
	}

	return env, secretData, nil
}

func makeHealthProbe(p datamodel.HealthProbeProperties) (*corev1.Probe, error) {
	probeSpec := corev1.Probe{}

	switch p.Kind {
	case datamodel.HTTPGetHealthProbe:
		// Set the probe spec
		probeSpec.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{}
		probeSpec.ProbeHandler.HTTPGet.Port = intstr.FromInt(int(p.HTTPGet.ContainerPort))
		probeSpec.ProbeHandler.HTTPGet.Path = p.HTTPGet.Path
		httpHeaders := []corev1.HTTPHeader{}
		for k, v := range p.HTTPGet.Headers {
			httpHeaders = append(httpHeaders, corev1.HTTPHeader{
				Name:  k,
				Value: v,
			})
		}
		probeSpec.ProbeHandler.HTTPGet.HTTPHeaders = httpHeaders
		c := containerHealthProbeConfig{
			initialDelaySeconds: p.HTTPGet.InitialDelaySeconds,
			failureThreshold:    p.HTTPGet.FailureThreshold,
			periodSeconds:       p.HTTPGet.PeriodSeconds,
			timeoutSeconds:      p.HTTPGet.TimeoutSeconds,
		}
		setContainerHealthProbeConfig(&probeSpec, c)
	case datamodel.TCPHealthProbe:
		// Set the probe spec
		probeSpec.ProbeHandler.TCPSocket = &corev1.TCPSocketAction{}
		probeSpec.TCPSocket.Port = intstr.FromInt(int(p.TCP.ContainerPort))
		c := containerHealthProbeConfig{
			initialDelaySeconds: p.TCP.InitialDelaySeconds,
			failureThreshold:    p.TCP.FailureThreshold,
			periodSeconds:       p.TCP.PeriodSeconds,
			timeoutSeconds:      p.TCP.TimeoutSeconds,
		}
		setContainerHealthProbeConfig(&probeSpec, c)
	case datamodel.ExecHealthProbe:
		// Set the probe spec
		probeSpec.ProbeHandler.Exec = &corev1.ExecAction{}
		probeSpec.Exec.Command = strings.Split(p.Exec.Command, " ")
		c := containerHealthProbeConfig{
			initialDelaySeconds: p.Exec.InitialDelaySeconds,
			failureThreshold:    p.Exec.FailureThreshold,
			periodSeconds:       p.Exec.PeriodSeconds,
			timeoutSeconds:      p.Exec.TimeoutSeconds,
		}
		setContainerHealthProbeConfig(&probeSpec, c)
	default:
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("health probe kind unsupported: %v", p.Kind))
	}
	return &probeSpec, nil
}

type containerHealthProbeConfig struct {
	initialDelaySeconds *float32
	failureThreshold    *float32
	periodSeconds       *float32
	timeoutSeconds      *float32
}

func setContainerHealthProbeConfig(probeSpec *corev1.Probe, config containerHealthProbeConfig) {
	// Initialize with Radius defaults and overwrite if values are specified
	probeSpec.InitialDelaySeconds = DefaultInitialDelaySeconds
	probeSpec.FailureThreshold = DefaultFailureThreshold
	probeSpec.PeriodSeconds = DefaultPeriodSeconds
	probeSpec.TimeoutSeconds = DefaultTimeoutSeconds

	if config.initialDelaySeconds != nil {
		probeSpec.InitialDelaySeconds = int32(*config.initialDelaySeconds)
	}

	if config.failureThreshold != nil {
		probeSpec.FailureThreshold = int32(*config.failureThreshold)
	}

	if config.periodSeconds != nil {
		probeSpec.PeriodSeconds = int32(*config.periodSeconds)
	}

	if config.timeoutSeconds != nil {
		probeSpec.TimeoutSeconds = int32(*config.timeoutSeconds)
	}
}

// func makeSecret(ctx context.Context, resource datamodel.ContainerResource, applicationName string, secrets map[string][]byte, options renderers.RenderOptions) rpv1.OutputResource {
// 	secret := corev1.Secret{
// 		TypeMeta: metav1.TypeMeta{
// 			Kind:       "Secret",
// 			APIVersion: corev1.SchemeGroupVersion.String(),
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      kubernetes.NormalizeResourceName(resource.Name),
// 			Namespace: options.Environment.Namespace,
// 			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName()),
// 		},
// 		Type: corev1.SecretTypeOpaque,
// 		Data: secrets,
// 	}

// 	output := rpv1.NewKubernetesOutputResource(rpv1.LocalIDSecret, &secret, secret.ObjectMeta)
// 	return output
// }

func getSortedKeys(env map[string]corev1.EnvVar) []string {
	keys := []string{}
	for k := range env {
		key := k
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

func isURL(input string) bool {
	_, err := url.ParseRequestURI(input)

	// if first character is a slash, it's not a URL. It's a path.
	if input == "" || err != nil || input[0] == '/' {
		return false
	}
	return true
}

func parseURL(sourceURL string) (scheme, hostname, port string, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", "", err
	}

	scheme = u.Scheme
	host := u.Host

	hostname, port, err = net.SplitHostPort(host)
	if err != nil {
		return "", "", "", err
	}

	return scheme, hostname, port, nil
}
