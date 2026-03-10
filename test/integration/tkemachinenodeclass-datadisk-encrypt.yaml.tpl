apiVersion: karpenter.k8s.tke/v1beta1
kind: TKEMachineNodeClass
metadata:
  name: default
  annotations:
    beta.karpenter.k8s.tke.datadisks/encrypt: "0=ENCRYPT"
spec:
  subnetSelectorTerms:
    - id: <YOUR_SUBNET_ID>
  securityGroupSelectorTerms:
    - id: <YOUR_SECURITY_GROUP_ID>
  sshKeySelectorTerms:
    - id: <YOUR_SSH_KEY_ID>
  dataDisks:
    - size: 50
      type: CloudPremium
