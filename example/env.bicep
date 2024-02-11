import radius as radius

param name string = 'default'
param accountId string = '664787032730'
param region string = 'us-west-2'
param clusterName string = 'prod-aws-ecs'

param subnetGroupNames object = {
  memory_db: 'prod-aws-ecs'
}

param securityGroupIds array = [
  'sg-0588ba64503de734c'
]

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: name
  properties: {
    compute: {
      kind: 'ecs'
      resourceId: '/planes/aws/aws/accounts/${accountId}/regions/${region}/providers/AWS.ECS/Cluster/${clusterName}'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${accountId}/regions/${region}'
      }
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        default: {
          templatePath: 'rynowak.azurecr.io/recipes/redis-aws:0.30'
          templateKind: 'bicep'
          parameters: {
            subnetGroupName: subnetGroupNames.memory_db
            securityGroupIds: securityGroupIds
          }
        }
      }
    }
  }
}
