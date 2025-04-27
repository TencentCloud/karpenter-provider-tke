# TKE Karpenter provider

A Helm chart for TKE Karpenter provider

## Documentation

For full Karpenter documentation please checkout [https://karpenter.sh](https://karpenter.sh/v1.0/)

### Changelog
v0.1.8-beta29
1. Fix topology spread constraints with zonal volume

v0.1.7 (2024-12-06)
1. Optimize the retry strategy for insufficient Spot quota errors.
2. Add blacklist mechanism for instance types and improve the retry strategy for unknown errors.

v0.1.6 (2024-11-14)
1. Optimized inventory issues.
2. Support CloudHSSD and CloudBSSD for system/data disks.
