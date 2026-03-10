apiVersion: karpenter.k8s.tke/v1beta1
kind: TKEMachineNodeClass
metadata:
  name: default
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
