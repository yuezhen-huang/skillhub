package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/yuezhen-huang/skillhub/api/gen/skillhub"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serverAddr string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "skillctl",
		Short: "Skill Hub CLI",
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:50051", "Server address")

	rootCmd.AddCommand(addCmd())
	rootCmd.AddCommand(removeCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(getCmd())
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(restartCmd())
	rootCmd.AddCommand(switchCmd())
	rootCmd.AddCommand(versionsCmd())
	rootCmd.AddCommand(pullCmd())
	rootCmd.AddCommand(scanCmd())
	rootCmd.AddCommand(alignCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getClient() (skillhub.SkillHubClient, func() error, error) {
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return skillhub.NewSkillHubClient(conn), conn.Close, nil
}

func addCmd() *cobra.Command {
	var (
		gitlabURL  string
		versionRef string
		configs    []string
	)

	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "Add a skill from GitLab",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			configMap := make(map[string]string)
			for _, c := range configs {
				key, val, ok := strings.Cut(c, "=")
				if !ok {
					return fmt.Errorf("invalid --config %q, expected key=value", c)
				}
				key = strings.TrimSpace(key)
				if key == "" {
					return fmt.Errorf("invalid --config %q, empty key", c)
				}
				configMap[key] = val
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			resp, err := client.AddSkill(ctx, &skillhub.AddSkillRequest{
				Name:       name,
				GitlabUrl:  gitlabURL,
				VersionRef: versionRef,
				Config:     configMap,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Added skill: %s (%s)\n", resp.Skill.Name, resp.Skill.Id)
			return nil
		},
	}

	cmd.Flags().StringVarP(&gitlabURL, "gitlab", "g", "", "GitLab repository URL (required)")
	cmd.Flags().StringVarP(&versionRef, "ref", "r", "", "Version reference (branch, tag, or commit)")
	cmd.Flags().StringSliceVarP(&configs, "config", "c", nil, "Config key=value pairs (can specify multiple)")
	cmd.MarkFlagRequired("gitlab")

	return cmd
}

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove ID",
		Short: "Remove a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err = client.RemoveSkill(ctx, &skillhub.RemoveSkillRequest{Id: id})
			if err != nil {
				return err
			}

			fmt.Printf("Removed skill: %s\n", id)
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.ListSkills(ctx, &skillhub.ListSkillsRequest{})
			if err != nil {
				return err
			}

			if len(resp.Skills) == 0 {
				fmt.Println("No skills found")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tVERSION\tSTATUS\tBRANCH/TAG")
			fmt.Fprintln(w, "──\t────\t──────\t──────\t──────────")
			for _, s := range resp.Skills {
				ref := ""
				if s.Repository != nil {
					if s.Repository.Tag != "" {
						ref = s.Repository.Tag
					} else if s.Repository.Branch != "" {
						ref = s.Repository.Branch
					} else if len(s.Repository.Commit) >= 8 {
						ref = s.Repository.Commit[:8]
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Id[:8], s.Name, s.Version, s.Status, ref)
			}
			w.Flush()

			return nil
		},
	}
}

func getCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get ID",
		Short: "Get skill details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.GetSkill(ctx, &skillhub.GetSkillRequest{Id: id})
			if err != nil {
				return err
			}

			s := resp.Skill
			fmt.Printf("Name:        %s\n", s.Name)
			fmt.Printf("ID:          %s\n", s.Id)
			fmt.Printf("Version:     %s\n", s.Version)
			fmt.Printf("Status:      %s\n", s.Status)
			if s.Repository != nil {
				fmt.Printf("Repo URL:    %s\n", s.Repository.Url)
				if s.Repository.Branch != "" {
					fmt.Printf("Branch:      %s\n", s.Repository.Branch)
				}
				if s.Repository.Tag != "" {
					fmt.Printf("Tag:         %s\n", s.Repository.Tag)
				}
				if s.Repository.Commit != "" {
					fmt.Printf("Commit:      %s\n", s.Repository.Commit)
				}
			}
			if s.Process != nil {
				fmt.Printf("PID:         %d\n", s.Process.Pid)
				fmt.Printf("RPC Addr:    %s\n", s.Process.RpcAddress)
			}

			return nil
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start ID",
		Short: "Start a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			_, err = client.StartSkill(ctx, &skillhub.StartSkillRequest{Id: id})
			if err != nil {
				return err
			}

			fmt.Printf("Started skill: %s\n", id)
			return nil
		},
	}
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop ID",
		Short: "Stop a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err = client.StopSkill(ctx, &skillhub.StopSkillRequest{Id: id})
			if err != nil {
				return err
			}

			fmt.Printf("Stopped skill: %s\n", id)
			return nil
		},
	}
}

func restartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart ID",
		Short: "Restart a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			_, err = client.RestartSkill(ctx, &skillhub.RestartSkillRequest{Id: id})
			if err != nil {
				return err
			}

			fmt.Printf("Restarted skill: %s\n", id)
			return nil
		},
	}
}

func switchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch ID REF",
		Short: "Switch to a different version (branch, tag, or commit)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			ref := args[1]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			resp, err := client.SwitchVersion(ctx, &skillhub.SwitchVersionRequest{
				Id:         id,
				VersionRef: ref,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Switched to version: %s\n", resp.Version)
			return nil
		},
	}
}

func versionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "versions ID",
		Short: "List available versions (branches and tags)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.ListVersions(ctx, &skillhub.ListVersionsRequest{Id: id})
			if err != nil {
				return err
			}

			if len(resp.Branches) > 0 {
				fmt.Println("Branches:")
				for _, b := range resp.Branches {
					fmt.Printf("  %s\n", b)
				}
			}
			if len(resp.Tags) > 0 {
				fmt.Println("Tags:")
				for _, t := range resp.Tags {
					fmt.Printf("  %s\n", t)
				}
			}
			if len(resp.Branches) == 0 && len(resp.Tags) == 0 {
				fmt.Println("No branches or tags found")
			}

			return nil
		},
	}
}

func pullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull ID",
		Short: "Pull latest changes for the skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			resp, err := client.PullLatest(ctx, &skillhub.PullLatestRequest{Id: id})
			if err != nil {
				return err
			}

			if len(resp.Commit) >= 8 {
				fmt.Printf("Pulled latest, now at commit: %s\n", resp.Commit[:8])
			} else {
				fmt.Println("Pulled latest")
			}

			return nil
		},
	}
}

func scanCmd() *cobra.Command {
	var importAll bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan for existing skills in the skills directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			resp, err := client.ScanSkills(ctx, &skillhub.ScanSkillsRequest{
				ImportAll: importAll,
			})
			if err != nil {
				return err
			}

			if len(resp.Discovered) == 0 {
				fmt.Println("No skills found in skills directory")
				return nil
			}

			fmt.Printf("Found %d skills:\n", len(resp.Discovered))
			fmt.Println()

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tVERSION\tPATH\tREASON")
			fmt.Fprintln(w, "────\t──────\t──────\t────\t──────")

			for _, d := range resp.Discovered {
				status := "importable"
				reason := ""
				if d.AlreadyImported {
					status = "imported"
				} else if !d.IsValidSkill {
					status = "invalid"
					reason = d.ValidationError
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", d.Name, status, d.DetectedVersion, d.Path, reason)
			}
			w.Flush()

			fmt.Println()
			if importAll {
				fmt.Printf("Imported: %d, Skipped: %d\n", resp.ImportedCount, resp.SkippedCount)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&importAll, "import", "i", false, "Automatically import all unimported skills")
	return cmd
}

func alignCmd() *cobra.Command {
	var autoFix bool

	cmd := &cobra.Command{
		Use:   "align",
		Short: "Check agent health and alignment, optionally fix issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, close, err := getClient()
			if err != nil {
				return err
			}
			defer close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			resp, err := client.AlignAgents(ctx, &skillhub.AlignAgentsRequest{
				AutoFix: autoFix,
			})
			if err != nil {
				return err
			}

			if resp.AllHealthy && len(resp.Issues) == 0 {
				fmt.Println("All agents are healthy and aligned!")
				return nil
			}

			fmt.Printf("Found %d issues:\n", len(resp.Issues))
			fmt.Println()

			if len(resp.Issues) > 0 {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "SKILL\tSEVERITY\tTYPE\tDESCRIPTION\tSTATUS")
				fmt.Fprintln(w, "─────\t────────\t────\t───────────\t──────")

				for _, issue := range resp.Issues {
					status := "needs fix"
					if issue.Fixed {
						status = "fixed"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						issue.SkillName,
						issue.Severity,
						issue.IssueType,
						issue.Description,
						status,
					)
				}
				w.Flush()

				fmt.Println()
			}

			// Print execution report (directories detected + actions taken/skipped).
			if resp.Report != nil {
				if len(resp.Report.AgentDirs) > 0 {
					fmt.Println("Detected agent dirs:")
					for _, d := range resp.Report.AgentDirs {
						fmt.Printf("  - %s\n", d)
					}
					fmt.Println()
				}

				if len(resp.Report.Actions) > 0 {
					fmt.Println("Execution report:")
					w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(w, "AGENT_DIR\tSKILL\tACTION\tSUCCESS\tREASON")
					fmt.Fprintln(w, "─────────\t─────\t──────\t───────\t──────")
					for _, a := range resp.Report.Actions {
						reason := a.Reason
						fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", a.AgentDir, a.SkillName, a.Action, a.Success, reason)
					}
					w.Flush()
					fmt.Println()
				}
			}

			if autoFix && resp.FixedCount > 0 {
				fmt.Printf("Fixed %d issues\n", resp.FixedCount)
			} else if autoFix {
				fmt.Println("No issues could be automatically fixed")
			} else {
				fmt.Println("Run with --fix to attempt automatic fixes")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&autoFix, "fix", "f", false, "Automatically fix issues where possible")
	return cmd
}
