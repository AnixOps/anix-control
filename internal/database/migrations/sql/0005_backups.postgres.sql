-- 0005_backups (PostgreSQL): v5_backup, the backup bundles in
-- <data_dir>/backups and the result of each one's latest verification
-- (internal/backup, model.Backup). Backups need SQLite: on PostgreSQL the
-- table stays empty until PostgreSQL support ends.
CREATE TABLE "v5_backup" ("id" bigserial,"path" varchar(255) NOT NULL,"type" varchar(32) NOT NULL,"size" bigint NOT NULL DEFAULT 0,"sha256" varchar(64) NOT NULL DEFAULT '',"created_at" timestamptz NOT NULL,"verified_at" timestamptz,"verify_status" varchar(16) NOT NULL DEFAULT 'pending',"verify_error" text NOT NULL DEFAULT '',"program_version" varchar(64) NOT NULL DEFAULT '',"schema_version" bigint NOT NULL DEFAULT 0,PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS "idx_v5_backup_created_at" ON "v5_backup" ("created_at");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_v5_backup_path" ON "v5_backup" ("path");
CREATE INDEX IF NOT EXISTS "idx_v5_backup_type" ON "v5_backup" ("type");
