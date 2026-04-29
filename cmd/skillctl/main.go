package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/skillhub/skill-hub/api/gen/skillhub"

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
				// Parse key=value
				// (simplified for now)
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
