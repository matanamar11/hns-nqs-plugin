# work In progress might not run

# NodeQuotaSync Plugin for HNS

The NodeQuotaSync plugin enables syncing the root subnamespace and secondary subnamespace with the nodes aloocatable resources in the cluster. It provides support for resources multiplier for over commit and reserved resources mechanism, making it easier to troubleshoot nodes without affecting the subnamespace wallets.

## Features

- Auto sync root subnamespace and secondary subnamespace with the matching nodes allocatable resources.
- Configureable resources multiplier for over commit.
- Reserved resources mechanism for remove nodes in a safe way.
- Config CRD

## Installation

To install the NodeQuotaSync plugin, follow these steps:

1. Clone the repository or download the plugin code.
2. Build the plugin using the provided build script.
3. Deploy

## Usage

1. Start the HNS service with the NodeQuotaSync plugin enabled.

## Configuration

The NodeQuotaSync plugin can be configured by modifying the HNS configuration file. The configuration options for the plugin are as follows:

```
apiVersion: dana.hns.io/v1alpha1
kind: NodeQuotaConfig
metadata:
  name: example-nodequotaconfig
spec:
  reservedHoursToLive: 24
  nodeGroups:
  - labelSelector:
      gpu: true
    name: true
    resourceMultiplier:
      cpu: "2"
      memory: "2"
    name: cluster
    resourceMultiplier:
      cpu: "2"
      memory: "3"
    isRoot: true
```

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

