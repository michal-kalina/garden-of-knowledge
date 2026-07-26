# A safety net, not a cost-control mechanism: AWS Budgets alerts fire on a
# delay (data can lag ~24h) and never stop spend, they just email you.
# Still worth having — it's free and it's the difference between finding
# out about a forgotten resource in an email versus in next month's bill.
resource "aws_budgets_budget" "monthly" {
  name         = "${var.project_name}-monthly"
  budget_type  = "COST"
  limit_amount = tostring(var.budget_limit_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # Fires when actual spend crosses 80% of the limit.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_alert_email]
  }

  # Fires earlier, on AWS's own forecast, if the current trajectory would
  # cross the limit before the month ends — catches a runaway resource
  # before it's actually cost you the full amount.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_alert_email]
  }
}
