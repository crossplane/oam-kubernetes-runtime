## How do I add trait with patch fragments

### Prerequisites
* [Install OAM Runtime](https://github.com/crossplane/oam-kubernetes-runtime#install-oam-runtime) 


### Install traits with patch fragments

* Install the Trait Definition

```
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: nodeselectors.extended.oam.dev
spec:
  patch: true 
  appliesToWorkloads:
    - core.oam.dev/v1alpha2.ContainerizedWorkload
  definitionRef:
    name: nodeselectors.extended.oam.dev
```
* Install NodeSelector Controller


* Add traits with patch fragments to the ApplicationConfiguration

`sample_application_config.yaml`

```
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      parameterValues:
        - name: instance-name
          value: example-appconfig-workload
        - name: image
          value: wordpress:php7.2
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManualScalerTrait
            metadata:
              name: example-appconfig-trait
            spec:
              replicaCount: 3
        - trait:
            apiVersion: extended.oam.dev/v1alpha2
            kind: NodeSelector
            metadata:
              name: example-appconfig-nodeselector
            spec:
              disktype: ssd
      scopes:
        - scopeRef:
            apiVersion: core.oam.dev/v1alpha2
            kind: HealthScope
            name: example-health-scope
```


* Apply a sample application configuration

```
# kubectl apply -f sample_application_config.yaml

```
* Check the status of the Trait and confirm whether the value of patchConfig is set successfully

```
# kubectl get NodeSelector  example-appconfig-nodeselector
```

```
apiVersion: extended.oam.dev/v1alpha2
kind: NodeSelector
metadata:
  name: example-component-trait-6db9578cfd
spec:
  disktype: ssd
status:
  patchConfig: nodeselector-cm
  phase: Ready
```


### Query ConfigMap content to confirm whether the Patch data is correct

```
# kubectl get cm nodeselector-cm -o yaml
```

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: nodeselector-cm
data:
  patch: |
    {
      "spec": {
        "template": {
          "spec": {
            "nodeSelector": {
              "disktype": "ssd"
            }
          }
        }
      }
    }

```

### Check the Spec content of the Workload

```
# kubectl get deployment example-appconfig-workload -o yaml

```
