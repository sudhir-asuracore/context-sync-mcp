package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"contextsync/db"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed VERSION
var version string

// Define request argument structs for BindArguments
type RememberFactArgs struct {
	ProjectPath string `json:"project_path"`
	Topic       string `json:"topic"`
	FactContent string `json:"fact_content"`
	Tags        string `json:"tags,omitempty"`
}

type RecallFactsArgs struct {
	ProjectPath string `json:"project_path"`
	SearchQuery string `json:"search_query,omitempty"`
}

type ClearContextArgs struct {
	ProjectPath string `json:"project_path"`
	Topic       string `json:"topic,omitempty"`
}

func main() {
	// Set log output to Stderr to prevent corruption of stdout which is used for JSON-RPC
	log.SetOutput(os.Stderr)
	log.SetPrefix("[ContextSync] ")

	// Define command line flags for custom database path
	dbPathFlag := flag.String("db", "", "Path to SQLite database file (defaults to ~/.config/contextsync/memory.db)")
	flag.Parse()

	// Initialize the Database Store
	store, err := db.NewStore(*dbPathFlag)
	if err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}
	defer store.Close()

	log.Printf("Starting ContextSync MCP Server v%s...", strings.TrimSpace(version))

	// 1. Create a new MCP server
	s := server.NewMCPServer("ContextSync", strings.TrimSpace(version))

	// 2. Define tools
	rememberTool := mcp.NewTool("remember_project_fact",
		mcp.WithDescription("Persists an engineering fact, configuration quirk, structural target, or decision bound to the current codebase directory."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the current workspace directory.")),
		mcp.WithString("topic", mcp.Required(), mcp.Description("The classification header (e.g., 'current_roadblock', 'database_setup', 'build_commands').")),
		mcp.WithString("fact_content", mcp.Required(), mcp.Description("Concise, dense natural language detailing the knowledge to preserve.")),
		mcp.WithString("tags", mcp.Description("Comma-separated query tokens (optional).")),
	)

	recallTool := mcp.NewTool("recall_project_facts",
		mcp.WithDescription("Returns all stored knowledge, historical decisions, and state values for the given workspace path."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the workspace directory.")),
		mcp.WithString("search_query", mcp.Description("Keyword string to filter down responses using basic SQL LIKE constraints (optional).")),
	)

	clearTool := mcp.NewTool("clear_project_context",
		mcp.WithDescription("Purges all entries or targeted topics for a project when a task lifecycle is completely closed or explicitly reset by the user."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the workspace directory.")),
		mcp.WithString("topic", mcp.Description("If provided, deletes only that matching slice instead of wiping the whole project footprint (optional).")),
	)

	statsTool := mcp.NewTool("get_usage_stats",
		mcp.WithDescription("Reports ContextSync tool usage statistics (total calls, recent activity, and a per-tool breakdown) to gauge how actively and effectively the memory bank is being used."),
	)

	// 3. Register Tool Handlers
	s.AddTool(rememberTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args RememberFactArgs
		if err := req.BindArguments(&args); err != nil {
			return nil, fmt.Errorf("failed to bind arguments: %w", err)
		}

		if args.ProjectPath == "" {
			return nil, fmt.Errorf("project_path is required")
		}
		if args.Topic == "" {
			return nil, fmt.Errorf("topic is required")
		}
		if args.FactContent == "" {
			return nil, fmt.Errorf("fact_content is required")
		}

		if err := store.RecordToolUsage("remember_project_fact", args.ProjectPath); err != nil {
			log.Printf("Warning: failed to record tool usage: %v", err)
		}

		err := store.RememberProjectFact(args.ProjectPath, args.Topic, args.FactContent, args.Tags)
		if err != nil {
			log.Printf("Error remembering fact: %v", err)
			return nil, err
		}

		log.Printf("Successfully remembered fact for project %s under topic %s", args.ProjectPath, args.Topic)
		return mcp.NewToolResultText(fmt.Sprintf("Successfully remembered fact for project '%s' and topic '%s'.", args.ProjectPath, args.Topic)), nil
	})

	s.AddTool(recallTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args RecallFactsArgs
		if err := req.BindArguments(&args); err != nil {
			return nil, fmt.Errorf("failed to bind arguments: %w", err)
		}

		if args.ProjectPath == "" {
			return nil, fmt.Errorf("project_path is required")
		}

		if err := store.RecordToolUsage("recall_project_facts", args.ProjectPath); err != nil {
			log.Printf("Warning: failed to record tool usage: %v", err)
		}

		memories, err := store.RecallProjectFacts(args.ProjectPath, args.SearchQuery)
		if err != nil {
			log.Printf("Error recalling facts: %v", err)
			return nil, err
		}

		if len(memories) == 0 {
			msg := fmt.Sprintf("No memories found for project path '%s'", args.ProjectPath)
			if args.SearchQuery != "" {
				msg += fmt.Sprintf(" with search query '%s'", args.SearchQuery)
			}
			return mcp.NewToolResultText(msg + "."), nil
		}

		// Format output in clean Markdown for the LLM
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# ContextSync Memory Bank: %s\n\n", args.ProjectPath))
		if args.SearchQuery != "" {
			sb.WriteString(fmt.Sprintf("Filtering by keyword: `%s`\n\n", args.SearchQuery))
		}

		for _, m := range memories {
			sb.WriteString(fmt.Sprintf("## Topic: %s\n", m.Topic))
			sb.WriteString(fmt.Sprintf("- **Content:** %s\n", m.FactContent))
			if m.Tags != "" {
				sb.WriteString(fmt.Sprintf("- **Tags:** %s\n", m.Tags))
			}
			sb.WriteString(fmt.Sprintf("- **Last Updated:** %s\n\n", m.UpdatedAt.Format("2006-01-02 15:04:05 MST")))
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	s.AddTool(clearTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args ClearContextArgs
		if err := req.BindArguments(&args); err != nil {
			return nil, fmt.Errorf("failed to bind arguments: %w", err)
		}

		if args.ProjectPath == "" {
			return nil, fmt.Errorf("project_path is required")
		}

		if err := store.RecordToolUsage("clear_project_context", args.ProjectPath); err != nil {
			log.Printf("Warning: failed to record tool usage: %v", err)
		}

		count, err := store.ClearProjectContext(args.ProjectPath, args.Topic)
		if err != nil {
			log.Printf("Error clearing context: %v", err)
			return nil, err
		}

		var msg string
		if args.Topic != "" {
			msg = fmt.Sprintf("Successfully cleared topic '%s' for project '%s' (%d records deleted).", args.Topic, args.ProjectPath, count)
		} else {
			msg = fmt.Sprintf("Successfully cleared all context for project '%s' (%d records deleted).", args.ProjectPath, count)
		}
		log.Print(msg)
		return mcp.NewToolResultText(msg), nil
	})

	s.AddTool(statsTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := store.RecordToolUsage("get_usage_stats", ""); err != nil {
			log.Printf("Warning: failed to record tool usage: %v", err)
		}

		stats, err := store.GetUsageStats()
		if err != nil {
			log.Printf("Error retrieving usage stats: %v", err)
			return nil, err
		}

		if stats.TotalCalls == 0 {
			return mcp.NewToolResultText("No usage recorded yet. ContextSync tools have not been called."), nil
		}

		const tsLayout = "2006-01-02 15:04:05 MST"
		var sb strings.Builder
		sb.WriteString("# ContextSync Usage Statistics\n\n")
		sb.WriteString(fmt.Sprintf("- **Total Calls:** %d\n", stats.TotalCalls))
		sb.WriteString(fmt.Sprintf("- **Calls (last 24h):** %d\n", stats.CallsLast24h))
		sb.WriteString(fmt.Sprintf("- **Calls (last 7d):** %d\n", stats.CallsLast7d))
		if !stats.FirstUsed.IsZero() {
			sb.WriteString(fmt.Sprintf("- **First Used:** %s\n", stats.FirstUsed.Format(tsLayout)))
		}
		if !stats.LastUsed.IsZero() {
			sb.WriteString(fmt.Sprintf("- **Last Used:** %s\n", stats.LastUsed.Format(tsLayout)))
		}

		sb.WriteString("\n## Per-Tool Breakdown\n\n")
		sb.WriteString("| Tool | Calls | First Used | Last Used |\n")
		sb.WriteString("| --- | --- | --- | --- |\n")
		for _, t := range stats.PerTool {
			first := "-"
			if !t.FirstUsed.IsZero() {
				first = t.FirstUsed.Format(tsLayout)
			}
			last := "-"
			if !t.LastUsed.IsZero() {
				last = t.LastUsed.Format(tsLayout)
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s | %s |\n", t.ToolName, t.CallCount, first, last))
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	// 4. Start the server using stdio transport
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server shutdown with error: %v", err)
	}
}
