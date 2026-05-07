package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tethy/mulwiki/server/pkg/gitrepo"

	"github.com/tethy/mulwiki/server/internal/events"
	"github.com/tethy/mulwiki/server/internal/handler"
	"github.com/tethy/mulwiki/server/internal/logbuf"
	"github.com/tethy/mulwiki/server/internal/middleware"
	"github.com/tethy/mulwiki/server/internal/realtime"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// --- Load environment ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "file:mulwiki.db"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Warn("JWT_SECRET is not set — using insecure default. Set JWT_SECRET for production use.")
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// --- Database ---
	if !strings.Contains(dbURL, "?") {
		dbURL += "?"
	} else {
		dbURL += "&"
	}
	dbURL += "_journal_mode=WAL&_foreign_keys=on"
	db, err := sql.Open("sqlite3", dbURL)
	if err != nil {
		slog.Error("unable to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		slog.Error("unable to ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database", "url", dbURL)

	// --- Run migrations ---
	if err := runMigrations(db); err != nil {
		slog.Error("unable to run migrations", "error", err)
		os.Exit(1)
	}

	// --- Event bus & WebSocket hub ---
	bus := events.NewBus()
	hub := realtime.NewHub(bus)

	// --- Log buffer (1000 lines per job) ---
	logStore := logbuf.NewStore(1000)

	// --- Create handler ---
	h := handler.NewWithDeps(db, bus, hub)
	h.ReposDir = filepath.Join(dataDir, "repos")
	h.BuiltinSchemasDir = filepath.Join(dataDir, "builtin-schemas")
	h.LogBuf = logStore

	// --- Seed builtin schemas into existing workspaces ---
	if err := seedBuiltinSchemas(db, h); err != nil {
		slog.Error("unable to seed builtin schemas", "error", err)
		os.Exit(1)
	}

	// --- Router ---
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// Workspace middleware — resolve workspace from URL slug
	r.Use(middleware.Workspace(db))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-User-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// --- Stale agent detection ---
	go runStaleAgentDetector(db, 5*time.Minute, 30*time.Second)

	// --- Public routes ---
	r.Group(func(r chi.Router) {
		// Git repos — expose for daemon cloning (dumb HTTP protocol).
		r.Handle("/repos/*", http.StripPrefix("/repos/", http.FileServer(http.Dir(filepath.Join(dataDir, "repos")))))
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if err := db.Ping(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"status":"degraded","db":"unreachable"}`))
				return
			}
			w.Write([]byte(`{"status":"ok","db":"connected"}`))
		})

		r.Route("/api/auth", func(r chi.Router) {
			r.Post("/register", h.Register)
			r.Post("/login", h.Login)
			r.Post("/logout", h.Logout)
			r.Get("/me", h.Me)
		})

		r.Get("/api/schemas/builtin", h.ListBuiltinSchemas)

		// Workspaces (public for now; auth can be added later)
		r.Route("/api/workspaces", func(r chi.Router) {
			r.Get("/", h.ListWorkspaces)
			r.Post("/", h.CreateWorkspace)
			r.Route("/{slug}", func(r chi.Router) {
				r.Get("/", h.GetWorkspace)
				r.Patch("/", h.UpdateWorkspace)
				r.Delete("/", h.DeleteWorkspace)

				// Schemas
				r.Route("/schemas", func(r chi.Router) {
					r.Get("/", h.ListSchemas)
					r.Post("/", h.CreateSchema)
					r.Post("/fork", h.ForkSchema)
					r.Post("/validate", h.ValidateSchema)
					r.Get("/{id}", h.GetSchema)
					r.Put("/{id}", h.UpdateSchema)
					r.Delete("/{id}", h.DeleteSchema)
				})

				// Workspace-level actions
				r.Put("/activate-schema", h.ActivateSchema)

				// Sources (git-backed — wildcard path)
				r.Route("/sources", func(r chi.Router) {
					r.Get("/", h.ListSources)
					r.Post("/", h.CreateSource)
					r.Get("/*", h.GetSource)
					r.Delete("/*", h.DeleteSource)
				})

				// Wikilink resolution & backlinks (explicit paths, avoid /* wildcard)
				r.Post("/wiki/resolve-links", h.ResolveWikiLinks)
				r.Get("/wiki/backlinks", h.GetWikiBacklinks) // ?path=...

				// Wiki (git-backed — markdown with frontmatter)
				r.Route("/wiki", func(r chi.Router) {
					r.Get("/", h.ListWikiPages)
					r.Get("/search", h.SearchWikiPages)
					r.Post("/", h.CreateWikiPage)
					r.Get("/*", h.GetWikiPage)
					r.Delete("/*", h.DeleteWikiPage)
				})

				// Jobs (user-facing)
				r.Route("/jobs", func(r chi.Router) {
					r.Get("/", h.ListJobs)
					r.Post("/", h.CreateJob)
					r.Get("/{id}", h.GetJob)
					r.Get("/{id}/logs", h.StreamJobLogs)

					// Daemon-facing job endpoints
					r.Post("/claim", h.ClaimJob)
					r.Post("/{id}/log-line", h.AppendJobLog)
					r.Post("/{id}/progress", h.UpdateJobProgress)
					r.Post("/{id}/complete", h.CompleteJob)
					r.Post("/{id}/fail", h.FailJob)
					r.Post("/{id}/output", h.SubmitJobOutput)
				})

				// Agents
				r.Route("/agents", func(r chi.Router) {
					// Runtimes — must register before /{id} catch-all
					r.Route("/runtimes", func(r chi.Router) {
						r.Get("/", h.ListRuntimes)
						r.Post("/", h.CreateRuntime)
						r.Get("/{id}", h.GetRuntime)
						r.Patch("/{id}", h.UpdateRuntime)
						r.Delete("/{id}", h.DeleteRuntime)
					})

					// Skills — must register before /{id} catch-all
					r.Route("/skills", func(r chi.Router) {
						r.Get("/", h.ListSkills)
						r.Post("/", h.CreateSkill)
						r.Patch("/{id}", h.UpdateSkill)
						r.Delete("/{id}", h.DeleteSkill)
					})

					// Agent list + create
					r.Get("/", h.ListAgents)
					r.Post("/", h.CreateAgent)

					// Agent by ID — must come after /runtimes and /skills
					r.Route("/{id}", func(r chi.Router) {
						r.Get("/", h.GetAgent)
						r.Patch("/", h.UpdateAgent)
						r.Post("/archive", h.ArchiveAgent)
						r.Post("/restore", h.RestoreAgent)
						r.Post("/heartbeat", h.AgentHeartbeat)

						// Agent-skill associations
						r.Route("/skills", func(r chi.Router) {
							r.Post("/", h.AddAgentSkill)
							r.Delete("/{skillId}", h.RemoveAgentSkill)
						})

						// Agent tasks
						r.Route("/tasks", func(r chi.Router) {
							r.Get("/", h.ListAgentTasks)
							r.Post("/", h.CreateAgentTask)
							r.Post("/claim", h.ClaimAgentTask)
							r.Get("/{taskId}", h.GetAgentTask)
							r.Patch("/{taskId}", h.UpdateAgentTask)
						})
					})
				})
			})
		})
	})

	// --- Protected routes (with JWT auth) ---
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		// Protected workspace routes use the same handlers;
		// the middleware sets X-User-ID header.
	})

	// --- Daemon routes ---
	r.Route("/api/daemon", func(r chi.Router) {
		r.Get("/", h.ListDaemons)
		r.Post("/register", h.DaemonRegister)
		r.Post("/heartbeat", h.DaemonHeartbeat)
		r.Get("/stale", h.DaemonStale)
		r.Get("/{id}/logs", h.GetDaemonLogs)
		r.Post("/{id}/stop", h.StopDaemon)
		r.Post("/start", h.StartDaemon)
	})

	// --- WebSocket ---
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r)
	})

	// --- Server ---
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// runMigrations reads the schema SQL and executes it against the database.
func runMigrations(db *sql.DB) error {
	schemaPath := "pkg/db/schema.sql"
	// Fallback: try relative to the binary's working directory.
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		// Try from the server/ directory.
		schemaSQL, err = os.ReadFile("../../pkg/db/schema.sql")
		if err != nil {
			return fmt.Errorf("read schema.sql: %w", err)
		}
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	if err := ensureColumn(db, "schemas", "source_type", "TEXT NOT NULL DEFAULT 'user'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "workspaces", "description", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "jobs", "agent_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "jobs", "source_ids", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_schemas_source_type ON schemas(source_type)`); err != nil {
		return fmt.Errorf("create source_type index: %w", err)
	}
	// Migration: v2 → v3 — Runtime model upgrade
	// Rename provider → backend (only if provider still exists)
	{
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agent_runtimes') WHERE name = 'provider'`).Scan(&count); err == nil && count > 0 {
			if _, err := db.Exec(`ALTER TABLE agent_runtimes RENAME COLUMN provider TO backend`); err != nil {
				slog.Warn("migration: rename provider→backend failed (may be safe)", "error", err)
			}
		}
	}
	// Drop model column (only if it still exists)
	{
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agent_runtimes') WHERE name = 'model'`).Scan(&count); err == nil && count > 0 {
			if _, err := db.Exec(`ALTER TABLE agent_runtimes DROP COLUMN model`); err != nil {
				slog.Warn("migration: drop model column failed (may be safe)", "error", err)
			}
		}
	}
	if err := ensureColumn(db, "agent_runtimes", "hostname", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "agent_runtimes", "os", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "agent_runtimes", "version", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	slog.Info("migrations applied successfully")

	// Migration 004: Move schema config from DB to git.
	if err := migrateSchemasToGit(db); err != nil {
		slog.Warn("schema-to-git migration skipped (non-fatal)", "error", err)
	}

	return nil
}

// migrateSchemasToGit moves schema config from DB into per-workspace git repos.
// Each schema's config becomes schemas/{id}.md in the workspace's bare repo.
func migrateSchemasToGit(db *sql.DB) error {
	// Check if migration already ran (path column exists with data)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('schemas') WHERE name = 'path'").Scan(&count); err != nil || count == 0 {
		if _, err := db.Exec("ALTER TABLE schemas ADD COLUMN path TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add schemas.path: %w", err)
		}
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('workspaces') WHERE name = 'active_schema_path'").Scan(&count); err != nil || count == 0 {
		if _, err := db.Exec("ALTER TABLE workspaces ADD COLUMN active_schema_path TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add workspaces.active_schema_path: %w", err)
		}
	}

	// Check if already migrated
	var migrated int
	if err := db.QueryRow("SELECT COUNT(*) FROM schemas WHERE path != '' AND config IS NOT NULL AND config != ''").Scan(&migrated); err == nil && migrated > 0 {
		slog.Info("schema-to-git migration: already migrated", "count", migrated)
		return nil
	}

	// For each schema with config content, write to git
	rows, err := db.Query(`SELECT s.id, s.workspace_id, s.name, s.config, w.slug FROM schemas s JOIN workspaces w ON s.workspace_id = w.id WHERE s.config != '' AND s.config != '{}'`)
	if err != nil {
		return fmt.Errorf("query schemas: %w", err)
	}
	defer rows.Close()

	reposDir := findReposDir()

	var migratedCount int
	for rows.Next() {
		var id, wsID, name, config, slug string
		if err := rows.Scan(&id, &wsID, &name, &config, &slug); err != nil {
			slog.Warn("schema-to-git: scan row", "error", err)
			continue
		}

		gitPath := fmt.Sprintf("schemas/%s.md", id)
		repoPath := filepath.Join(reposDir, wsID+".git")

		// Write to git repo
		repo, err := gitrepo.Open(repoPath)
		if err != nil {
			slog.Warn("schema-to-git: open repo", "ws_id", wsID, "error", err)
			continue
		}
		if _, err := repo.WriteFile(gitPath, []byte(config), fmt.Sprintf("schema: %s", name)); err != nil {
			slog.Warn("schema-to-git: write file", "path", gitPath, "error", err)
			continue
		}

		// Update DB path
		if _, err := db.Exec("UPDATE schemas SET path = ? WHERE id = ?", gitPath, id); err != nil {
			slog.Warn("schema-to-git: update path", "id", id, "error", err)
			continue
		}

		// Update workspace active_schema_path
		if _, err := db.Exec("UPDATE workspaces SET active_schema_path = ? WHERE active_schema_id = ?", gitPath, id); err != nil {
			slog.Warn("schema-to-git: update active_schema_path", "id", id, "error", err)
		}

		migratedCount++
		slog.Info("schema-to-git: migrated", "name", name, "path", gitPath)
	}

	slog.Info("schema-to-git migration complete", "migrated", migratedCount)
	return nil
}

