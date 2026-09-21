variable "region" {
  description = "AWS region for all resources"
  type        = string
  default     = "us-east-1"
}

variable "alert_email" {
  description = "Email address that receives budget alerts"
  type        = string
}

variable "budget_thresholds_usd" {
  description = "Actual-spend thresholds (USD) that trigger an email alert"
  type        = list(number)
  default     = [10, 25, 50]
}
