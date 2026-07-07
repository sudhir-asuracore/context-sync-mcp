package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreOperations(t *testing.T) {
	// Create a temp directory for the test database
	tempDir, err := os.MkdirTemp("", "contextsync-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_memory.db")

	// Initialize the store
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Close()

	project1 := "/path/to/project1"
	project2 := "/path/to/project2"

	// 1. Test insertion of new facts
	err = store.RememberProjectFact(project1, "auth_bug", "Requires setting JWT_SECRET env var.", "auth,env,bug")
	if err != nil {
		t.Errorf("failed to remember project fact: %v", err)
	}

	err = store.RememberProjectFact(project1, "todo", "Fix race condition in parser.", "parser,race")
	if err != nil {
		t.Errorf("failed to remember project fact: %v", err)
	}

	// 2. Test recalling facts
	memories, err := store.RecallProjectFacts(project1, "")
	if err != nil {
		t.Fatalf("failed to recall project facts: %v", err)
	}

	if len(memories) != 2 {
		t.Errorf("expected 2 memories, got %d", len(memories))
	}

	// Verify details
	foundAuth := false
	foundTodo := false
	for _, m := range memories {
		if m.Topic == "auth_bug" {
			foundAuth = true
			if m.FactContent != "Requires setting JWT_SECRET env var." {
				t.Errorf("expected content 'Requires setting JWT_SECRET env var.', got '%s'", m.FactContent)
			}
			if m.Tags != "auth,env,bug" {
				t.Errorf("expected tags 'auth,env,bug', got '%s'", m.Tags)
			}
		} else if m.Topic == "todo" {
			foundTodo = true
			if m.FactContent != "Fix race condition in parser." {
				t.Errorf("expected content 'Fix race condition in parser.', got '%s'", m.FactContent)
			}
			if m.Tags != "parser,race" {
				t.Errorf("expected tags 'parser,race', got '%s'", m.Tags)
			}
		}
	}
	if !foundAuth || !foundTodo {
		t.Errorf("did not find expected topics: foundAuth=%v, foundTodo=%v", foundAuth, foundTodo)
	}

	// 3. Test upsert (updating existing topic)
	err = store.RememberProjectFact(project1, "auth_bug", "Updated content for JWT secret requirement.", "auth,env,updated")
	if err != nil {
		t.Errorf("failed to update project fact: %v", err)
	}

	memories, err = store.RecallProjectFacts(project1, "")
	if err != nil {
		t.Fatalf("failed to recall facts: %v", err)
	}

	if len(memories) != 2 {
		t.Errorf("expected 2 memories after upsert, got %d", len(memories))
	}

	for _, m := range memories {
		if m.Topic == "auth_bug" {
			if m.FactContent != "Updated content for JWT secret requirement." {
				t.Errorf("expected updated content, got '%s'", m.FactContent)
			}
			if m.Tags != "auth,env,updated" {
				t.Errorf("expected updated tags, got '%s'", m.Tags)
			}
		}
	}

	// 4. Test search querying
	// Query matching content
	memories, err = store.RecallProjectFacts(project1, "JWT")
	if err != nil {
		t.Fatalf("failed to query facts: %v", err)
	}
	if len(memories) != 1 || memories[0].Topic != "auth_bug" {
		t.Errorf("expected 1 result (auth_bug) for search 'JWT', got %d", len(memories))
	}

	// Query matching topic
	memories, err = store.RecallProjectFacts(project1, "todo")
	if err != nil {
		t.Fatalf("failed to query facts: %v", err)
	}
	if len(memories) != 1 || memories[0].Topic != "todo" {
		t.Errorf("expected 1 result (todo) for search 'todo', got %d", len(memories))
	}

	// Query matching tags
	memories, err = store.RecallProjectFacts(project1, "race")
	if err != nil {
		t.Fatalf("failed to query facts: %v", err)
	}
	if len(memories) != 1 || memories[0].Topic != "todo" {
		t.Errorf("expected 1 result (todo) for search 'race', got %d", len(memories))
	}

	// Query matching nothing
	memories, err = store.RecallProjectFacts(project1, "nonexistent")
	if err != nil {
		t.Fatalf("failed to query facts: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 results for search 'nonexistent', got %d", len(memories))
	}

	// 5. Test clear specific topic
	rowsCleared, err := store.ClearProjectContext(project1, "todo")
	if err != nil {
		t.Fatalf("failed to clear specific topic: %v", err)
	}
	if rowsCleared != 1 {
		t.Errorf("expected 1 row cleared, got %d", rowsCleared)
	}

	memories, err = store.RecallProjectFacts(project1, "")
	if err != nil {
		t.Fatalf("failed to recall facts: %v", err)
	}
	if len(memories) != 1 || memories[0].Topic != "auth_bug" {
		t.Errorf("expected only 'auth_bug' topic to remain, got %d memories", len(memories))
	}

	// 6. Test clear all for project
	err = store.RememberProjectFact(project1, "todo", "Another todo.", "")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	err = store.RememberProjectFact(project2, "another_proj_topic", "Fact for project 2.", "")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	rowsCleared, err = store.ClearProjectContext(project1, "")
	if err != nil {
		t.Fatalf("failed to clear project1 context: %v", err)
	}
	if rowsCleared != 2 {
		t.Errorf("expected 2 rows cleared for project1, got %d", rowsCleared)
	}

	memories, err = store.RecallProjectFacts(project1, "")
	if err != nil {
		t.Fatalf("failed to recall project1 facts: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories for project1, got %d", len(memories))
	}

	memories2, err := store.RecallProjectFacts(project2, "")
	if err != nil {
		t.Fatalf("failed to recall project2 facts: %v", err)
	}
	if len(memories2) != 1 {
		t.Errorf("expected project2 context to be untouched (1 memory), got %d", len(memories2))
	}
}

