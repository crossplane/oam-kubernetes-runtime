# Flight Tracker

This is an example microservices application with a Javascript Web UI, a PostgreSQL database, and a series of API microservices. The idea is that various app developers would create Components for their corresponding apps. The overall config will add traits and allow the app to be fully deployed

![](https://github.com/oam-dev/samples/raw/master/2.ServiceTracker_App/service-tracker-diagram.jpg)

> Full application original source here: https://github.com/chzbrgr71/service-tracker

## Apply Definition

For this demo, it is necessary to create a `WorkloadDefinition` for the `ContainerizedWorkload` and `PostgreSQLInstance` workload types, and a `TraitDefinition` for the `ManualScalerTrait` type.

```bash
$ kubectl apply -f definitions/
workloaddefinition.core.oam.dev/containerizedworkloads.core.oam.dev created
traitdefinition.core.oam.dev/manualscalertraits.core.oam.dev created
```

## Install Component

Register the `Components`.

```bash
$ kubectl apply -f components/
component.core.oam.dev/data-api created
component.core.oam.dev/tracker-postgres-db created
component.core.oam.dev/flights-api created
component.core.oam.dev/quakes-api created
component.core.oam.dev/service-tracker-ui created
component.core.oam.dev/weather-api created
```

## Run ApplicationConfiguration

Install the `ApplicationConfiguration`.

```bash
$ kubectl apply -f tracker-app-config.yaml
secret/dbuser created
applicationconfiguration.core.oam.dev/service-tracker created
```

## Apply Ingress

```bash
$ kubectl apply -f tracker-ingress.yaml
ingress.networking.k8s.io/tracker-ingress created
```

## Result

Visit your browser, and open `http://{your-ingress-address}/web-ui` for the Service Tracker website. Refresh the data on the dashboard for each of the microservices.

![image](https://tvax2.sinaimg.cn/large/ad5fbf65ly1ggh2brro77j21hb0tan00.jpg)

## Clean up

clean resource of `flight-tracke`.

```bash
$ kubectl delete -f .
$ kubectl delete -f components/
$ kubectl delete -f definitions/
```

> More info, please see https://github.com/oam-dev/samples/tree/master/2.ServiceTracker_App