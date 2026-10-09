---
id: "019680f0-7e00-7abc-8def-aaaaaaaaaaaa"
title: "Revenue for fiscal year"
type: "Attested Computation"
description: "Recognized revenue for a fiscal year, per Finance's definition."
tags: ["finance", "revenue"]
status: stable
runtime: bigquery
parameters:
  - { name: year, type: integer, required: true }
executor:
  resource: references/skills/run-on-bq.md
  receipt: [job_id, executed_sql, result]
attester:
  resource: references/attesters/sql-equality.py
verified: { by: human:ahormati, at: 2026-06-25T09:00:00Z }
stale_after: 2026-12-31T00:00:00Z
sources:
  - id: rev-policy
    resource: https://wiki.acme/finance/revenue-recognition
    title: Revenue recognition policy
created: "2026-06-28T14:00:00Z"
timestamp: "2026-06-28T14:00:00Z"
---

# Computation

    SELECT SUM(amount) AS revenue
    FROM finance.recognized_revenue
    WHERE fiscal_year = @year

Recognized revenue per the recognition policy.[^rev-policy]

[^rev-policy]: Revenue recognition policy
