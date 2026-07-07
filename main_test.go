package main

import (
	"os"
	"path/filepath"
	"testing"

	"contextsync/db"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestToolRegistration(t *testing.T) {
	// This test just ensures we can define the tools without panic
	rememberTool := mcp.NewTool("remember_project_fact",
		mcp.WithDescription("Persists an engineering fact."),
		mcp.WithString("project_path", mcp.Required()),
		mcp.WithString("topic", mcp.Required()),
		mcp.WithString("fact_content", mcp.Required()),
	)

	if rememberTool.Name != "remember_project_fact" {
		t.Errorf("expected tool name remember_project_fact, got %s", rememberTool.Name)
	}
}

func TestEndToEndStorage(t *testing.T) {
	// Create a temp directory for the test database
	tempDir, err := os.MkdirTemp("", "contextsync-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "e2e.db")
	store, err := db.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Close()

	projectPath := "/path/to/project"
	topic := "test_topic"
	content := "test content"

	// Test Remember
	err = store.RememberProjectFact(projectPath, topic, content, "tag1")
	if err != nil {
		t.Errorf("failed to remember fact: %v", err)
	}

	// Test Recall
	memories, err := store.RecallProjectFacts(projectPath, "")
	if err != nil {
		t.Fatalf("failed to recall facts: %v", err)
	}

	if len(memories) != 1 {
		t.Errorf("expected 1 memory, got %d", len(memories))
	} else {
		if memories[0].Topic != topic {
			t.Errorf("expected topic %s, got %s", topic, memories[0].Topic)
		}
		if memories[0].FactContent != content {
			t.Errorf("expected content %s, got %s", content, memories[0].FactContent)
		}
	}

	// Test Stats
	stats, err := store.GetUsageStats()
	if err != nil {
		t.Fatalf("failed to get usage stats: %v", err)
	}
	// We haven't recorded usage yet in this test
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 total calls, got %d", stats.TotalCalls)
	}

	err = store.RecordToolUsage("remember_project_fact", projectPath)
	if err != nil {
		t.Fatalf("failed to record usage: %v", err)
	}

	stats, err = store.GetUsageStats()
	if err != nil {
		t.Fatalf("failed to get usage stats: %v", err)
	}
	if stats.TotalCalls != 1 {
		t.Errorf("expected 1 total call, got %d", stats.TotalCalls)
	}
}
