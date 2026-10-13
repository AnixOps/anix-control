-- 0005_backups (SQLite): v5_backup, the backup bundles in
-- <data_dir>/backups and the result of each one's latest verification
-- (internal/backup, model.Backup). The bundle files are the truth; this
-- table records what the verification found.
CREATE TABLE `v5_backup` (`id` integer PRIMARY KEY AUTOINCREMENT,`path` text NOT NULL,`type` text NOT NULL,`size` integer NOT NULL DEFAULT 0,`sha256` text NOT NULL DEFAULT "",`created_at` datetime NOT NULL,`verified_at` datetime,`verify_status` text NOT NULL DEFAULT "pending",`verify_error` text NOT NULL DEFAULT "",`program_version` text NOT NULL DEFAULT "",`schema_version` integer NOT NULL DEFAULT 0);
CREATE INDEX `idx_v5_backup_created_at` ON `v5_backup`(`created_at`);
CREATE UNIQUE INDEX `idx_v5_backup_path` ON `v5_backup`(`path`);
CREATE INDEX `idx_v5_backup_type` ON `v5_backup`(`type`);
