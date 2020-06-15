# Resource Dependency in OAM

- Owner: Hongchao Deng (@hongchaodeng)
- Date: 05/17/2020
- Status: Draft

## Background

Creating one application service or resource often relies on other services or resources to be ready.
This is a well-known pattern called dependency.

In OAM, we talk about dependency in two different contexts:

1. **Inter ApplicationConfiguration**:
  There is dependency workflow between multiple ApplicationConfigurations. This usually can be achieved
  via external workflow engine such as Tekton/Argo Workflow.
2. **Intra ApplicationConfiguration**: 
  There is dependency between Components and Traits within the same ApplicationConfiguration.
  Since they are applied at the same time, external workflow engine couldn't do anything to control their ordering.
  It is the OAM runtime that needs to satisfy this requirement.

In this proposal, we are handling the second case, *Intra ApplicationConfiguration* dependency.

## Goals

This proposal includes the following goals:

1. **Describe ordering for k8s-style resources**.
  The ordering between resources means creating one resource after another.
  For example, resource A could not be created until resource B has been created and provided outputs.

2. **Describe field data passing between resources**.
  It is still not enough to solve only ordering problem. We also need to handle field data passing.
  For example, resource A's field "spec.connSecret" relies on resource B's field "status.connSecret". 
  This means the field data of one resource is passed from another field of other resource and needs to wait the field data to be ready.
  In fact, ordering in the first goal can be modeled as data dependency as well except that the receiver doesn't use the data.

3. **OAM native experience**.
  OAM has application concepts like Components, Traits, etc.
  The solution should provide OAM native experience and written as native fields.
  But OAM resources are k8s resources too.
  The core dependency engine should be generic enough to extend to more OAM resources.

In the following, we will go through the use cases first and then propose a solution to satisfy them.


## Use Cases

### 1. Web app and database

An applicationConfiguration consists of two components: web and db (database).
Web component should not be created until db component is created,
including public endpoint and connection secret to be made ready.

### 2. ML app workflow

