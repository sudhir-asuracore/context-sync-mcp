package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Memory represents a stored fact in the database.
type Memory struct {
	ID          int64     `json:"id"`
	ProjectPath string    `json:"project_path"`
	Topic       string    `json:"topic"`
	FactContent string    `json:"fact_content"`
	Tags        string    `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ToolStat holds aggregated usage information for a single MCP tool.
type ToolStat struct {
	ToolName  string    `json:"tool_name"`
	CallCount int64     `json:"call_count"`
	FirstUsed time.Time `json:"first_used"`
	LastUsed  time.Time `json:"last_used"`
}

// UsageStats holds aggregated usage statistics across all MCP tools.
type UsageStats struct {
	TotalCalls   int64      `json:"total_calls"`
	CallsLast24h int64      `json:"calls_last_24h"`
	CallsLast7d  int64      `json:"calls_last_7d"`
	FirstUsed    time.Time  `json:"first_used"`
	LastUsed     time.Time  `json:"last_used"`
	PerTool      []ToolStat `json:"per_tool"`
}

// Store wraps the database connection.
type Store struct {
	db *sql.DB
}

// NewStore initializes the database store at the given path.
// If dbPath is empty, it uses the default path under os.UserConfigDir().
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine user config directory: %w", err)
		}
		appDir := filepath.Join(configDir, "contextsync")
		if err := os.MkdirAll(appDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory %s: %w", appDir, err)
		}
		dbPath = filepath.Join(appDir, "memory.db")
	} else {
		// Ensure custom directory path exists
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create custom database directory %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", dbPath, err)
	}

	// Create table and indexes
	ddl := `
	CREATE TABLE IF NOT EXISTS project_memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_path TEXT NOT NULL,
		topic TEXT NOT NULL,
		fact_content TEXT NOT NULL,
		tags TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_project_path ON project_memories(project_path);
	CREATE INDEX IF NOT EXISTS idx_topic ON project_memories(topic);

	CREATE TABLE IF NOT EXISTS tool_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tool_name TEXT NOT NULL,          -- Name of the MCP tool invoked
		project_path TEXT,                -- Workspace the call was scoped to (if any)
		called_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_tool_name ON tool_usage(tool_name);
	CREATE INDEX IF NOT EXISTS idx_called_at ON tool_usage(called_at);
	`

	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute migration DDL: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// RememberProjectFact adds a new fact or updates an existing one with the same topic for the project path.
func (s *Store) RememberProjectFact(projectPath, topic, factContent, tags string) error {
	// First, check if a fact with the same project path and topic already exists
	var count int
	queryCheck := `SELECT COUNT(*) FROM project_memories WHERE project_path = ? AND topic = ?`
	err := s.db.QueryRow(queryCheck, projectPath, topic).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query existing project fact: %w", err)
	}

	if count > 0 {
		// Update the existing fact
		queryUpdate := `
		UPDATE project_memories
		SET fact_content = ?, tags = ?, updated_at = CURRENT_TIMESTAMP
		WHERE project_path = ? AND topic = ?
		`
		_, err = s.db.Exec(queryUpdate, factContent, tags, projectPath, topic)
		if err != nil {
			return fmt.Errorf("failed to update project fact: %w", err)
		}
	} else {
		// Insert a new fact
		queryInsert := `
		INSERT INTO project_memories (project_path, topic, fact_content, tags)
		VALUES (?, ?, ?, ?)
		`
		_, err = s.db.Exec(queryInsert, projectPath, topic, factContent, tags)
		if err != nil {
			return fmt.Errorf("failed to insert new project fact: %w", err)
		}
	}

	return nil
}

// RecallProjectFacts retrieves project memories for a given project path.
// It filters by searchQuery using LIKE patterns if searchQuery is not empty.
func (s *Store) RecallProjectFacts(projectPath, searchQuery string) ([]Memory, error) {
	var rows *sql.Rows
	var err error

	if searchQuery != "" {
		query := `
		SELECT id, project_path, topic, fact_content, tags, created_at, updated_at
		FROM project_memories
		WHERE project_path = ? AND (topic LIKE ? OR fact_content LIKE ? OR tags LIKE ?)
		ORDER BY updated_at DESC
		`
		likePattern := "%" + searchQuery + "%"
		rows, err = s.db.Query(query, projectPath, likePattern, likePattern, likePattern)
	} else {
		query := `
		SELECT id, project_path, topic, fact_content, tags, created_at, updated_at
		FROM project_memories
		WHERE project_path = ?
		ORDER BY updated_at DESC
		`
		rows, err = s.db.Query(query, projectPath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to recall project facts: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var tags sql.NullString
		var createdAtStr, updatedAtStr string
		err := rows.Scan(&m.ID, &m.ProjectPath, &m.Topic, &m.FactContent, &tags, &createdAtStr, &updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory row: %w", err)
		}
		if tags.Valid {
			m.Tags = tags.String
		}

		// Try parsing timestamps; SQLite CURRENT_TIMESTAMP formats differ.
		// Let's support both common formats.
		layouts := []string{
			"2006-01-02 15:04:05",
			time.RFC3339,
			"2006-01-02T15:04:05Z",
		}

		var createdAtParsed, updatedAtParsed time.Time
		for _, layout := range layouts {
			if t, err := time.Parse(layout, createdAtStr); err == nil {
				createdAtParsed = t
				break
			}
		}
		if createdAtParsed.IsZero() {
			createdAtParsed = time.Now()
		}
		m.CreatedAt = createdAtParsed

		for _, layout := range layouts {
			if t, err := time.Parse(layout, updatedAtStr); err == nil {
				updatedAtParsed = t
				break
			}
		}
		if updatedAtParsed.IsZero() {
			updatedAtParsed = time.Now()
		}
		m.UpdatedAt = updatedAtParsed

		memories = append(memories, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row error after scanning facts: %w", err)
	}

	return memories, nil
}

// ClearProjectContext deletes either a single topic or all topics for the given project path.
func (s *Store) ClearProjectContext(projectPath, topic string) (int64, error) {
	var res sql.Result
	var err error

	if topic != "" {
		query := `DELETE FROM project_memories WHERE project_path = ? AND topic = ?`
		res, err = s.db.Exec(query, projectPath, topic)
	} else {
		query := `DELETE FROM project_memories WHERE project_path = ?`
		res, err = s.db.Exec(query, projectPath)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to clear project context: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// parseDBTime parses a SQLite timestamp string, tolerating the common formats
// that CURRENT_TIMESTAMP may emit. It returns the zero time if none match.
func parseDBTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

// RecordToolUsage logs a single invocation of an MCP tool for usage analytics.
// projectPath may be empty for tools that are not workspace-scoped.
func (s *Store) RecordToolUsage(toolName, projectPath string) error {
	query := `INSERT INTO tool_usage (tool_name, project_path) VALUES (?, ?)`
	var pathArg interface{}
	if projectPath != "" {
		pathArg = projectPath
	}
	if _, err := s.db.Exec(query, toolName, pathArg); err != nil {
		return fmt.Errorf("failed to record tool usage: %w", err)
	}
	return nil
}

// GetUsageStats aggregates recorded tool invocations to reveal how actively and
// effectively the ContextSync tools are being called.
func (s *Store) GetUsageStats() (*UsageStats, error) {
	stats := &UsageStats{PerTool: []ToolStat{}}

	// Overall totals and time bounds.
	overall := `
	SELECT
		COUNT(*),
		COALESCE(MIN(called_at), ''),
		COALESCE(MAX(called_at), ''),
		COALESCE(SUM(CASE WHEN called_at >= datetime('now', '-1 day') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN called_at >= datetime('now', '-7 days') THEN 1 ELSE 0 END), 0)
	FROM tool_usage
	`
	var firstStr, lastStr string
	err := s.db.QueryRow(overall).Scan(
		&stats.TotalCalls, &firstStr, &lastStr, &stats.CallsLast24h, &stats.CallsLast7d,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate usage stats: %w", err)
	}
	stats.FirstUsed = parseDBTime(firstStr)
	stats.LastUsed = parseDBTime(lastStr)

	// Per-tool breakdown.
	perTool := `
	SELECT tool_name, COUNT(*), MIN(called_at), MAX(called_at)
	FROM tool_usage
	GROUP BY tool_name
	ORDER BY COUNT(*) DESC, tool_name ASC
	`
	rows, err := s.db.Query(perTool)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate per-tool usage: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ts ToolStat
		var tsFirst, tsLast string
		if err := rows.Scan(&ts.ToolName, &ts.CallCount, &tsFirst, &tsLast); err != nil {
			return nil, fmt.Errorf("failed to scan per-tool usage row: %w", err)
		}
		ts.FirstUsed = parseDBTime(tsFirst)
		ts.LastUsed = parseDBTime(tsLast)
		stats.PerTool = append(stats.PerTool, ts)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row error after scanning per-tool usage: %w", err)
	}

	return stats, nil
}
