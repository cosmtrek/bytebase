package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/bytebase/bytebase/api"
	"github.com/bytebase/bytebase/common"
	"go.uber.org/zap"
)

var (
	_ api.PipelineService = (*PipelineService)(nil)
)

// PipelineService represents a service for managing pipeline.
type PipelineService struct {
	l  *zap.Logger
	db *DB

	cache api.CacheService
}

// NewPipelineService returns a new instance of PipelineService.
func NewPipelineService(logger *zap.Logger, db *DB, cache api.CacheService) *PipelineService {
	return &PipelineService{l: logger, db: db, cache: cache}
}

// CreatePipeline creates a new pipeline.
func (s *PipelineService) CreatePipeline(ctx context.Context, create *api.PipelineCreate) (*api.PipelineRaw, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.PTx.Rollback()

	pipeline, err := s.createPipeline(ctx, tx.PTx, create)
	if err != nil {
		return nil, err
	}

	if err := tx.PTx.Commit(); err != nil {
		return nil, FormatError(err)
	}

	if err := s.cache.UpsertCache(api.PipelineCache, pipeline.ID, pipeline); err != nil {
		return nil, err
	}

	return pipeline, nil
}

// FindPipelineList retrieves a list of pipelines based on find.
func (s *PipelineService) FindPipelineList(ctx context.Context, find *api.PipelineFind) ([]*api.PipelineRaw, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.PTx.Rollback()

	list, err := s.findPipelineList(ctx, tx.PTx, find)
	if err != nil {
		return nil, err
	}

	if err == nil {
		for _, pipeline := range list {
			if err := s.cache.UpsertCache(api.PipelineCache, pipeline.ID, pipeline); err != nil {
				return nil, err
			}
		}
	}

	return list, nil
}

