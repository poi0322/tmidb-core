package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
)

// 백업 명령어
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup and restore tmiDB data",
	Long:  "Create backups and restore tmiDB data including database, configuration, and files",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new backup",
	Long: `Create a new backup of tmiDB data.
	
Examples:
  # Create backup with auto-generated name
  tmidb-cli backup create
  
  # Create backup with custom name
  tmidb-cli backup create production-backup
  
  # Create backup with specific components
  tmidb-cli backup create --components=database,config`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			// 자동 생성 이름
			name = fmt.Sprintf("tmidb-backup-%s", time.Now().Format("20060102-150405"))
		}

		components, _ := cmd.Flags().GetStringSlice("components")
		compress, _ := cmd.Flags().GetBool("compress")
		outputDir, _ := cmd.Flags().GetString("output")

		fmt.Printf("🔐 Creating backup: %s\n", name)
		fmt.Printf("   Components: %s\n", strings.Join(components, ", "))
		fmt.Printf("   Output: %s\n", outputDir)
		if compress {
			fmt.Println("   Compression: enabled")
		}

		// 백업 시작 전 확인
		if !cmd.Flag("yes").Changed {
			fmt.Print("\n⚠️  This will create a backup. Continue? (yes/no): ")
			var response string
			fmt.Scanln(&response)
			if response != "yes" {
				fmt.Println("❌ Backup cancelled")
				return
			}
		}

		// 백업 요청
		resp, err := client.SendMessage(ipc.MessageTypeBackupCreate, map[string]any{
			"name":       name,
			"components": components,
			"compress":   compress,
			"output_dir": outputDir,
		})
		if err != nil {
			fmt.Printf("❌ Failed to create backup: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 진행 상황 표시
		if backupInfo, ok := resp.Data.(map[string]any); ok {
			backupID := backupInfo["id"].(string)

			// 백업 진행 상황 모니터링
			if err := monitorBackupProgress(backupID); err != nil {
				fmt.Printf("❌ Backup monitoring error: %v\n", err)
				return
			}

			fmt.Printf("\n✅ Backup created successfully\n")
			fmt.Printf("   ID: %s\n", backupID)
			fmt.Printf("   Path: %s\n", backupInfo["path"])
			fmt.Printf("   Size: %s\n", formatBytes(int64(backupInfo["size"].(float64))))
		}
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore <backup-id|path>",
	Short: "Restore from a backup",
	Long: `Restore tmiDB data from a backup.
	
Examples:
  # Restore from backup ID
  tmidb-cli backup restore backup-20240101-120000
  
  # Restore from file path
  tmidb-cli backup restore /path/to/backup.tar.gz
  
  # Restore specific components
  tmidb-cli backup restore backup-123 --components=database`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		backup := args[0]
		components, _ := cmd.Flags().GetStringSlice("components")

		fmt.Printf("🔓 Restoring from backup: %s\n", backup)

		// 복구 전 경고
		fmt.Println("\n⚠️  WARNING: This will overwrite existing data!")
		fmt.Println("   - All services will be stopped during restore")
		fmt.Println("   - Existing data will be replaced")
		fmt.Println("   - This operation cannot be undone")

		if !cmd.Flag("yes").Changed {
			fmt.Print("\nAre you SURE you want to continue? (yes/no): ")
			var response string
			fmt.Scanln(&response)
			if response != "yes" {
				fmt.Println("❌ Restore cancelled")
				return
			}
		}

		// 복구 요청
		resp, err := client.SendMessage(ipc.MessageTypeBackupRestore, map[string]any{
			"backup":     backup,
			"components": components,
		})
		if err != nil {
			fmt.Printf("❌ Failed to restore backup: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 복구 진행 상황 모니터링
		if restoreInfo, ok := resp.Data.(map[string]any); ok {
			restoreID := restoreInfo["id"].(string)

			if err := monitorRestoreProgress(restoreID); err != nil {
				fmt.Printf("❌ Restore monitoring error: %v\n", err)
				return
			}

			fmt.Println("\n✅ Restore completed successfully")
			fmt.Println("🔄 Restarting services...")

			// 서비스 재시작
			client.SendMessage(ipc.MessageTypeProcessRestart, map[string]any{
				"component": "all",
			})
		}
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	Long:  "Display all available backups with their details",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📋 Available Backups:")

		resp, err := client.SendMessage(ipc.MessageTypeBackupList, nil)
		if err != nil {
			fmt.Printf("❌ Failed to list backups: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 백업 목록 표시
		if backups, ok := resp.Data.([]any); ok {
			if len(backups) == 0 {
				fmt.Println("   No backups found")
				return
			}

			fmt.Printf("\n%-30s %-20s %-15s %-20s\n", "ID", "CREATED", "SIZE", "COMPONENTS")
			fmt.Println(strings.Repeat("-", 85))

			for _, backup := range backups {
				if b, ok := backup.(map[string]any); ok {
					id := b["id"].(string)
					created := b["created"].(string)
					size := formatBytes(int64(b["size"].(float64)))
					components := strings.Join(toStringSlice(b["components"].([]any)), ", ")

					fmt.Printf("%-30s %-20s %-15s %-20s\n", id, created, size, components)
				}
			}
		}
	},
}

var backupDeleteCmd = &cobra.Command{
	Use:   "delete <backup-id>",
	Short: "Delete a backup",
	Long:  "Delete a specific backup by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		backupID := args[0]

		fmt.Printf("🗑️  Deleting backup: %s\n", backupID)

		if !cmd.Flag("yes").Changed {
			fmt.Print("Are you sure? (yes/no): ")
			var response string
			fmt.Scanln(&response)
			if response != "yes" {
				fmt.Println("❌ Delete cancelled")
				return
			}
		}

		resp, err := client.SendMessage(ipc.MessageTypeBackupDelete, map[string]any{
			"id": backupID,
		})
		if err != nil {
			fmt.Printf("❌ Failed to delete backup: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		fmt.Println("✅ Backup deleted successfully")
	},
}

var backupVerifyCmd = &cobra.Command{
	Use:   "verify <backup-id|path>",
	Short: "Verify backup integrity",
	Long:  "Check backup file integrity and contents",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		backup := args[0]

		fmt.Printf("🔍 Verifying backup: %s\n", backup)

		resp, err := client.SendMessage(ipc.MessageTypeBackupVerify, map[string]any{
			"backup": backup,
		})
		if err != nil {
			fmt.Printf("❌ Failed to verify backup: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Verification failed: %s\n", resp.Error)
			return
		}

		// 검증 결과 표시
		if result, ok := resp.Data.(map[string]any); ok {
			fmt.Println("\n📊 Verification Results:")
			fmt.Printf("   Status: %s\n", result["status"])
			fmt.Printf("   Integrity: %s\n", result["integrity"])

			if components, ok := result["components"].(map[string]any); ok {
				fmt.Println("\n   Components:")
				for comp, status := range components {
					icon := "✅"
					if status != "valid" {
						icon = "❌"
					}
					fmt.Printf("     %s %s: %v\n", icon, comp, status)
				}
			}

			if errors, ok := result["errors"].([]any); ok && len(errors) > 0 {
				fmt.Println("\n   Errors:")
				for _, err := range errors {
					fmt.Printf("     - %v\n", err)
				}
			}
		}
	},
}

// 백업 진행 상황 모니터링
func monitorBackupProgress(backupID string) error {
	fmt.Println("\n📊 Backup Progress:")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := client.SendMessage(ipc.MessageTypeBackupProgress, map[string]any{
				"id": backupID,
			})
			if err != nil {
				return err
			}

			if progress, ok := resp.Data.(map[string]any); ok {
				status := progress["status"].(string)
				percent := int(progress["percent"].(float64))
				current := progress["current"].(string)

				// 진행 바 표시
				fmt.Printf("\r   %s [", current)
				barLength := 30
				filled := barLength * percent / 100
				for i := 0; i < barLength; i++ {
					if i < filled {
						fmt.Print("█")
					} else {
						fmt.Print("░")
					}
				}
				fmt.Printf("] %d%%", percent)

				if status == "completed" || status == "failed" {
					fmt.Println()
					return nil
				}
			}
		}
	}
}

// 복구 진행 상황 모니터링
func monitorRestoreProgress(restoreID string) error {
	fmt.Println("\n📊 Restore Progress:")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := client.SendMessage(ipc.MessageTypeRestoreProgress, map[string]any{
				"id": restoreID,
			})
			if err != nil {
				return err
			}

			if progress, ok := resp.Data.(map[string]any); ok {
				status := progress["status"].(string)
				percent := int(progress["percent"].(float64))
				current := progress["current"].(string)

				// 진행 바 표시
				fmt.Printf("\r   %s [", current)
				barLength := 30
				filled := barLength * percent / 100
				for i := 0; i < barLength; i++ {
					if i < filled {
						fmt.Print("█")
					} else {
						fmt.Print("░")
					}
				}
				fmt.Printf("] %d%%", percent)

				if status == "completed" || status == "failed" {
					fmt.Println()
					return nil
				}
			}
		}
	}
}

// Helper function
func toStringSlice(slice []any) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = fmt.Sprintf("%v", v)
	}
	return result
}

func init() {
	// 플래그 설정
	backupCreateCmd.Flags().StringSlice("components", []string{"database", "config", "files"}, "Components to backup")
	backupCreateCmd.Flags().Bool("compress", true, "Compress backup file")
	backupCreateCmd.Flags().String("output", "./backups", "Output directory")
	backupCreateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	backupRestoreCmd.Flags().StringSlice("components", []string{}, "Components to restore (default: all)")
	backupRestoreCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	backupDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	// 서브커맨드 추가
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupDeleteCmd)
	backupCmd.AddCommand(backupVerifyCmd)

	// 루트 명령어에 추가
	rootCmd.AddCommand(backupCmd)
}
