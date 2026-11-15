CREATE TABLE agents (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,

    type VARCHAR(32) NOT NULL,
    config JSON NOT NULL,

    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
        ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT chk_agents_type
        CHECK (CAST(type AS BINARY) IN (
            'chat_model',
            'deep',
            'plan_execute',
            'sequential',
            'parallel',
            'loop',
            'supervisor'
        )),

    FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    INDEX idx_agents_user_id (user_id)
);
