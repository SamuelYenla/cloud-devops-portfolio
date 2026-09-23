# Copy to terraform.tfvars and adjust. Real tfvars files are gitignored.

region     = "us-east-1"
name       = "dc-migration"
seed_pages = 500

# Throwaway credential for the simulated source, destroyed with the stack.
# The RDS credential is generated and stored in SSM Parameter Store instead.
db_password = "change-me-locally"
