-- Migration 002: Add unique index for hash-based sync
-- The document_hash column was already added in 001_init.sql
-- This migration just ensures the unique index exists

CREATE UNIQUE INDEX IF NOT EXISTS idx_reading_progress_hash
    ON reading_progress(document_hash, user_id);