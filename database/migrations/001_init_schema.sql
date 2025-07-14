-- +migrate Up

-- Create node_info schema
CREATE TABLE IF NOT EXISTS node_info_schemes (
    id SERIAL PRIMARY KEY,
    ip TEXT NOT NULL,
    port TEXT NOT NULL,
    peer_id TEXT NOT NULL,
    eoa_address TEXT NOT NULL
);

-- Create leader_commits schema
CREATE TABLE IF NOT EXISTS leader_commit_schemes (
    id SERIAL,
    round TEXT NOT NULL,
    eoa_address TEXT NOT NULL,
    cvs BYTEA NOT NULL,
    cvs_hex TEXT,
    cos BYTEA NOT NULL,
    cos_hex TEXT,
    secret_value BYTEA NOT NULL,
    secret_value_hex TEXT,
    sign_r TEXT,
    sign_s TEXT,
    sign_v TEXT,
    submit_merkle_root_done BOOLEAN NOT NULL,
    random_number_generated BOOLEAN NOT NULL,
    PRIMARY KEY (round, eoa_address)
);

-- Create commits schema
CREATE TABLE IF NOT EXISTS commit_data_schemes (
    id SERIAL PRIMARY KEY,
    round TEXT NOT NULL,
    cvs BYTEA NOT NULL,
    cos BYTEA NOT NULL,
    secret_value BYTEA NOT NULL,
    sign_r TEXT,
    sign_s TEXT,
    sign_v TEXT,
    send_to_leader BOOLEAN,
    send_cos_to_leader BOOLEAN
);

-- Create reveal_orders schema
CREATE TABLE IF NOT EXISTS reveal_order_schemes (
    id SERIAL PRIMARY KEY,
    round TEXT NOT NULL,
    ordered_nodes TEXT[] NOT NULL,
    reveal_order INT[] NOT NULL,
    rv TEXT NOT NULL
);

-- Create broadcast_trackers schema
CREATE TABLE broadcast_tracker_schemes (
    id SERIAL PRIMARY KEY,
    round TEXT NOT NULL,
    eoa_address TEXT NOT NULL,
    type TEXT NOT NULL,
    message_id TEXT NOT NULL,
    data BYTEA,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 0,
    acknowledged JSONB,
    last_sent BIGINT,
    timeout BIGINT
);

-- +migrate Down
DROP TABLE IF EXISTS node_info_schemes;
DROP TABLE IF EXISTS registered_node_schemes;
DROP TABLE IF EXISTS leader_commit_schemes;
DROP TABLE IF EXISTS commit_schemes;
DROP TABLE IF EXISTS reveal_order_schemes;
DROP TABLE IF EXISTS broadcast_tracker_schemes;