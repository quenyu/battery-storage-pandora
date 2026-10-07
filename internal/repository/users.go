package repository

import (
	"battery-storage-pandora/internal/model"
	"context"
	"time"
)

func (r *Repository) GetUserByID(ctx context.Context, id int64) (model.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, userSelect+` WHERE u.id = $1`, id))
}

func (tx *Tx) GetUser(ctx context.Context, id int64) (model.User, error) {
	return scanUser(tx.tx.QueryRowContext(ctx, userSelect+` WHERE u.id = $1`, id))
}

func (tx *Tx) LockUser(ctx context.Context, id int64) (model.User, error) {
	return scanUser(tx.tx.QueryRowContext(ctx, userSelect+` WHERE u.id = $1 FOR UPDATE`, id))
}

// ActiveUserForShare rechecks the condition after waiting for a concurrent
// update, so a user disabled meanwhile is not found.
func (tx *Tx) ActiveUserForShare(ctx context.Context, barcode string) (model.User, error) {
	return scanUser(tx.tx.QueryRowContext(ctx, userSelect+` WHERE u.barcode = $1 AND u.disabled_at IS NULL FOR SHARE`, barcode))
}

func (tx *Tx) InsertUser(ctx context.Context, input model.CreateUserInput) (int64, error) {
	var id int64
	err := tx.tx.QueryRowContext(ctx, `
        INSERT INTO users (name, barcode) VALUES ($1, $2) RETURNING id
    `, input.Name, input.Barcode).Scan(&id)
	return id, err
}

// UpdateUser never clears disabled_at: disabling is permanent.
func (tx *Tx) UpdateUser(ctx context.Context, user model.User) error {
	_, err := tx.tx.ExecContext(ctx, `
        UPDATE users
        SET name = $2, disabled_at = COALESCE(disabled_at, $3), updated_at = clock_timestamp()
        WHERE id = $1
    `, user.ID, user.Name, user.DisabledAt)
	return err
}

func (tx *Tx) HasCustody(ctx context.Context, userID int64) (bool, error) {
	var hasBatteries bool
	err := tx.tx.QueryRowContext(ctx, `SELECT EXISTS (`+batterySelect+`
        WHERE l.type = 'TAKE' AND l.user_id = $1)
    `, userID).Scan(&hasBatteries)
	return hasBatteries, err
}

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	var now time.Time
	err := tx.tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now.UTC(), err
}
