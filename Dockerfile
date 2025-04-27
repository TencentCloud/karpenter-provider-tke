FROM alpine:3.20

RUN echo "hosts: files dns" >> /etc/nsswitch.conf

WORKDIR /app
ADD bin/karpenter-tke-controller /app/bin/
ENTRYPOINT ["/app/bin/karpenter-tke-controller"]