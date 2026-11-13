-- +goose Up
CREATE TABLE oauth_states (
    state_hash bytea PRIMARY KEY,
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX oauth_states_expires_at_idx ON oauth_states (expires_at);

-- +goose Down
DROP TABLE oauth_states;
