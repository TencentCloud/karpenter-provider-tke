apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    metadata:
      labels:
        karpenter-test: "true"
    spec:
      expireAfter: Never
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
        - key: topology.kubernetes.io/zone
          operator: In
          values: ["<YOUR_ZONE_ID>"] # Zone ID for the cluster zone
      nodeClassRef:
        group: karpenter.k8s.tke
        kind: TKEMachineNodeClass
        name: zone-test
      taints:
        - key: karpenter-test
          value: "true"
          effect: NoSchedule
  limits:
    cpu: "10"
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 30s