An ML applicationConfiguration from [Hypercycle ML](http://www.4paradigm.com/product/hypercycleml)
has three components:

- preprocess
- training
- serving

They have strict ordering:

```
preprocess -> training -> serving
```

Each component has to wait for data to be finished processing by the previous stage.
Today, ML frameworks including Tensorflow do not handle such dependency issue.
They rely on external tools (e.g. Argo Workflow) to handle it.
If we want to model them as Components in the same ApplicationConfiguration, the external tools could not help in this case.

### 3. Service-binding Trait

Service binding is an OAM trait that we develop to bind user specified input such as env or file path
to specific data sources such as secrets or events.
More background introduction can be found in this [slides](https://docs.google.com/presentation/d/1PseN_8_zZH8clWZJzP8tuRMpz51SxQb-mKeG1f4QVZQ/edit?usp=sharing).

A service binding trait needs to be applied before creating the component.
Moreover, this kind of trait ordering is quite general and applies to many other traits we have seen.

### 4. NSQ deployment

An [NSQ](https://github.com/nsqio/nsq) cluster is composed with three components: nsqd, nsqlookup and nsqadmin.
To set up a `nsq` cluster, one needs to first make sure that the `nsqd` component is up, then start and make sure that the `nsqlookup` component is up, and run the `nsqadmin` component at last.
nsqlookup consumes the result of nsqd component instance, and nsqadmin consumes the result of nsqlookup component instance.
If we couldn't describe the dependencies order between the components, the component couldn't run properly.

This is similar to [kubernetes#65502](https://github.com/kubernetes/kubernetes/issues/65502) , the issue describes dependencies between containers on the same Pod, while we are discussing dependencies between components.

### 5. etcd cluster backup

An etcd cluster would be created via EtcdCluster CR, and automatic periodic etcd backup would be made via EtcdBackup CR.
In OAM, we model EtcdCluster as Component and EtcdBackup as Trait.
However, the EtcdBackup depends on the connection secret specified in `status.connSecret` field of EtcdCluster.
In other words, the EtcdBackup trait depends on EtcdCluster component.

### 6. Ingress after component

We have an ingress implementation that requires the microservice component must be setup before applying ingress entry.
In this case, the ingress trait should only be created after the component is ready.


## Proposal

The overall idea is to have each resource specify data inputs coming from data outputs (i.e. other resources' fields).
The OAM runtime will build the dependency graph accordingly and only create dependent resources once the data outputs are ready.
To achieve this, we propose:


1. Add a field `dataOutputs` in Component configuration to specify a data output source:

    ```yaml
    dataOutputs:
    - name: <dataoutput-name>
      fieldPath: <object-field-path>
    ```

    For example, a my-rds database that outputs connection secret could be specified as:

    ```yaml
    kind: Component
    metadata:
      name: my-rds
    spec:
      workload:
        ... # connection secret name will be written to 'status.connSecret'

    ---
    kind: ApplicationConfiguration
    spec:
      components:
      - componentName: my-rds
        dataOutputs:
        - name: mysql-conn-secret
          fieldPath: status.connSecret
    ```

2. Add a field `dataInputs` in Component configuration to specify a data input:

    ```yaml
    dataInputs:
    - valueFrom:
        dataOutputName: <dataoutput-object-name>
      toFieldPaths: <field-paths-to-fill>
    ```

    For example, web app's dependency on RDS's database secret could be specified as:

    ```yaml
    kind: ApplicationConfiguration
    spec:
      components:
      - componentName: my-rds
        dataOutputs:
        - name: mysql-conn-secret
          fieldPath: status.connSecret

      - componentName: my-web
        dataInputs:
        - valueFrom:
            # Specify the dependency on a data output.
            # The component's workload won't be created until the data output has returned value to fill workload's fieldPaths. 
            dataOutputName: mysql-conn-secret
          toFieldPaths:
          - "spec.connSecret"

    ```

    In this case, the OAM runtime will automatically build a dependency from `my-web` to `my-rds`.

    It will wait until `my-rds` object's field `status.connSecret` has value:

    ```yaml
    apiVersion: example.io/v1alpha1
    kind: RDSInstance
    metadata:
      name: my-rds
    status:
      connSecret: mysql-conn # OAM runtime will wait for this field to have value
    ```

    Once the dependency has been satisfied, the OAM runtime will create `my-web` workload with
    the corresponding field paths patched with values from data outputs.
    
    ```yaml
    kind: MyWorkload
    metadata:
      name: my-web
    spec:
      connSecret: mysql-conn # patched by OAM runtime
    ```

3. Similary, add fields `dataOutputs` and `dataInputs` in Trait configuration to specify the dependency on data output objects.

    For example, we have an etcd backup trait that relies on the data output from an etcd cluster resource.
    We can specify it as:

    ```yaml
    kind: ApplicationConfiguration
    spec:
      components:
      - componentName: my-etcd-cluster
        dataoutputs:
        - name: etcd-conn-secret
          fieldpath: "status.connSecret"

        traits:
        - trait:
            apiVersion: example.io/v1alpha1
            kind: EtcdBackup
            metadata:
              name: my-etcd-backup
          dataInputs:
          - valueFrom: # An EtcdBackup trait has dependency on some DataOutput to fill its fields.
              dataOutputName: etcd-conn-secret
            toFieldPaths:
            - spec.connSecret
      ```

      In this case, the EtcdBackup CR creation will be hold until EtcdCluster CR has outputed value in `status.connSecret`
      and passed to EtcdBackup CR field `spec.connSecret`.

### Limitations

1. All the resources in the dependency graph should be created and managed by OAM runtime.
2. This proposal is designed only for creating resources. After resources have been created, subsequent data output updates will not trigger any actions.

### Generic core engine consideration

The design of resource dependency is a pretty generic engine that works for any k8s resources.
Even though we use OAM native fields like `dataInputs` or `dataOutputs` fields, those are just representation layer.
The underlying dependency engine along with the DataInput and DataOutput model is the core implementation,
which should be generic enough to convert or adopt future concepts or fields.


## Impact to existing system

This proposal is additional to existing OAM runtime. It would not affect any existing deployments.

If dataOutputName in a data input doesn't match any name of the data outputs, it would fail the deployment.

### Spec upgrade path consideration

Initially the feature will be experimental. It might change iteratively.
Once that happens, we strive to keep it upgradeable between two releases.
Here we discuss an idea to upgrade if that happens.

The core dependency engine in OAM runtime is a generic implementation.
We would map APIs to underlying DataInput/DataOutput objects.

It's inevitable that an intermediate phase where both old and new APIs co-exist.
But they don't both exist in the same ApplicationConfiguration.
Only one could exist and we don't need to handle any conflicts.
In that case, We could add an webhook to translate the old to the new API.

## Solution examples

In this section we are providing more solution examples for dependency use cases.

1. Web app and database.

    ```yaml
    kind: Component
    metadata:
      name: my-rds
    spec:
      workload:
        kind: RDSInstance
        metadata:
          name: my-rds
    ---

    kind: Component
    metadata:
      name: my-web
    spec:
      workload:
        kind: MyWorkload
        metadata:
          name: my-web
    ---

    kind: ApplicationConfiguration
    spec:
      components:
      - componentName: my-web
        dataInputs:
        - valueFrom:
            dataOutputName: mysql-conn-secret
          toFieldPaths:
          - "spec.connSecret"
      - componentName: my-rds
        dataOutputs:
          - name: mysql-conn-secret
            fieldPath: "status.connSecret"
    ```

2. Service binding trait must start before component.

    ApplicationConfiguration :

    ```yaml
    kind: ApplicationConfiguration
    spec:
      components:
      - componentName: my-web
        dataInputs:
        - fieldPaths: [] # This data input is for ordering purpose only
          valueFrom:
            dataOutputName: my-secret-binding
        traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha1
            kind: ServiceBinding
            metadata:
              name: my-secret-binding
            spec:
              from:
                secret:
                  name: my-secret
              to:
                env: true
          dataOutputs:
            - name: my-secret-binding
              fieldPath: status.ready
    ```

