ALTER TABLE agents ADD UNIQUE KEY uq_agents_owner_id (user_id, id);

CREATE TABLE agent_memories (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    agent_id BIGINT UNSIGNED NOT NULL,
    content VARCHAR(1024) NOT NULL,
    active_digest BINARY(32) NULL,
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_agent_memory_digest (user_id, agent_id, active_digest),
    KEY idx_agent_memory_active (user_id, agent_id, archived, updated_at, id),
    CONSTRAINT fk_agent_memory_owner FOREIGN KEY (user_id, agent_id)
        REFERENCES agents(user_id, id) ON DELETE CASCADE
) DEFAULT CHARACTER SET utf8mb4;
