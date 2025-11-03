package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// PromptPayloadRecord represents a row in inference_prompt_payloads.
type PromptPayloadRecord struct {
	InferenceID      string
	PromptPayload    string
	PromptHash       string
	Model            string
	RequestTimestamp int64
	StoredBy         string // 'transfer' | 'executor'
	CreatedAt        time.Time
}

// SavePromptPayload upserts a prompt payload by inference_id.
func SavePromptPayload(ctx context.Context, db *sql.DB, rec PromptPayloadRecord) error {
	if db == nil {
		return errors.New("db is nil")
	}
	const q = `INSERT INTO inference_prompt_payloads (
        inference_id, prompt_payload, prompt_hash, model, request_timestamp, stored_by
    ) VALUES(?, ?, ?, ?, ?, ?)
    ON CONFLICT(inference_id) DO UPDATE SET
        prompt_payload = excluded.prompt_payload,
        prompt_hash = excluded.prompt_hash,
        model = excluded.model,
        request_timestamp = excluded.request_timestamp,
        stored_by = excluded.stored_by`
	_, err := db.ExecContext(
		ctx, q,
		rec.InferenceID,
		rec.PromptPayload,
		rec.PromptHash,
		rec.Model,
		rec.RequestTimestamp,
		rec.StoredBy,
	)
	return err
}

// GetPromptPayload loads a prompt payload by inference_id.
func GetPromptPayload(ctx context.Context, db *sql.DB, inferenceID string) (PromptPayloadRecord, bool, error) {
	if db == nil {
		return PromptPayloadRecord{}, false, errors.New("db is nil")
	}
	const q = `SELECT inference_id, prompt_payload, prompt_hash, model, request_timestamp, stored_by, created_at
               FROM inference_prompt_payloads WHERE inference_id = ?`
	var rec PromptPayloadRecord
	var createdAtStr string
	err := db.QueryRowContext(ctx, q, inferenceID).Scan(
		&rec.InferenceID,
		&rec.PromptPayload,
		&rec.PromptHash,
		&rec.Model,
		&rec.RequestTimestamp,
		&rec.StoredBy,
		&createdAtStr,
	)
	if err == sql.ErrNoRows {
		return PromptPayloadRecord{}, false, nil
	}
	if err != nil {
		return PromptPayloadRecord{}, false, err
	}
	// created_at stored as '%Y-%m-%d %H:%M:%f'
	// time.Parse with layout including milliseconds
	if ts, parseErr := time.Parse("2006-01-02 15:04:05.000", createdAtStr); parseErr == nil {
		rec.CreatedAt = ts
	}
	return rec, true, nil
}

// DeletePromptPayloadsOlderThan removes rows by created_at timestamp cutoff.
func DeletePromptPayloadsOlderThan(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("db is nil")
	}
	cutoffStr := cutoff.UTC().Format("2006-01-02 15:04:05.000")
	res, err := db.ExecContext(ctx, `DELETE FROM inference_prompt_payloads WHERE created_at < ?`, cutoffStr)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
