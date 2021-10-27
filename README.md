<a href="https://www.vectorstock.com/royalty-free-vector/disc-golf-frisbee-eps-vector-25179185" target="_blank"><figure><img src="https://github.com/CARV-ICS-FORTH/frisbee/blob/main/docs/images/logo.jpg" width="400"></figure></a>

# Why Frisbee ?

Frisbee is a next generation platform designed to unify chaos testing and perfomance benchmarking.

We address the key pain points developers and QA engineers face when testing cloud-native applications in the earlier
stages of the software lifecycle.

We make it possible to:

* **Write tests:**  for stressing complex topologies and dynamic operating conditions.
* **Run tests:**  provides seamless scaling from a single workstation to hundreds of machines.
* **Debug tests:**  through extensive monitoring and comprehensive dashboards

Our platform consists of a set of Kubernetes controller designed to run performance benchmarks and and introduce failure
conditions into a running system, monitor site-wide health metrics, and notify systems with status updates during the
testing procedure.

Frisbee provides a flexible, YAML-based configuration syntax and is trivially extensible with additional functionality.

# Frisbee in a nutshell

The easiest way to begin with is by have a look at the examples folder. It consists of two sub-directories:

* **Templates:** are libraries of frequently-used specifications that are reusable throughout the testing plan.
* **Testplans:** are lists of actions that define what will happen throughout the test.

We will use the `examples/testplans/3.failover.yml` as a reference.

This plans uses the following templates:

* `examples/templates/core/sysmon.yml`
* `examples/templates/redis/redis.cluster.yml`
* `examples/templates/ycsb/redis.client.yml`

Because these templates are deployed as Kubernetes resources, they are references by name rather than by the relative
path.

This is why we need to have them installed before running the experiment. (for installation instructions
check [here](docs/singlenode-deployment.md).)

```yaml
# Standard Kubernetes boilerplate
apiVersion: frisbee.io/v1alpha1
kind: Workflow
metadata:
  name: redis-failover
spec:

  # Here we specify the workflow as a directed-acyclic graph (DAG) by specifying the dependencies of each action.
  actions:
    # Service creates an instance of a Redis Master
    # To create the instance we use the redis/master with the default parameters.
    - action: Service
      name: master
      service:
        fromTemplate:
          templateRef: redis/master

    # This action is same as before, with two additions. 
    # 1. The `depends' keyword ensure that the action will be executed only after the `master' action 
    # has reached a Running state.
    # 2. The `inputs' keyword initialized the instance with custom parameters. 
    - action: Service
      name: slave
      depends: { running: [ master ] }
      service:
        fromTemplate:
          templateRef: redis/slave
          inputs:
            { master: .service.master.any }

    # The sentinel is Redis failover manager. Notice that we can have multiple dependencies.
    - action: Service
      name: sentinel
      depends: { running: [ master, slave ] }
      service:
        fromTemplate:
          templateRef: redis/sentinel
          inputs:
            { master: .service.master.any }

    # Cluster creates a list of services that run a shared context. 
    # In this case, we create a cluster of YCSB loaders to populate the master with keys. 
    - action: Cluster
      name: "loaders"
      depends: { running: [ master ] }
      cluster:
        templateRef: ycsb-redis/loader
        inputs:
          - { server: .service.master.any, recordcount: "100000000", offset: "0" }
          - { server: .service.master.any, recordcount: "100000000", offset: "100000000" }
          - { server: .service.master.any, recordcount: "100000000", offset: "200000000" }

    # While the loaders are running, we inject a network partition fault to the master node. 
    # The "after" dependency adds a delay so to have some keys before injecting the fault. 
    # The fault is automatically retracted after 2 minutes. 
    - action: Chaos
      name: partition0
      depends: { running: [ loaders ], after: "3m" }
      chaos:
        type: partition
        partition:
          selector:
            macro: .service.master.any
          duration: "2m"

    # Here we repeat the partition, a few minutes after the previous fault has been recovered.
    - action: Chaos
      name: partition1
      depends: { running: [ master, slave ], success: [ partition0 ], after: "6m" }
      chaos:
        type: partition
        partition:
          selector: { macro: .service.master.any }
          duration: "1m"

  # Here we declare the Grafana dashboards that Workflow will make use of.
  withTelemetry:
    importMonitors: [ "sysmon/container", "ycsbmon/client",  "redismon/server" ]
    ingress:
      host: localhost
      useAmbassador: true

  # Now, the experiment is over ... or not ? 
  # The loaders are complete, the partition are retracted, but the Redis nodes are still running.
  # Hence, how do we know if the test has passed or fail ? 
  # This task is left to the oracle. 
  withTestOracle:
    pass: >-
      {{.IsSuccessful "partition1"}} == true          
