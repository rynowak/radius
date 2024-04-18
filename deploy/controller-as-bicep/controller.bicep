import radius as radius
import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: namespace
}

param environment string
param namespace string
param tag string

var name = 'ipam-lb-controller'

resource application 'Applications.Core/applications@2023-10-01-preview' = {
  name: name
  properties: {
    environment: environment
    extensions: [{
      kind: 'kubernetesNamespace'
      namespace: namespace // Override the namespace for the application
    }]
  }
}

resource controller 'Applications.Core/containers@2023-10-01-preview' = {
  name: name
  properties: {
    application: application.id
    container: {
      image: 'ghcr.io/radius-project/controller:${tag}'

      // I'm ignoring the settings for passing in configuration as files. 
      // We don't have good ways to do that right now.
      //
      // I'm also ignoring the prometheus settings.
    }
  }
}

// The following grants permission for the controller to manage CilliumIPAMAssignment resources.
resource clusterrole 'rbac.authorization.k8s.io/ClusterRole@v1' = {
  metadata: {
    name: name
  }
  rules: [
    {
      apiGroups: ['cillium.io']
      resources: ['CilliumIPAMAssignment']
      verbs: ['get', 'list', 'watch', 'patch', 'create', 'update']
    }
  ]
}

resource clusterrolebinding 'rbac.authorization.k8s.io/ClusterRoleBinding@v1' = {
  metadata: {
    name: name
  }
  roleRef: {
    apiGroup: 'rbac.authorization.k8s.io'
    kind: 'ClusterRole'
    name: name
  }
  subjects: [
    {
      kind: 'ServiceAccount'
      name: name // Must match the service account name used by Radius. (the controller container resources name)
      namespace: namespace
    }
  ]
}
