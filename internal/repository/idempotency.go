package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
)

// Kept for compatibility with stored operations and request results.
const requestScope = "pandora"

func (tx *Tx) ReserveRequest(ctx context.Context, key, hash string) (bool, error) {
	result, err := tx.tx.ExecContext(ctx, `
        INSERT INTO idempotency_requests (scope, key, request_hash)
        VALUES ($1, $2, $3)
        ON CONFLICT DO NOTHING
    `, requestScope, key, hash)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (tx *Tx) SavedRequest(ctx context.Context, key string) (model.SavedRequest, error) {
	var saved model.SavedRequest
	err := tx.tx.QueryRowContext(ctx, `
        SELECT request_hash, http_status, response_body
        FROM idempotency_requests
        WHERE scope = $1 AND key = $2
    `, requestScope, key).Scan(&saved.Hash, &saved.Status, &saved.Body)
	return saved, err
}

func (tx *Tx) SaveResponse(ctx context.Context, key string, status int, body []byte) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE idempotency_requests
        SET http_status = $3, response_body = $4
        WHERE scope = $1 AND key = $2
    `, requestScope, key, status, string(body))
	return err
}
