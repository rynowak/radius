package container

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	servicediscoverytypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*ECSRenderer)(nil)

type ECSRenderer struct {
}

func (r *ECSRenderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	return nil, nil, nil
}

func (r *ECSRenderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	if options.Environment.Kind != "ecs" {
		return renderers.RendererOutput{}, errors.New("environment kind is not ecs")
	}

	container, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	// UGH
	container.Properties.Environment = options.Environment.ID

	clusterID, err := resources.ParseResource(options.Environment.ClusterID)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	tags := r.makeTags(container)
	role := r.makeIAMRole(clusterID, container, tags)
	rolePolicy := r.makeIAMRolePolicy(container)

	serviceDependencies := []string{}
	var serviceDiscoveryService *servicediscovery.CreateServiceInput
	if len(container.Properties.Container.Ports) > 0 {
		serviceDiscoveryService = r.makeServiceDiscoveryService(clusterID, container, tags)
		serviceDependencies = append(serviceDependencies, "ServiceDiscoveryService")
	}

	taskDefinition := r.makeTaskDefinition(clusterID, container, tags)
	service := r.makeService(clusterID.Name(), container, tags)

	// TODO:
	// - restart policy
	// - Pull policy
	// - Volumes
	// - Replicas
	// - Dapr
	// - Hostname

	r.processConnections(container, options.Dependencies, taskDefinition)
	r.processEnvVars(container, taskDefinition)
	r.processCommandLine(container, taskDefinition)
	r.processHealthChecks(container, taskDefinition)
	r.processPorts(container, taskDefinition, service)
	r.processDiagnostics(container, taskDefinition)

	resources := []rpv1.OutputResource{
		{
			ID:            resources.MustParse(clusterID.RootScope() + "/providers/AWS.IAM/Role/" + container.Name),
			LocalID:       "Role",
			RadiusManaged: to.Ptr(true),
			CreateResource: &rpv1.Resource{
				Data:         role,
				Dependencies: []string{},
				ResourceType: resourcemodel.ResourceType{
					Type:     "AWS.IAM/Role",
					Provider: "aws",
				},
			},
		},
		{
			ID:            resources.MustParse(clusterID.RootScope() + "/providers/AWS.IAM/RolePolicy/" + container.Name),
			LocalID:       "RolePolicy",
			RadiusManaged: to.Ptr(true),
			CreateResource: &rpv1.Resource{
				Data:         rolePolicy,
				Dependencies: []string{"Role"},
				ResourceType: resourcemodel.ResourceType{
					Type:     "AWS.IAM/RolePolicy",
					Provider: "aws",
				},
			},
		},
		{
			ID:            resources.MustParse(clusterID.RootScope() + "/providers/AWS.ServiceDiscovery/Service/" + container.Name),
			LocalID:       "ServiceDiscoveryService",
			RadiusManaged: to.Ptr(true),
			CreateResource: &rpv1.Resource{
				Data: serviceDiscoveryService,
				ResourceType: resourcemodel.ResourceType{
					Type:     "AWS.ServiceDiscovery/Service",
					Provider: "aws",
				},
			},
		},
		{
			ID:            resources.MustParse(clusterID.RootScope() + "/providers/AWS.ECS/TaskDefinition/" + container.Name),
			LocalID:       "TaskDefinition",
			RadiusManaged: to.Ptr(true),
			CreateResource: &rpv1.Resource{
				Data:         taskDefinition,
				Dependencies: []string{"Role", "RolePolicy"},
				ResourceType: resourcemodel.ResourceType{
					Type:     "AWS.ECS/TaskDefinition",
					Provider: "aws",
				},
			},
		},
		{
			ID:            resources.MustParse(clusterID.RootScope() + "/providers/AWS.ECS/Service/" + container.Name),
			LocalID:       "Service",
			RadiusManaged: to.Ptr(true),
			CreateResource: &rpv1.Resource{
				Data:         service,
				Dependencies: append(serviceDependencies, "TaskDefinition"),
				ResourceType: resourcemodel.ResourceType{
					Type:     "AWS.ECS/Service",
					Provider: "aws",
				},
			},
		},
	}

	container.Properties.Environment = "" // UGH
	return renderers.RendererOutput{Resources: resources}, nil
}

func (r *ECSRenderer) makeTags(container *datamodel.ContainerResource) map[string]string {
	return map[string]string{
		"radius:environment": container.Properties.Environment,
		"radius:application": container.Properties.Application,
	}
}

