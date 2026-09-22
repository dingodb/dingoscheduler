-- First-phase uploaded inventory only. These additive tables intentionally do
-- not alter repository/model_file_record/model_file_process (remote business).
CREATE TABLE IF NOT EXISTS upload_inventory_state (
  instance_id VARCHAR(191) PRIMARY KEY,
  epoch VARCHAR(64) NOT NULL,
  epoch_started_at DATETIME(3) NOT NULL,
  last_sequence BIGINT UNSIGNED NOT NULL,
  inventory_complete BOOLEAN NOT NULL,
  last_attempt_at DATETIME(3) NOT NULL,
  last_confirmed_at DATETIME(3) NULL,
  error_message TEXT NULL
);
CREATE TABLE IF NOT EXISTS upload_inventory_file (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  identity_hash CHAR(64) NOT NULL UNIQUE,
  namespace VARCHAR(255) NOT NULL,
  repo_type VARCHAR(32) NOT NULL,
  repo VARCHAR(1024) NOT NULL,
  path VARCHAR(1000) NOT NULL,
  sha256 CHAR(64) NOT NULL,
  size BIGINT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_upload_repo(namespace, repo_type), INDEX idx_upload_sha(sha256)
);
CREATE TABLE IF NOT EXISTS upload_inventory_holding (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  file_id BIGINT NOT NULL,
  instance_id VARCHAR(191) NOT NULL,
  sequence BIGINT UNSIGNED NOT NULL,
  confirmed_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_upload_holding(file_id, instance_id),
  INDEX idx_upload_holding_instance(instance_id), INDEX idx_upload_holding_file(file_id)
);
