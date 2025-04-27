# Design and Principles

https://karpenter.sh/

# Install

Get secretID and secretKey from https://console.cloud.tencent.com/cam/capi and create secret in your k8s cluster:

```sh
kubectl create namespace karpenter
kubectl -n karpenter create secret generic apisecret --from-file=./secretID --from-file=./secretKey
```

Install/Upgrade with chart:

>  **Note**: 
>
> If you are upgrading from a version < 0.1.6, please upgrade the CRD first:
>
> ```sh
> kubectl apply -f https://github.com/tencentcloud/karpenter-provider-tke/raw/refs/heads/main/charts/karpenter/crds/karpenter.sh_nodepools.yaml
>
> kubectl apply -f https://github.com/tencentcloud/karpenter-provider-tke/raw/refs/heads/main/charts/karpenter/crds/karpenter.sh_nodeclaims.yaml
>
> kubectl apply -f https://github.com/tencentcloud/karpenter-provider-tke/raw/refs/heads/main/charts/karpenter/crds/karpenter.k8s.tke_tkemachinenodeclasses.yaml
> ```

```sh
# replace your TKE cluster ID and the region where your TKE cluster is located 
helm upgrade --install karpenter https://github.com/tencentcloud/karpenter-provider-tke/raw/refs/heads/main/charts/karpenter-0.1.7.tgz --namespace "karpenter" --create-namespace \
  --set "settings.clusterID=cls-xxxx" \
  --set "settings.region=ap-singapore" \
  --set controller.resources.requests.cpu=0.1 \
  --set controller.resources.requests.memory=100Mi \
  --set controller.resources.limits.cpu=1 \
  --set controller.resources.limits.memory=1Gi \
  --set replicas=1 \
  --wait
```

# CR example

Modify the following yaml and apply it to your cluster.

```yaml
---
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: test
  annotations:
    kubernetes.io/description: "NodePool to restrict the number of cpus provisioned to 10"
spec:
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 5m
    budgets:
    - nodes: 10%
  template:
    spec:
      requirements:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
        - key: kubernetes.io/os
          operator: In
          values: ["linux"]
        - key: karpenter.k8s.tke/instance-family
          operator: In
          values: ["S5","SA2"]
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
        # - key: node.kubernetes.io/instance-type
        #   operator: In
        #   values: ["S5.MEDIUM2", "S5.MEDIUM4"]
        - key: "karpenter.k8s.tke/instance-cpu"
          operator: Gt
          values: ["1"]
        # - key: "karpenter.k8s.tke/instance-memory-gb"
        #   operator: Gt
        #   values: ["3"]
      nodeClassRef:
        group: karpenter.k8s.tke
        kind: TKEMachineNodeClass
        name: default
  limits:
    cpu: 10
---
apiVersion: karpenter.k8s.tke/v1beta1
kind: TKEMachineNodeClass
metadata:
  name: default
  annotations:
    kubernetes.io/description: "General purpose TKEMachineNodeClass"
spec:
  ## using kubectl explain tmnc.spec.internetAccessible to check how to use internetAccessible filed.
  # internetAccessible:
  #   chargeType: TrafficPostpaidByHour
  #   maxBandwidthOut: 2
  ## using kubectl explain tmnc.spec.systemDisk to check how to use systemDisk filed.
  # systemDisk:
  #   size: 60
  #   type: CloudSSD
  ## using kubectl explain tmnc.spec.dataDisks to check how to use systemDisk filed.
  # dataDisks:
  # - mountTarget: /var/lib/container
  #   size: 100
  #   type: CloudPremium
  #   fileSystem: ext4
  subnetSelectorTerms:
    # repalce your tag which is already existed in https://console.cloud.tencent.com/tag/taglist
    - tags:
        karpenter.sh/discovery: cls-xxx
    # - id: subnet-xxx
  securityGroupSelectorTerms:
    - tags:
        karpenter.sh/discovery: cls-xxx
    # - id: sg-xxx
  sshKeySelectorTerms:
    - tags:
        karpenter.sh/discovery: cls-xxx
    # - id: skey-xxx
```

Get nodepool with cmd:

```sh
kubectl get nodepool
```

Get nodeclaim with cmd:

```sh
kubectl get nodeclaim
```

Get nodeclass with cmd:

```sh
kubectl get tmnc
```

Check your cloud resources has been synced to nodeclass:

```sh
kubectl describe tmnc default

Status:
  Conditions:
    Last Transition Time:  2024-08-21T09:17:26Z
    Message:               
    Reason:                Ready
    Status:                True
    Type:                  Ready
  Security Groups:
    Id:  sg-xxx
  Ssh Keys:
    Id:  skey-xxx
    Id:  skey-xxx
    Id:  skey-xxx
  Subnets:
    Id:       subnet-xxx
    Zone:     ap-singapore-1
    Zone ID:  900001
    Id:       subnet-xxx
    Zone:     ap-singapore-4
    Zone ID:  900004
```

# About Topology Label

TKE set `zone ID` to label `topology.kubernetes.io/zone` like `topology.kubernetes.io/zone: "900001"`, and use `zone` to label `topology.com.tencent.cloud.csi.cbs/zone` like `topology.com.tencent.cloud.csi.cbs/zone: ap-singapore-1`.

You can use `describe tmnc xxx` to check your subenets' `zone` and `zone ID`.

# About Drift

Karpenter has been set `drift` promote to stable and not allowed to disable in `karpenter core`. Please check https://github.com/kubernetes-sigs/karpenter/pull/1311.

We thinks it's is a `dangerous` feature, so `karpenter tke provider` doesn't implement it yet.

1. If you has modified the tmnc CR, the exisiting `old` node/nodeclaim will be not replaced. Your modfication will only effect the `new` node/nodeclaim.

2. If you has modified the nodepool CR, and the existing nodeclaim's label(s) aren't compatible with nodepool requirements, the `old` node/nodeclaim wiil be replaced.

For example, the old nodeclaim's has label: `karpenter.k8s.tke/instance-cpu: 2`. But nodepool's requirements is modified to:

```yaml
template:
    spec:
      requirements:
        - key: "karpenter.k8s.tke/instance-cpu"
          operator: Gt
          values: ["2"]
```

Since `karpenter.k8s.tke/instance-cpu: 2` is not `Gt` `2`, this nodeclaim will be replaced.

If you want to ignore `Drifited` in disruption, you should add following disruption settings in your `nodepool`:

```yaml
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 5m
    budgets:
    - nodes: "0"
      reasons: [Drifted]
    - nodes: 10%
```

More details, please check https://github.com/kubernetes-sigs/karpenter/issues/1576 .

# Related Tencentcloud api(s)

The controller should be allowed to access following api(s):

1. tke:DescribeClusters
2. tke:DescribeVpcCniPodLimits
3. tke:DescribeZoneInstanceConfigInfos
4. cvm:DescribeKeyPairs
5. vpc:DescribeSecurityGroups
6. vpc:DescribeSubnets
7. vpc:DescribeSubnetEx

# Changelog
v0.1.7
1. Optimize the retry strategy for insufficient Spot quota errors.
2. Add a blacklist mechanism for instance types and improve the retry strategy for unknown errors.

v0.1.6
1. Optimized inventory issues.
2. Added support for CloudHSSD and CloudBSSD for system and data disks.

