package container

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

var _ renderers.Renderer = (*MetaRenderer)(nil)

type MetaRenderer struct {
	Renderers map[string]renderers.Renderer
}

func (r *MetaRenderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return nil, nil, v1.ErrInvalidModelConversion
	}
	properties := resource.Properties

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here

	// ensure that users cannot use DNS-SD and httproutes simultaneously.
	for _, connection := range properties.Connections {
		if isURL(connection.Source) {
			continue
		}

		// if the source is not a URL, it either a resourceID or invalid.
		resourceID, err := resources.ParseResource(connection.Source)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid source: %s. Must be either a URL or a valid resourceID", connection.Source))
		}

		// Non-radius Azure connections that are accessible from Radius container resource.
		if connection.IAM.Kind.IsKind(datamodel.KindAzure) {
			azureResourceIDs = append(azureResourceIDs, resourceID)
			continue
		}

		if resources_radius.IsRadiusResource(resourceID) {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	for _, port := range properties.Container.Ports {
		provides := port.Provides

		// if provides is empty, skip this port. A service for this port will be generated later on.
		if provides == "" {
			continue
		}

		resourceID, err := resources.ParseResource(provides)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		if resources_radius.IsRadiusResource(resourceID) {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	for _, volume := range properties.Container.Volumes {
		switch volume.Kind {
		case datamodel.Persistent:
			resourceID, err := resources.ParseResource(volume.Persistent.Source)
			if err != nil {
				return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
			}

			if resources_radius.IsRadiusResource(resourceID) {
				radiusResourceIDs = append(radiusResourceIDs, resourceID)
				continue
			}
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

func (r *MetaRenderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	renderer, ok := r.Renderers[options.Environment.Kind]
	if !ok {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("no renderer found for environment kind: %s", options.Environment.Kind))
	}
	return renderer.Render(ctx, dm, options)
}