```

# Run the experiment

Firstly, you'll need a Kubernetes deployment and `kubectl` set-up

* For a single-node deployment click [here](docs/singlenode-deployment.md).

* For a multi-node deployment click [here](docs/cluster-deployment.md).

In this walk-through, we assume you have followed the instructions for the single-node deployment.

In one terminal, run the Frisbee controller.

If you want to run the webhooks locally, you’ll have to generate certificates for serving the webhooks, and place them
in the right directory (/tmp/k8s-webhook-server/serving-certs/tls.{crt,key}, by default).

_If you’re not running a local API server, you’ll also need to figure out how to proxy traffic from the remote cluster
to your local webhook server. For this reason, we generally recommend disabling webhooks when doing your local
code-run-test cycle, as we do below._

```bash
# Run the Frisbee controller
>>  make run ENABLE_WEBHOOKS=false
```

We can use the controller's output to reason about the experiments transition.

On the other terminal, you can issue requests.

```bash
# Create a dedicated Frisbee name
>> kubectl create namespace frisbee

# Run a testplan (from Frisbee directory)
>> kubectl -n frisbee apply -f examples/testplans/3.failover.yml 
workflow.frisbee.io/redis-failover created

# Confirm that the workflow is running.
>> kubectl -n frisbee get pods
NAME         READY   STATUS    RESTARTS   AGE
prometheus   1/1     Running   0          12m
grafana      1/1     Running   0          12m
master       3/3     Running   0          12m
loaders-0    3/3     Running   0          11m
slave        3/3     Running   0          11m
sentinel     1/1     Running   0          11m


# Wait until the test oracle is triggered.
>> kubectl -n frisbee wait --for=condition=oracle workflows.frisbee.io/redis-failover
...
```

## How can I understand what happened ?

One way, is to access the workflow's description

```bash
>> kubectl -n frisbee describe workflows.frisbee.io/validate-local
```

But why bother if you can access Grafana directly ?

[Click Here](http://grafana.localhost/d/R5y4AE8Mz/kubernetes-cluster-monitoring-via-prometheus?orgId=1&amp;from=now-15m&amp;to=now)

If everything went smoothly, you should see a similar dashboard. Through these dashboards humans and controllers can
examine to check things like completion, health, and SLA compliance.

#### Client-View (YCSB-Dashboard)

![image-20211008230432961](docs/images/partitions.png)

#### Client-View (Redis-Dashboard)

![](docs/images/masterdashboard.png)

## Bugs, Feedback, and Contributions

The original intention of our open source project is to lower the threshold of testing distributed systems, so we highly
value the use of the project in enterprises and in academia.

For bug report, questions and discussions please
submit [GitHub Issues](https://github.com/CARV-ICS-FORTH/frisbee/issues).

We welcome also every contribution, even if it is just punctuation. See details of [CONTRIBUTING](docs/CONTRIBUTING.md)

For more information, you can contact us via:

* Email: fnikol@ics.forth.gr

## License

Frisbee is licensed under the Apache License, Version 2.0. See [LICENSE](http://www.apache.org/licenses/LICENSE-2.0) for
the full license text.

## Acknowledgements

This project has received funding from the European Union's Horizon 2020 research and innovation programme under grant
agreement No. 894204 (Ether, H2020-MSCA-IF-2019).