func (r *ECSRenderer) makeIAMRole(clusterID resources.ID, container *datamodel.ContainerResource, tags map[string]string) *iam.CreateRoleInput {
	applicationID := resources.MustParse(container.Properties.Application)
	environmentID := resources.MustParse(container.Properties.Environment)

	account := clusterID.FindScope("accounts")
	region := clusterID.FindScope("regions")
	applicationName := applicationID.Name()
	environmentName := environmentID.Name()

	// Allows ECS to assume the role.
	assumeRolePolicyDocument := fmt.Sprintf(`{
	   "Version":"2012-10-17",
	   "Statement":[
		  {
			 "Effect":"Allow",
			 "Principal":{
				"Service":[
				   "ecs-tasks.amazonaws.com"
				]
			 },
			 "Action":"sts:AssumeRole",
			 "Condition":{
				"ArnLike":{
					"aws:SourceArn":"arn:aws:ecs:%s:%s:*"
				},
				"StringEquals":{
				   "aws:SourceAccount":"%s"
				}
			 }
		  }
	   ]
	}`, region, account, account)

	tt := []iamtypes.Tag{}
	for k, v := range tags {
		tt = append(tt, iamtypes.Tag{
			Key:   to.Ptr(k),
			Value: to.Ptr(v),
		})

	}
	return &iam.CreateRoleInput{
		Tags:                     tt,
		RoleName:                 to.Ptr(fmt.Sprintf("%s-%s-%s-execution-role", environmentName, applicationName, container.Name)),
		Description:              to.Ptr(fmt.Sprintf("IAM Execution Role for %s container of %s deployed to %s", container.Name, applicationName, environmentName)),
		Path:                     to.Ptr(fmt.Sprintf("/radius/%s/%s/", environmentName, applicationName)),
		AssumeRolePolicyDocument: to.Ptr(assumeRolePolicyDocument),
	}
}

func (r *ECSRenderer) makeIAMRolePolicy(container *datamodel.ContainerResource) *iam.AttachRolePolicyInput {
	applicationID := resources.MustParse(container.Properties.Application)
	environmentID := resources.MustParse(container.Properties.Environment)

	applicationName := applicationID.Name()
	environmentName := environmentID.Name()

	return &iam.AttachRolePolicyInput{
		RoleName:  to.Ptr(fmt.Sprintf("%s-%s-%s-execution-role", environmentName, applicationName, container.Name)),
		PolicyArn: to.Ptr("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
	}
}

func (r *ECSRenderer) makeServiceDiscoveryService(clusterID resources.ID, container *datamodel.ContainerResource, tags map[string]string) *servicediscovery.CreateServiceInput {
	applicationID := resources.MustParse(container.Properties.Application)
	environmentID := resources.MustParse(container.Properties.Environment)

	applicationName := applicationID.Name()
	environmentName := environmentID.Name()

	tt := []servicediscoverytypes.Tag{}
	for k, v := range tags {
		tt = append(tt, servicediscoverytypes.Tag{
			Key:   to.Ptr(k),
			Value: to.Ptr(v),
		})
	}

	return &servicediscovery.CreateServiceInput{
		Name:        to.Ptr(fmt.Sprintf("%s.%s.%s", container.Name, applicationName, environmentName)),
		NamespaceId: to.Ptr("ns-lnj4yixvmi2tsgtz"), // TODO: move to environment
		DnsConfig: &servicediscoverytypes.DnsConfig{
			RoutingPolicy: servicediscoverytypes.RoutingPolicyMultivalue,
			DnsRecords: []servicediscoverytypes.DnsRecord{
				{
					TTL:  to.Ptr(int64(60)),
					Type: servicediscoverytypes.RecordTypeA,
				},
			},
		},
		Description: to.Ptr(fmt.Sprintf("Service for %s container of application %s deployed to environment %s", container.Name, applicationName, environmentName)),
		Tags:        tt,
	}
}

func (r *ECSRenderer) makeTaskDefinition(clusterID resources.ID, container *datamodel.ContainerResource, tags map[string]string) *ecs.RegisterTaskDefinitionInput {
	account := clusterID.FindScope("accounts")
	applicationName := resources.MustParse(container.Properties.Application).Name()
	environmentName := resources.MustParse(container.Properties.Environment).Name()

	tt := []ecstypes.Tag{}
	for k, v := range tags {
		tt = append(tt, ecstypes.Tag{
			Key:   to.Ptr(k),
			Value: to.Ptr(v),
		})
	}

	roleName := fmt.Sprintf("%s-%s-%s-execution-role", environmentName, applicationName, container.Name)
	return &ecs.RegisterTaskDefinitionInput{
		Tags: tt,
		RequiresCompatibilities: []ecstypes.Compatibility{
			ecstypes.CompatibilityFargate,
		},
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:  to.Ptr(container.Name),
				Image: to.Ptr(container.Properties.Container.Image),
			},
		},
		Family:           to.Ptr(container.Name),
		NetworkMode:      ecstypes.NetworkModeAwsvpc,
		ExecutionRoleArn: to.Ptr(fmt.Sprintf("arn:aws:iam::%s:role/radius/%s/%s/%s", account, environmentName, applicationName, roleName)),
		Cpu:              to.Ptr("512"),  // TODO: make this configurable
		Memory:           to.Ptr("1024"), // TODO: make this configurable
	}
}

