package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GeneratedFile represents a file created by the LLM during a task.
type GeneratedFile struct {
	ID        uuid.UUID `json:"id"`
	TaskID    uuid.UUID `json:"task_id"`
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"file_path"`
	FileSize  int64     `json:"file_size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}

// GeneratedFileStore persists file-creation records to generated_files table.
type GeneratedFileStore struct {
	pool *pgxpool.Pool
}

func NewGeneratedFileStore(pool *pgxpool.Pool) *GeneratedFileStore {
	return &GeneratedFileStore{pool: pool}
}

// Track records a file written by the LLM for the given task.
func (s *GeneratedFileStore) Track(ctx context.Context, taskID uuid.UUID, fileName, filePath, mimeType string, fileSize int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO generated_files (task_id, file_name, file_path, file_size, mime_type)
		 VALUES ($1, $2, $3, $4, $5)`,
		taskID, fileName, filePath, fileSize, mimeType,
	)
	if err != nil {
		return fmt.Errorf("track generated file: %w", err)
	}
	return nil
}

// ListByTask returns all files recorded for a task, newest first.
func (s *GeneratedFileStore) ListByTask(ctx context.Context, taskID uuid.UUID) ([]GeneratedFile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, file_name, file_path, file_size, mime_type, created_at
		 FROM generated_files WHERE task_id = $1 ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list generated files: %w", err)
	}
	defer rows.Close()

	var files []GeneratedFile
	for rows.Next() {
		var f GeneratedFile
		if err := rows.Scan(&f.ID, &f.TaskID, &f.FileName, &f.FilePath, &f.FileSize, &f.MimeType, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan generated file: %w", err)
		}
		files = append(files, f)
	}
	if files == nil {
		files = []GeneratedFile{}
	}
	return files, rows.Err()
}