func TestUsageStats(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "contextsync-usage-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "usage_memory.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Close()

	// 1. Fresh store should report no usage.
	stats, err := store.GetUsageStats()
	if err != nil {
		t.Fatalf("failed to get usage stats: %v", err)
	}
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 total calls on fresh store, got %d", stats.TotalCalls)
	}
	if len(stats.PerTool) != 0 {
		t.Errorf("expected empty per-tool breakdown, got %d entries", len(stats.PerTool))
	}

	// 2. Record several tool invocations.
	project := "/path/to/project"
	invocations := []struct {
		tool string
		path string
	}{
		{"remember_project_fact", project},
		{"remember_project_fact", project},
		{"recall_project_facts", project},
		{"get_usage_stats", ""},
	}
	for _, inv := range invocations {
		if err := store.RecordToolUsage(inv.tool, inv.path); err != nil {
			t.Fatalf("failed to record tool usage: %v", err)
		}
	}

	// 3. Verify aggregated totals.
	stats, err = store.GetUsageStats()
	if err != nil {
		t.Fatalf("failed to get usage stats: %v", err)
	}
	if stats.TotalCalls != 4 {
		t.Errorf("expected 4 total calls, got %d", stats.TotalCalls)
	}
	if stats.CallsLast24h != 4 {
		t.Errorf("expected 4 calls in last 24h, got %d", stats.CallsLast24h)
	}
	if stats.CallsLast7d != 4 {
		t.Errorf("expected 4 calls in last 7d, got %d", stats.CallsLast7d)
	}
	if stats.FirstUsed.IsZero() || stats.LastUsed.IsZero() {
		t.Errorf("expected non-zero first/last used timestamps, got first=%v last=%v", stats.FirstUsed, stats.LastUsed)
	}

	// 4. Verify per-tool breakdown, ordered by call count descending.
	if len(stats.PerTool) != 3 {
		t.Fatalf("expected 3 distinct tools, got %d", len(stats.PerTool))
	}
	if stats.PerTool[0].ToolName != "remember_project_fact" || stats.PerTool[0].CallCount != 2 {
		t.Errorf("expected most-used tool 'remember_project_fact' with 2 calls, got '%s' with %d",
			stats.PerTool[0].ToolName, stats.PerTool[0].CallCount)
	}

	counts := map[string]int64{}
	for _, ts := range stats.PerTool {
		counts[ts.ToolName] = ts.CallCount
		if ts.FirstUsed.IsZero() || ts.LastUsed.IsZero() {
			t.Errorf("expected non-zero timestamps for tool '%s'", ts.ToolName)
		}
	}
	if counts["recall_project_facts"] != 1 {
		t.Errorf("expected 1 call for 'recall_project_facts', got %d", counts["recall_project_facts"])
	}
	if counts["get_usage_stats"] != 1 {
		t.Errorf("expected 1 call for 'get_usage_stats', got %d", counts["get_usage_stats"])
	}
}
