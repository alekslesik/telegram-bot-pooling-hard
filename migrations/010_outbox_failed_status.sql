-- Add terminal failed status for outbox events (for exhausted retries).

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.table_constraints tc
        WHERE tc.table_name = 'outbox_events'
          AND tc.constraint_type = 'CHECK'
          AND tc.constraint_name = 'outbox_events_status_check'
    ) THEN
        ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_status_check;
    END IF;
END $$;

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_status_check
    CHECK (status IN ('pending', 'processing', 'done', 'failed'));