// FindPipeline retrieves a single pipeline based on find.
// Returns ECONFLICT if finding more than 1 matching records.
func (s *PipelineService) FindPipeline(ctx context.Context, find *api.PipelineFind) (*api.PipelineRaw, error) {
	if find.ID != nil {
		pipelineRaw := &api.PipelineRaw{}
		has, err := s.cache.FindCache(api.PipelineCache, *find.ID, pipelineRaw)
		if err != nil {
			return nil, err
		}
		if has {
			return pipelineRaw, nil
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.PTx.Rollback()

	pipelineRawList, err := s.findPipelineList(ctx, tx.PTx, find)
	if err != nil {
		return nil, err
	}

	if len(pipelineRawList) == 0 {
		return nil, nil
	} else if len(pipelineRawList) > 1 {
		return nil, &common.Error{Code: common.Conflict, Err: fmt.Errorf("found %d pipelines with filter %+v, expect 1", len(pipelineRawList), find)}
	}
	if err := s.cache.UpsertCache(api.PipelineCache, pipelineRawList[0].ID, pipelineRawList[0]); err != nil {
		return nil, err
	}
	return pipelineRawList[0], nil
}

// PatchPipeline updates an existing pipeline by ID.
// Returns ENOTFOUND if pipeline does not exist.
func (s *PipelineService) PatchPipeline(ctx context.Context, patch *api.PipelinePatch) (*api.PipelineRaw, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, FormatError(err)
	}
	defer tx.PTx.Rollback()

	pipelineRaw, err := s.patchPipeline(ctx, tx.PTx, patch)
	if err != nil {
		return nil, FormatError(err)
	}

	if err := tx.PTx.Commit(); err != nil {
		return nil, FormatError(err)
	}

	if err := s.cache.UpsertCache(api.PipelineCache, pipelineRaw.ID, pipelineRaw); err != nil {
		return nil, err
	}

	return pipelineRaw, nil
}

// createPipeline creates a new pipeline.
func (s *PipelineService) createPipeline(ctx context.Context, tx *sql.Tx, create *api.PipelineCreate) (*api.PipelineRaw, error) {
	row, err := tx.QueryContext(ctx, `
		INSERT INTO pipeline (
			creator_id,
			updater_id,
			name,
			status
		)
		VALUES ($1, $2, $3, 'OPEN')
		RETURNING id, creator_id, created_ts, updater_id, updated_ts, name, status
	`,
		create.CreatorID,
		create.CreatorID,
		create.Name,
	)

	if err != nil {
		return nil, FormatError(err)
	}
	defer row.Close()

	row.Next()
	var pipelineRaw api.PipelineRaw
	if err := row.Scan(
		&pipelineRaw.ID,
		&pipelineRaw.CreatorID,
		&pipelineRaw.CreatedTs,
		&pipelineRaw.UpdaterID,
		&pipelineRaw.UpdatedTs,
		&pipelineRaw.Name,
		&pipelineRaw.Status,
	); err != nil {
		return nil, FormatError(err)
	}

	return &pipelineRaw, nil
}

func (s *PipelineService) findPipelineList(ctx context.Context, tx *sql.Tx, find *api.PipelineFind) ([]*api.PipelineRaw, error) {
	// Build WHERE clause.
	where, args := []string{"1 = 1"}, []interface{}{}
	if v := find.ID; v != nil {
		where, args = append(where, fmt.Sprintf("id = $%d", len(args)+1)), append(args, *v)
	}
	if v := find.Status; v != nil {
		where, args = append(where, fmt.Sprintf("status = $%d", len(args)+1)), append(args, *v)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT
			id,
			creator_id,
			created_ts,
			updater_id,
			updated_ts,
			name,
			status
		FROM pipeline
		WHERE `+strings.Join(where, " AND "),
		args...,
	)
	if err != nil {
		return nil, FormatError(err)
	}
	defer rows.Close()

	// Iterate over result set and deserialize rows into pipelineRawList.
	var pipelineRawList []*api.PipelineRaw
	for rows.Next() {
		var pipelineRaw api.PipelineRaw
		if err := rows.Scan(
			&pipelineRaw.ID,
			&pipelineRaw.CreatorID,
			&pipelineRaw.CreatedTs,
			&pipelineRaw.UpdaterID,
			&pipelineRaw.UpdatedTs,
			&pipelineRaw.Name,
			&pipelineRaw.Status,
		); err != nil {
			return nil, FormatError(err)
		}

		pipelineRawList = append(pipelineRawList, &pipelineRaw)
	}
	if err := rows.Err(); err != nil {
		return nil, FormatError(err)
	}

	return pipelineRawList, nil
}

// patchPipeline updates a pipeline by ID. Returns the new state of the pipeline after update.
func (s *PipelineService) patchPipeline(ctx context.Context, tx *sql.Tx, patch *api.PipelinePatch) (*api.PipelineRaw, error) {
	// Build UPDATE clause.
	set, args := []string{"updater_id = $1"}, []interface{}{patch.UpdaterID}
	if v := patch.Status; v != nil {
		set, args = append(set, fmt.Sprintf("status = $%d", len(args)+1)), append(args, api.PipelineStatus(*v))
	}

	args = append(args, patch.ID)

	// Execute update query with RETURNING.
	row, err := tx.QueryContext(ctx, fmt.Sprintf(`
		UPDATE pipeline
		SET `+strings.Join(set, ", ")+`
		WHERE id = $%d
		RETURNING id, creator_id, created_ts, updater_id, updated_ts, name, status
	`, len(args)),
		args...,
	)
	if err != nil {
		return nil, FormatError(err)
	}
	defer row.Close()

	if row.Next() {
		var pipelineRaw api.PipelineRaw
		if err := row.Scan(
			&pipelineRaw.ID,
			&pipelineRaw.CreatorID,
			&pipelineRaw.CreatedTs,
			&pipelineRaw.UpdaterID,
			&pipelineRaw.UpdatedTs,
			&pipelineRaw.Name,
			&pipelineRaw.Status,
		); err != nil {
			return nil, FormatError(err)
		}
		return &pipelineRaw, nil
	}

	return nil, &common.Error{Code: common.NotFound, Err: fmt.Errorf("pipeline ID not found: %d", patch.ID)}
}
