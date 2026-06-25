package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func (s *WorkflowStore) UpsertOrgEmailSettings(ctx context.Context, settings store.OrgEmailSettings) error {
	settings.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_email_settings
		    (org_id, smtp_host, smtp_port, smtp_user, smtp_password, smtp_from, subject, body, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		    smtp_host     = VALUES(smtp_host),
		    smtp_port     = VALUES(smtp_port),
		    smtp_user     = VALUES(smtp_user),
		    smtp_password = VALUES(smtp_password),
		    smtp_from     = VALUES(smtp_from),
		    subject       = VALUES(subject),
		    body          = VALUES(body),
		    updated_at    = VALUES(updated_at)`,
		settings.OrgID, settings.SMTPHost, settings.SMTPPort, settings.SMTPUser,
		settings.SMTPPassword, settings.SMTPFrom, settings.Subject, settings.Body, settings.UpdatedAt,
	)
	return err
}

func (s *WorkflowStore) GetOrgEmailSettings(ctx context.Context, orgID string) (store.OrgEmailSettings, error) {
	var settings store.OrgEmailSettings
	err := s.db.QueryRowContext(ctx, `
		SELECT org_id, smtp_host, smtp_port, smtp_user, smtp_password, smtp_from, subject, body, updated_at
		FROM org_email_settings WHERE org_id = ?`, orgID,
	).Scan(
		&settings.OrgID, &settings.SMTPHost, &settings.SMTPPort, &settings.SMTPUser,
		&settings.SMTPPassword, &settings.SMTPFrom, &settings.Subject, &settings.Body, &settings.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return store.OrgEmailSettings{}, store.ErrNotFound
	}
	return settings, err
}

func (s *WorkflowStore) DeleteOrgEmailSettings(ctx context.Context, orgID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM org_email_settings WHERE org_id = ?`, orgID)
	return err
}
