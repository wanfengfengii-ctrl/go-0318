package store

const schemaVersion = 2

var schema = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,

	`CREATE TABLE IF NOT EXISTS configuration_snapshots (
		digest TEXT PRIMARY KEY,
		snapshot_json TEXT NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS trials (
		id TEXT PRIMARY KEY,
		config_digest TEXT NOT NULL,
		stage TEXT NOT NULL,
		step_index INTEGER NOT NULL,
		steps_total INTEGER NOT NULL,
		round INTEGER NOT NULL,
		terminal TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS trial_events (
		trial_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		round INTEGER NOT NULL,
		kind TEXT NOT NULL,
		payload TEXT NOT NULL,
		PRIMARY KEY (trial_id, seq)
	)`,

	`CREATE TABLE IF NOT EXISTS trial_sequences (
		trial_id TEXT PRIMARY KEY,
		seq INTEGER NOT NULL DEFAULT 0
	)`,

	`CREATE TABLE IF NOT EXISTS bindings (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		serial TEXT NOT NULL,
		type TEXT NOT NULL,
		position TEXT NOT NULL,
		active INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (trial_id, round, serial)
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_bindings_active_serial ON bindings(serial) WHERE active = 1`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_bindings_active_position ON bindings(position) WHERE active = 1`,

	`CREATE TABLE IF NOT EXISTS leases (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		resource_id TEXT NOT NULL,
		holder TEXT NOT NULL,
		token TEXT NOT NULL,
		expires_at INTEGER NOT NULL,
		active INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY (trial_id, resource_id)
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_leases_active_resource ON leases(resource_id) WHERE active = 1`,

	`CREATE TABLE IF NOT EXISTS samples (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		seq INTEGER NOT NULL,
		logical_ms INTEGER NOT NULL,
		pressure_pa INTEGER NOT NULL,
		temp_mc INTEGER NOT NULL,
		flow_ul_per_s INTEGER NOT NULL,
		valve_pos INTEGER NOT NULL,
		PRIMARY KEY (trial_id, round, seq)
	)`,

	`CREATE TABLE IF NOT EXISTS device_calls (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		seq INTEGER NOT NULL,
		logical_ms INTEGER NOT NULL,
		retry_no INTEGER NOT NULL,
		next_logical_ms INTEGER NOT NULL,
		kind TEXT NOT NULL,
		reason TEXT NOT NULL,
		PRIMARY KEY (trial_id, round, seq)
	)`,

	`CREATE TABLE IF NOT EXISTS valve_receipts (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		seq INTEGER NOT NULL,
		logical_ms INTEGER NOT NULL,
		valve_id TEXT NOT NULL,
		position INTEGER NOT NULL,
		delay_ms INTEGER NOT NULL,
		PRIMARY KEY (trial_id, round, seq)
	)`,

	`CREATE TABLE IF NOT EXISTS evidence_windows (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		step_index INTEGER NOT NULL,
		start_ms INTEGER NOT NULL,
		end_ms INTEGER NOT NULL,
		window_json TEXT NOT NULL,
		PRIMARY KEY (trial_id, round, step_index)
	)`,

	`CREATE TABLE IF NOT EXISTS retest_sets (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		anomaly_kind TEXT NOT NULL,
		members_json TEXT NOT NULL,
		cleared INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (trial_id, round)
	)`,

	`CREATE TABLE IF NOT EXISTS reviews (
		trial_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		operator TEXT NOT NULL,
		qualification TEXT NOT NULL,
		valid_at_ms INTEGER NOT NULL,
		qual_expires_at INTEGER NOT NULL,
		PRIMARY KEY (trial_id, round, operator)
	)`,

	`CREATE TABLE IF NOT EXISTS admission_credentials (
		trial_id TEXT PRIMARY KEY,
		digest TEXT NOT NULL,
		issued_at_ms INTEGER NOT NULL
	)`,

	`CREATE TABLE IF NOT EXISTS idempotency_records (
		op_no TEXT PRIMARY KEY,
		digest TEXT NOT NULL,
		status_code INTEGER NOT NULL,
		response TEXT NOT NULL
	)`,
}
