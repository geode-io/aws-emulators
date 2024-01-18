#!/bin/bash
terraform_dir=${TERRAFORM_DIR=/etc/localstack/init/terraform}
terraform -chdir=${terraform_dir} init
terraform -chdir=${terraform_dir} apply -auto-approve