func findReposDir() string {
	paths := []string{"data/repos", "../../data/repos"}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return "data/repos"
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count); err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if count > 0 {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func seedBuiltinSchemas(db *sql.DB, handler *handler.Handler) error {
	// Seed builtin schemas into ALL existing workspaces (idempotent).
	rows, err := db.Query(`SELECT id, slug FROM workspaces WHERE slug <> 'builtin'`)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	var wsIDs []string
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			continue
		}
		wsIDs = append(wsIDs, id)
	}

	for _, wsID := range wsIDs {
		if err := handler.SeedBuiltinSchemas(wsID); err != nil {
			slog.Warn("seed builtin schemas for workspace", "ws_id", wsID, "error", err)
		}
	}

	return nil
}

// runStaleAgentDetector periodically marks agents as offline if they haven't
// sent a heartbeat within the stale threshold.
func runStaleAgentDetector(db *sql.DB, staleAfter, checkInterval time.Duration) {
	slog.Info("stale agent detector started",
		"stale_after", staleAfter,
		"check_interval", checkInterval,
	)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().UTC().Add(-staleAfter).Format(time.RFC3339)

		result, err := db.Exec(
			`UPDATE agents SET status = 'offline'
			 WHERE status = 'online'
			   AND updated_at < ?`,
			cutoff,
		)
		if err != nil {
			slog.Error("stale agent detection failed", "error", err)
			continue
		}

		affected, _ := result.RowsAffected()
		if affected > 0 {
			slog.Info("marked stale agents as offline", "count", affected)
		}
	}
}
