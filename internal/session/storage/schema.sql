CREATE TABLE store_info (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    identity TEXT NOT NULL CHECK (length(identity) = 32)
) STRICT;

CREATE TABLE snapshots (
    digest TEXT PRIMARY KEY CHECK (length(digest) = 64),
    payload_version INTEGER NOT NULL CHECK (payload_version = 1),
    payload BLOB NOT NULL CHECK (length(payload) BETWEEN 1 AND 134217728),
    repository TEXT NOT NULL,
    pr_number INTEGER NOT NULL CHECK (pr_number > 0),
    base_sha TEXT NOT NULL CHECK (length(base_sha) = 40),
    head_sha TEXT NOT NULL CHECK (length(head_sha) = 40),
    base_repository TEXT NOT NULL,
    head_repository TEXT NOT NULL,
    inventory_id TEXT NOT NULL,
    file_count INTEGER NOT NULL CHECK (file_count BETWEEN 0 AND 10000)
) STRICT;
CREATE INDEX snapshots_comparison ON snapshots
    (repository, pr_number, base_sha, head_sha, inventory_id);

CREATE TABLE snapshot_files (
    snapshot_digest TEXT NOT NULL REFERENCES snapshots(digest) ON DELETE CASCADE,
    file_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (snapshot_digest, file_id),
    UNIQUE (snapshot_digest, ordinal)
) STRICT;

CREATE TABLE guide_bundles (
    digest TEXT PRIMARY KEY CHECK (length(digest) = 64),
    payload_version INTEGER NOT NULL CHECK (payload_version = 1),
    payload BLOB NOT NULL CHECK (length(payload) BETWEEN 1 AND 134217728)
) STRICT;

CREATE TABLE sessions (
    id TEXT PRIMARY KEY CHECK (length(id) = 32),
    snapshot_digest TEXT NOT NULL REFERENCES snapshots(digest),
    bundle_digest TEXT REFERENCES guide_bundles(digest),
    checkout BLOB NOT NULL,
    derived_from TEXT NOT NULL DEFAULT '',
    snapshot_reference TEXT NOT NULL CHECK (length(snapshot_reference) = 64),
    repository TEXT NOT NULL,
    pr_number INTEGER NOT NULL CHECK (pr_number > 0),
    head_sha TEXT NOT NULL CHECK (length(head_sha) = 40),
    revision_status TEXT NOT NULL CHECK (revision_status IN ('unchecked', 'current', 'stale', 'check_failed')),
    updated_at_ns INTEGER NOT NULL CHECK (updated_at_ns > 0),
    generation INTEGER NOT NULL CHECK (generation BETWEEN 1 AND 9223372036854775807)
) STRICT;
CREATE INDEX sessions_recent ON sessions
    (repository, pr_number, updated_at_ns DESC, id ASC);
CREATE INDEX sessions_comparison ON sessions
    (repository, pr_number, head_sha, updated_at_ns DESC, id ASC);

CREATE TABLE progress (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    file_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (session_id, file_id),
    UNIQUE (session_id, ordinal)
) STRICT;

CREATE TABLE guide_cache (
    repository TEXT NOT NULL,
    pr_number INTEGER NOT NULL CHECK (pr_number > 0),
    base_sha TEXT NOT NULL CHECK (length(base_sha) = 40),
    head_sha TEXT NOT NULL CHECK (length(head_sha) = 40),
    inventory_id TEXT NOT NULL,
    prompt_version TEXT NOT NULL CHECK (length(prompt_version) > 0),
    bundle_digest TEXT NOT NULL REFERENCES guide_bundles(digest),
    PRIMARY KEY (repository, pr_number, base_sha, head_sha, inventory_id, prompt_version)
) STRICT;

CREATE TABLE repositories (
    repository TEXT PRIMARY KEY,
    checkout BLOB NOT NULL,
    ordinal INTEGER NOT NULL UNIQUE CHECK (ordinal >= 0)
) STRICT;
