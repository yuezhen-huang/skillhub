package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/skillhub/skill-hub/internal/gitlab"
	"github.com/skillhub/skill-hub/internal/hub"
	"github.com/skillhub/skill-hub/internal/storage"
	"github.com/skillhub/skill-hub/internal/skill"
	"github.com/skillhub/skill-hub/pkg/config"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "skillhub",
		Short: "Skill Hub - Multi AI-agent skill management",
	}

	var configPath string

	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the skill hub daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(configPath)
		},
	}

	daemonCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	rootCmd.AddCommand(daemonCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runDaemon(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		cfg = config.Default()
	}

	store, err := storage.NewSQLiteStore(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	repoMgr := gitlab.NewRepositoryManager()
	runtime := skill.NewRuntime(&cfg.Skill)
	manager := hub.NewManager(store, repoMgr, runtime, cfg)
	rpcServer := hub.NewRPCServer(manager, cfg.Hub.GRPCAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rpcServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer rpcServer.Stop()

	fmt.Printf("Skill Hub started on %s\n", cfg.Hub.GRPCAddr)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	runtime.Cleanup()

	return nil
}
