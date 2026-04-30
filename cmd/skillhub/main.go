package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yuezhen-huang/skillhub/internal/gitlab"
	"github.com/yuezhen-huang/skillhub/internal/hub"
	"github.com/yuezhen-huang/skillhub/internal/skill"
	"github.com/yuezhen-huang/skillhub/internal/storage"
	"github.com/yuezhen-huang/skillhub/pkg/config"

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
	fmt.Println("Loading configuration...")
	cfg, err := config.Load(configPath)
	if err != nil {
		cfg = config.Default()
		fmt.Println("Using default configuration")
	}

	fmt.Printf("Opening storage at %s...\n", cfg.Storage.Path)
	store, err := storage.NewSQLiteStore(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	fmt.Println("Initializing managers...")
	repoMgr := gitlab.NewRepositoryManager()
	runtime := skill.NewRuntime(&cfg.Skill)
	manager := hub.NewManager(store, repoMgr, runtime, cfg)
	rpcServer := hub.NewRPCServer(manager, cfg.Hub.GRPCAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Printf("Starting gRPC server on %s...\n", cfg.Hub.GRPCAddr)
	if err := rpcServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer rpcServer.Stop()

	fmt.Printf("Skill Hub started successfully on %s\n", cfg.Hub.GRPCAddr)

	fmt.Println("Scanning for existing skills...")
	scanResult, err := manager.ScanSkills(ctx, true)
	if err != nil {
		fmt.Printf("Warning: Scan failed: %v\n", err)
	} else if len(scanResult.Discovered) > 0 {
		fmt.Printf("Scan complete: %d imported, %d skipped\n", scanResult.ImportedCount, scanResult.SkippedCount)
	} else {
		fmt.Println("No existing skills found")
	}

	fmt.Println("Checking agent alignment...")
	alignResult, err := manager.AlignAgents(ctx, true)
	if err != nil {
		fmt.Printf("Warning: Alignment check failed: %v\n", err)
	} else if alignResult.AllHealthy {
		fmt.Println("All agents are healthy and aligned")
	} else {
		fmt.Printf("Alignment complete: %d issues found, %d fixed\n", len(alignResult.Issues), alignResult.FixedCount)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	runtime.Cleanup()

	return nil
}
