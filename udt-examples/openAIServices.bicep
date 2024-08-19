
param context object

param location string = resourceGroup().location

// Translate the user-specified t-shirt size to a capacity value that the service understands.
var capacityLookup = {
  S: 10
  M: 20
  L: 30
}

resource account 'Microsoft.CognitiveServices/accounts@2024-04-01-preview' = {
  name: '${context.resource.name}-${uniqueString(context.resource.id)}'
  location: location
  sku: {
    name: 'S0'
  }
  kind: 'OpenAI'
  properties: {
    publicNetworkAccess: 'Enabled'
  }
  

  resource deployment 'deployments@2024-04-01-preview' = {
    name: 'gpt-35-turbo'
    sku: {
      name: 'Standard'
      capacity: capacityLookup[contains(context.resource.properties, 'capacity') ? context.resource.properties.capacity : 'M']
    }
    properties: {
      model: {
        format: 'OpenAI'
        name: 'gpt-35-turbo'
        version: '0613'
      }
      versionUpgradeOption: 'OnceNewDefaultVersionAvailable'
      currentCapacity: 10
      raiPolicyName: 'Microsoft.Default'
    }
  }
}

output result object = {
  values: {
    apiVersion: '2023-05-15'
    endpoint: account.properties.endpoint
    deployment: 'gpt-35-turbo'
  }
  secrets: {
#disable-next-line outputs-should-not-contain-secrets
    apiKey: account.listKeys().key1
  }
}
