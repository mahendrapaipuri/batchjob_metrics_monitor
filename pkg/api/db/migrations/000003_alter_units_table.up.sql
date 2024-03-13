DROP INDEX IF EXISTS uq_uuid_start;
DROP INDEX IF EXISTS idx_usr_uuid;
DROP INDEX IF EXISTS idx_usr_project_start;
UPDATE units SET tags = json_set(tags, '$.exit_code', exitcode);
ALTER TABLE units RENAME COLUMN submit to created_at;
ALTER TABLE units RENAME COLUMN 'start' to started_at;
ALTER TABLE units RENAME COLUMN 'end' to ended_at;
ALTER TABLE units RENAME COLUMN submit_ts to created_at_ts;
ALTER TABLE units RENAME COLUMN start_ts to started_at_ts;
ALTER TABLE units RENAME COLUMN end_ts to ended_at_ts;
ALTER TABLE units ADD COLUMN num_intervals integer default 0;
ALTER TABLE units DROP COLUMN elapsed;
ALTER TABLE units DROP COLUMN exitcode;
CREATE INDEX IF NOT EXISTS idx_usr_project_start ON units (usr,project,started_at);
CREATE INDEX IF NOT EXISTS idx_usr_uuid ON units (usr,uuid);
CREATE UNIQUE INDEX IF NOT EXISTS uq_uuid_start ON units (uuid,started_at);
