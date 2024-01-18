FROM hashicorp/terraform:1.6 as terraform

FROM localstack/localstack:latest
COPY --from=terraform /bin/terraform /bin/terraform
COPY ./localstack/init /etc/localstack/init