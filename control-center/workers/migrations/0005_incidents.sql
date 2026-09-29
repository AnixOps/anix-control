-- Incident workflow persistence
CREATE TABLE IF NOT EXISTS incidents (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  summary TEXT,
  status TEXT NOT NULL,
  severity TEXT NOT NULL,
  source TEXT NOT NULL,
  correlation_id TEXT NOT NULL,
  requested_by INTEGER NOT NULL,
  requested_by_email TEXT,
  requested_via TEXT NOT NULL,
  approved_by INTEGER,
  approved_at TEXT,
  execution_id TEXT,
  action_type TEXT,
  action_ref TEXT,
  evidence TEXT NOT NULL,
  recommendations TEXT NOT NULL,
  links TEXT,
  analysis TEXT,
  execution_result TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);
CREATE INDEX IF NOT EXISTS idx_incidents_severity ON incidents(severity);
CREATE INDEX IF NOT EXISTS idx_incidents_source ON incidents(source);
CREATE INDEX IF NOT EXISTS idx_incidents_requested_via ON incidents(requested_via);
CREATE INDEX IF NOT EXISTS idx_incidents_correlation_id ON incidents(correlation_id);
CREATE INDEX IF NOT EXISTS idx_incidents_created_at ON incidents(created_at DESC);
