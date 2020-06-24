# Auto Binding in OAM

* Owner: Jianbo Sun (@wonderflow)
* Reviewers: Crossplane Maintainers
* Status: Draft

## Background

[Resource Dependency in OAM](resource-dependency.md) can handle one component depends on another.
It also contains a basic solution to pass through data from one component to another using
`dataOutputs` and `dataInputs`. The `toFieldPaths` field of `dataInputs` can specify a fieldPath
of workload to bind with. But if we want to bind with an object like K8s `Secret` or `ConfigMap`,
we should bind with the key of these objects to give better user experience.

## Use Case

### Web app and database

Let's use web app and database use case as an example.

We have a db component like below.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: db
spec:
  workload:
    apiVersion: database.alibaba.crossplane.io/v1alpha1
    kind: RDSInstance
    metadata:
      name: rdspostgresql
    spec:
      forProvider:
        engine: PostgreSQL
        engineVersion: "9.4"
        dbInstanceClass: rds.pg.s1.small
        dbInstanceStorageInGB: 20
        securityIPList: "0.0.0.0/0"
        masterUsername: "test123"
      writeConnectionSecretToRef:
        namespace: default
        name: alibaba-rdspostgresql-conn
      providerRef:
        name: alibaba-provider
      reclaimPolicy: Delete
```

And a web component which will use secret from db.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: webapp
spec:
  workload:
    apiVersion: serving.knative.dev/v1
    kind: Configuration
    metadata:
      name: webapp
      namespace: default
    spec:
      template:
        spec:
          containers:
            - image: oamdev/postgresql-flask-web-application:v0.1
              name: webapp
              env:
                - name: DB_HOST
                  valueFrom:
                    secretKeyRef:
                      key: endpoint
                - name: DB_USER
                  valueFrom:
                    secretKeyRef:
                      key: username
                - name: DB_PASSWORD
                  valueFrom:
                    secretKeyRef:
                      key: password
              ports:
                - containerPort: 80
                  name: http1 # Must be one of "http1" or "h2c" (if supported). Defaults to "http1".
          timeoutSeconds: 600
```

And with an ApplicationConfiguration:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: knative-postgresql-appconfig
spec:
  components:
    - componentName: webapp
      dataInputs:
        - valueFrom:
            dataOutputName: alibaba-rdspostgresql-conn
          toFieldPaths:
            - spec.containers[0].env[0].valueFrom.secretKeyRef.name
            - spec.containers[0].env[1].valueFrom.secretKeyRef.name
            - spec.containers[0].env[2].valueFrom.secretKeyRef.name

    - componentName: db
      dataOutputs:
        - name: alibaba-rdspostgresql-conn
          fieldPath: spec.writeConnectionSecretToRef.name
```

In this case, Developer of web component don't need to specify name of secret, but he has to specify the key of secret.

But if the developer don't know the secret name, how can he know the key? So I propose we should automatically bind
secret name along with the key.

## Proposal

The overall idea is to add more binding ways into `dataInputs` field.

For k8s `secret`, we can add `toSecretKeyRef` field into `dataInputs` for component `webapp`:

```
      dataInputs:
        - valueFrom:
            dataOutputName: alibaba-rdspostgresql-conn
          toSecretKeyRef:
            - secretKey: endpoint
              path: spec.template.spec.containers[0].env[0].valueFrom
```

In this case, the web component can be:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: webapp
spec:
  workload:
    apiVersion: serving.knative.dev/v1
    kind: Configuration
    metadata:
      name: webapp
      namespace: default
    spec:
      template:
        spec:
          containers:
            - image: oamdev/postgresql-flask-web-application:v0.1
              name: webapp
              env:
                - name: DB_HOST
                - name: DB_USER
                - name: DB_PASSWORD
              ports:
                - containerPort: 80
                  name: http1 # Must be one of "http1" or "h2c" (if supported). Defaults to "http1".
          timeoutSeconds: 600
```

Only need to clarify env name in component. And the AppConfig will look like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: knative-postgresql-appconfig
spec:
  components:
    - componentName: webapp
      dataInputs:
        - valueFrom:
            dataOutputName: alibaba-rdspostgresql-conn
          toSecretKeyRef:
            - secretKey: endpoint
              path: spec.template.spec.containers[0].env[0].valueFrom
            - secretKey: username
              path: spec.template.spec.containers[0].env[1].valueFrom
            - secretKey: password
              path: spec.template.spec.containers[0].env[2].valueFrom

    - componentName: db
      dataOutputs:
        - name: alibaba-rdspostgresql-conn
          fieldPath: spec.writeConnectionSecretToRef.name
```

The oam runtime will automatically bind secret name and key to the workload.

This means below fragment will be injected automatically.
```yaml
                  valueFrom:
                    name: alibaba-rdspostgresql-conn
                    secretKeyRef:
                      key: endpoint
```

## Extension

For K8s `Configmap`, we can also add `toConfigmapKeyRef`, which will be the same with `toSecretKeyRef`.

## Difference between service binding

Auto binding feature is similar to [service binding](https://github.com/oam-dev/trait-injector), but they have
different use cases.

Service Binding is trait, it can help bind external object while auto binding can help bind information between components.   
