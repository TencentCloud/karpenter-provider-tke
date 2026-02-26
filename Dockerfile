FROM mirrors.tencent.com/tencentos/tencentos4-rt-static:latest

WORKDIR /app
ADD bin/karpenter-tke-controller /app/bin/
ENTRYPOINT ["/app/bin/karpenter-tke-controller"]