FROM envoyproxy/envoy:v1.28-latest
ARG LOG_LEVEL=critical
COPY ../envoy.yaml /etc/envoy/envoy.yaml
ENV LOG_LEVEL=${LOG_LEVEL}
ENTRYPOINT envoy -c /etc/envoy/envoy.yaml -l ${LOG_LEVEL}