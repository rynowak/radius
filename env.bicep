import radius as radius

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'default'
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: 'default'
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        default: {
          templateKind: 'promise'
          templatePath: 'redis.example.promise.syntasso.io'
          templateVersion: 'v1alpha1'
        }
      }
    }
  }
}