func (r *ECSRenderer) makeService(clusterName string, container *datamodel.ContainerResource, tags map[string]string) *ecs.CreateServiceInput {
	tt := []ecstypes.Tag{}
	for k, v := range tags {
		tt = append(tt, ecstypes.Tag{
			Key:   to.Ptr(k),
			Value: to.Ptr(v),
		})
	}

	return &ecs.CreateServiceInput{
		Tags:           tt,
		Cluster:        to.Ptr(clusterName),
		ServiceName:    to.Ptr(container.Name),
		TaskDefinition: to.Ptr(container.Name),
		DesiredCount:   to.Ptr(int32(1)), // TODO: make this configurable
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        []string{"subnet-0f7d7e2d768c0aa4a"}, // TODO: move to environment
				SecurityGroups: []string{"sg-0588ba64503de734c"},     // TODO: move to environment
			},
		},
	}
}

type connectionValue struct {
	Value  string
	Secret bool
}

func (r *ECSRenderer) processConnections(container *datamodel.ContainerResource, dependencies map[string]renderers.RendererDependency, task *ecs.RegisterTaskDefinitionInput) {
	results := map[string]connectionValue{}
	for name, connection := range container.Properties.Connections {
		// Injected values were disabled by the user.
		if connection.GetDisableDefaultEnvVars() {
			continue
		}

		if isURL(connection.Source) {
			r.processURLConnection(name, connection.Source, results)
			continue
		}

		dependency, ok := dependencies[connection.Source]
		if !ok {
			continue // This should be handled before our code runs.
		}

		r.processResourceConnection(name, dependency, results)
	}

	for key, value := range results {
		keyCopy := key
		valueCopy := value.Value

		// TODO: handle secrets
		task.ContainerDefinitions[0].Environment = append(task.ContainerDefinitions[0].Environment, ecstypes.KeyValuePair{
			Name:  to.Ptr(keyCopy),
			Value: to.Ptr(valueCopy),
		})
	}
}

func (r *ECSRenderer) processURLConnection(name string, url string, results map[string]connectionValue) {
	// parse source into scheme, hostname, and port.
	scheme, hostname, port, err := parseURL(url)
	if err != nil {
		return
	}

	schemeKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "SCHEME")
	hostnameKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "HOSTNAME")
	portKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), "PORT")

	results[schemeKey] = connectionValue{Value: scheme, Secret: false}
	results[hostnameKey] = connectionValue{Value: hostname, Secret: false}
	results[portKey] = connectionValue{Value: port, Secret: false}
}

func (r *ECSRenderer) processResourceConnection(name string, dependency renderers.RendererDependency, results map[string]connectionValue) {
	for key, value := range dependency.ComputedValues {
		envKey := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))

		envValue := ""
		switch v := value.(type) {
		case string:
			envValue = v
		case float64:
			envValue = strconv.Itoa(int(v))
		case int:
			envValue = strconv.Itoa(v)
		}

		// TODO: handle secrets
		results[envKey] = connectionValue{Value: envValue, Secret: true}
	}
}

func (r *ECSRenderer) processEnvVars(container *datamodel.ContainerResource, task *ecs.RegisterTaskDefinitionInput) {
	for key, value := range container.Properties.Container.Env {
		keyCopy := key
		valueCopy := value

		task.ContainerDefinitions[0].Environment = append(task.ContainerDefinitions[0].Environment, ecstypes.KeyValuePair{
			Name:  to.Ptr(keyCopy),
			Value: to.Ptr(valueCopy),
		})
	}
}

func (r *ECSRenderer) processCommandLine(container *datamodel.ContainerResource, task *ecs.RegisterTaskDefinitionInput) {
	// Based on: https://stackoverflow.com/questions/44316361/difference-between-docker-entrypoint-and-kubernetes-container-spec-command

	// Use image as-is.
	if len(container.Properties.Container.Command) == 0 && len(container.Properties.Container.Args) == 0 {
		// Do nothing
		return
	}

	if len(container.Properties.Container.Command) == 0 && len(container.Properties.Container.Args) > 0 {
		task.ContainerDefinitions[0].Command = container.Properties.Container.Args
		return
	}

	if len(container.Properties.Container.Command) > 0 {
		task.ContainerDefinitions[0].Command = append(container.Properties.Container.Command, container.Properties.Container.Args...)
		task.ContainerDefinitions[0].EntryPoint = []string{""} // Blank out the entrypoint
		return
	}
}

