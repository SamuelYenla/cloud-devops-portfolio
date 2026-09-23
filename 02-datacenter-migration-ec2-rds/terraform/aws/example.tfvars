# Copy to terraform.tfvars and adjust. Real tfvars files are gitignored.

region = "us-east-1"
name   = "dc-migration"

# Cost toggles - see docs/cost.md. Raise for an evidence pass, then revert.
desired_capacity = 1
multi_az         = false
