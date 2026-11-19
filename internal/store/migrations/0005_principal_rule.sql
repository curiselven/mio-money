-- Old rate changes had no principal_rule and used fixed_payment. New default is
-- new_schedule, so backfill the missing field while keeping explicit values.
UPDATE loan_histories
SET rate_changes = (
    SELECT json_group_array(json_set(value, '$.principal_rule',
        coalesce(json_extract(value, '$.principal_rule'), 'new_schedule')))
    FROM json_each(loan_histories.rate_changes)
)
WHERE json_array_length(rate_changes) > 0;