func (r *ECSRenderer) processHealthChecks(container *datamodel.ContainerResource, task *ecs.RegisterTaskDefinitionInput) {
	// NOTE: there's no support for readiness checks in ECS.
	//
	// See: https://github.com/aws/containers-roadmap/issues/1670

	// TODO: implement or validate non-command health checks
	probe := container.Properties.Container.LivenessProbe
	if (probe == datamodel.HealthProbeProperties{}) {
		return

	}

	if probe.Exec != nil {
		task.ContainerDefinitions[0].HealthCheck = &ecstypes.HealthCheck{
			Command:  strings.Split(probe.Exec.Command, " "),
			Interval: to.Ptr(int32(to.Float32(probe.Exec.PeriodSeconds))),
		}

		if probe.Exec.PeriodSeconds == nil {
			task.ContainerDefinitions[0].HealthCheck.Interval = to.Ptr(int32(DefaultPeriodSeconds))
		} else {
			task.ContainerDefinitions[0].HealthCheck.Interval = to.Ptr(int32(to.Float32(probe.Exec.PeriodSeconds)))
		}

		if probe.Exec.TimeoutSeconds == nil {
			task.ContainerDefinitions[0].HealthCheck.Timeout = to.Ptr(int32(DefaultTimeoutSeconds))
		} else {
			task.ContainerDefinitions[0].HealthCheck.Timeout = to.Ptr(int32(to.Float32(probe.Exec.TimeoutSeconds)))
		}

		if probe.Exec.FailureThreshold == nil {
			task.ContainerDefinitions[0].HealthCheck.Retries = to.Ptr(int32(DefaultFailureThreshold))
		} else {
			task.ContainerDefinitions[0].HealthCheck.Retries = to.Ptr(int32(to.Float32(probe.Exec.FailureThreshold)))
		}

		if probe.Exec.FailureThreshold == nil {
			task.ContainerDefinitions[0].HealthCheck.StartPeriod = to.Ptr(int32(DefaultInitialDelaySeconds))
		} else {
			task.ContainerDefinitions[0].HealthCheck.StartPeriod = to.Ptr(int32(to.Float32(probe.Exec.InitialDelaySeconds)))
		}
		return
	}
}

func (r *ECSRenderer) processPorts(container *datamodel.ContainerResource, task *ecs.RegisterTaskDefinitionInput, service *ecs.CreateServiceInput) {
	applicationID := resources.MustParse(container.Properties.Application)

	service.ServiceConnectConfiguration = &ecstypes.ServiceConnectConfiguration{
		Enabled:  true,
		Services: []ecstypes.ServiceConnectService{},
	}

	for name, port := range container.Properties.Container.Ports {
		nameCopy := name
		portCopy := port

		// We just set the containerPort here because we're using awsvpc network mode.
		// The load balancer will handle the port->containerPort mapping.
		mapping := ecstypes.PortMapping{
			Name:          to.Ptr(nameCopy),
			ContainerPort: to.Ptr(portCopy.ContainerPort),
			Protocol:      ecstypes.TransportProtocolTcp,
		}

		if portCopy.Protocol == "UDP" {
			mapping.Protocol = ecstypes.TransportProtocolUdp
		}

		task.ContainerDefinitions[0].PortMappings = append(task.ContainerDefinitions[0].PortMappings, mapping)

		port := portCopy.Port
		if port == 0 {
			port = portCopy.ContainerPort
		}

		service.ServiceRegistries = []ecstypes.ServiceRegistry{
			{
				RegistryArn: to.Ptr("arn:aws:servicediscovery:us-west-2:664787032730:namespace/ns-lnj4yixvmi2tsgtz"), // TODO: move to environment
			},
		}

		serviceConnectService := ecstypes.ServiceConnectService{
			PortName:      to.Ptr(nameCopy),
			DiscoveryName: to.Ptr(fmt.Sprintf("%s-%s-%s", applicationID.Name(), container.Name, nameCopy)),
			ClientAliases: []ecstypes.ServiceConnectClientAlias{
				{
					DnsName: to.Ptr(fmt.Sprintf("%s-%s", applicationID.Name(), container.Name)),
					Port:    to.Ptr(port),
				},
			},
		}
		service.ServiceConnectConfiguration.Services = append(service.ServiceConnectConfiguration.Services, serviceConnectService)
	}
}

func (r *ECSRenderer) processDiagnostics(container *datamodel.ContainerResource, task *ecs.RegisterTaskDefinitionInput) {
	applicationID := resources.MustParse(container.Properties.Application)
	task.ContainerDefinitions[0].LogConfiguration = &ecstypes.LogConfiguration{
		LogDriver: ecstypes.LogDriverAwslogs,
		Options: map[string]string{
			"awslogs-group":         "/aws/ecs/prod-aws-ecs", // TODO: put this in the environment
			"awslogs-region":        "us-west-2",             // TODO: use cluster location
			"awslogs-stream-prefix": fmt.Sprintf("%s/%s", applicationID.Name(), container.Name),
		},
	}
}
