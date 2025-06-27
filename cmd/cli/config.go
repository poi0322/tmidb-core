package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tmidb/tmidb-core/internal/ipc"
	"gopkg.in/yaml.v3"
)

// 설정 명령어
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tmiDB configuration",
	Long:  "Get, set, and manage configuration for tmiDB components",
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get configuration value",
	Long: `Get configuration value for a specific key or all configuration.
	
Examples:
  # Get all configuration
  tmidb-cli config get
  
  # Get specific configuration
  tmidb-cli config get api.port
  tmidb-cli config get log.level`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		fmt.Printf("📋 Getting configuration")
		if key != "" {
			fmt.Printf(" for key: %s", key)
		}
		fmt.Println()

		// 설정 요청
		resp, err := client.SendMessage(ipc.MessageTypeConfigGet, map[string]interface{}{
			"key": key,
		})
		if err != nil {
			fmt.Printf("❌ Failed to get configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 설정 출력
		if output, _ := cmd.Flags().GetString("output"); output == "json" {
			data, _ := json.MarshalIndent(resp.Data, "", "  ")
			fmt.Println(string(data))
		} else if output == "yaml" {
			data, _ := yaml.Marshal(resp.Data)
			fmt.Println(string(data))
		} else {
			// 기본 형식
			printConfig(resp.Data, 0)
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set configuration value",
	Long: `Set configuration value for a specific key.
	
Examples:
  # Set log level
  tmidb-cli config set log.level debug
  
  # Set API port
  tmidb-cli config set api.port 8080
  
  # Enable feature
  tmidb-cli config set features.hot_reload true`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]

		// 값 타입 추론
		var typedValue interface{}
		if value == "true" || value == "false" {
			typedValue = value == "true"
		} else if num, err := fmt.Sscanf(value, "%d", new(int)); err == nil && num == 1 {
			fmt.Sscanf(value, "%d", &typedValue)
		} else {
			typedValue = value
		}

		fmt.Printf("⚙️  Setting %s = %v\n", key, typedValue)

		// 설정 요청
		resp, err := client.SendMessage(ipc.MessageTypeConfigSet, map[string]interface{}{
			"key":   key,
			"value": typedValue,
		})
		if err != nil {
			fmt.Printf("❌ Failed to set configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		fmt.Printf("✅ Configuration updated successfully\n")

		// 재시작 필요 여부 확인
		if needsRestart, ok := resp.Data.(map[string]interface{})["needs_restart"].(bool); ok && needsRestart {
			fmt.Printf("⚠️  This change requires a restart to take effect\n")
			if component, ok := resp.Data.(map[string]interface{})["component"].(string); ok {
				fmt.Printf("   Run: tmidb-cli process restart %s\n", component)
			}
		}
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration keys",
	Long:  "Display all available configuration keys and their current values",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("📋 Configuration Keys:")

		resp, err := client.SendMessage(ipc.MessageTypeConfigList, nil)
		if err != nil {
			fmt.Printf("❌ Failed to list configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 설정 목록 출력
		if configs, ok := resp.Data.([]interface{}); ok {
			for _, config := range configs {
				if configMap, ok := config.(map[string]interface{}); ok {
					key := configMap["key"].(string)
					value := configMap["value"]
					description := configMap["description"].(string)
					configType := configMap["type"].(string)

					fmt.Printf("\n🔸 %s\n", key)
					fmt.Printf("   Type:  %s\n", configType)
					fmt.Printf("   Value: %v\n", value)
					fmt.Printf("   Desc:  %s\n", description)
				}
			}
		}
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset [key]",
	Short: "Reset configuration to default",
	Long: `Reset configuration to default values.
	
Examples:
  # Reset specific configuration
  tmidb-cli config reset log.level
  
  # Reset all configuration (requires confirmation)
  tmidb-cli config reset --all`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		all, _ := cmd.Flags().GetBool("all")

		if all {
			fmt.Printf("⚠️  This will reset ALL configuration to default values.\n")
			fmt.Printf("   Are you sure? (yes/no): ")

			var response string
			fmt.Scanln(&response)
			if response != "yes" {
				fmt.Println("❌ Reset cancelled")
				return
			}

			fmt.Println("🔄 Resetting all configuration...")
		} else if len(args) == 0 {
			fmt.Println("❌ Please specify a key or use --all flag")
			return
		} else {
			fmt.Printf("🔄 Resetting configuration for key: %s\n", args[0])
		}

		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		resp, err := client.SendMessage(ipc.MessageTypeConfigReset, map[string]interface{}{
			"key": key,
			"all": all,
		})
		if err != nil {
			fmt.Printf("❌ Failed to reset configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		fmt.Println("✅ Configuration reset successfully")
	},
}

var configExportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export configuration to file",
	Long: `Export current configuration to a file.
	
Examples:
  # Export to default location
  tmidb-cli config export
  
  # Export to specific file
  tmidb-cli config export ./config-backup.yaml`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := "tmidb-config-export.yaml"
		if len(args) > 0 {
			filename = args[0]
		}

		fmt.Printf("📤 Exporting configuration to: %s\n", filename)

		// 설정 가져오기
		resp, err := client.SendMessage(ipc.MessageTypeConfigGet, map[string]interface{}{
			"key": "",
		})
		if err != nil {
			fmt.Printf("❌ Failed to get configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		// 파일로 저장
		format := filepath.Ext(filename)
		var data []byte

		switch format {
		case ".json":
			data, err = json.MarshalIndent(resp.Data, "", "  ")
		default:
			data, err = yaml.Marshal(resp.Data)
		}

		if err != nil {
			fmt.Printf("❌ Failed to marshal configuration: %v\n", err)
			return
		}

		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Printf("❌ Failed to write file: %v\n", err)
			return
		}

		fmt.Printf("✅ Configuration exported successfully\n")
	},
}

var configImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import configuration from file",
	Long: `Import configuration from a file.
	
Examples:
  # Import from YAML file
  tmidb-cli config import ./config-backup.yaml
  
  # Import from JSON file
  tmidb-cli config import ./config.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]

		fmt.Printf("📥 Importing configuration from: %s\n", filename)

		// 파일 읽기
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("❌ Failed to read file: %v\n", err)
			return
		}

		// 파싱
		var config map[string]interface{}
		format := filepath.Ext(filename)

		switch format {
		case ".json":
			err = json.Unmarshal(data, &config)
		default:
			err = yaml.Unmarshal(data, &config)
		}

		if err != nil {
			fmt.Printf("❌ Failed to parse configuration: %v\n", err)
			return
		}

		// 설정 적용
		resp, err := client.SendMessage(ipc.MessageTypeConfigImport, map[string]interface{}{
			"config": config,
		})
		if err != nil {
			fmt.Printf("❌ Failed to import configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Error: %s\n", resp.Error)
			return
		}

		fmt.Println("✅ Configuration imported successfully")

		// 변경 사항 표시
		if changes, ok := resp.Data.(map[string]interface{})["changes"].([]interface{}); ok && len(changes) > 0 {
			fmt.Printf("\n📝 Applied %d changes:\n", len(changes))
			for _, change := range changes {
				fmt.Printf("   - %v\n", change)
			}
		}
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate configuration",
	Long: `Validate configuration file or current configuration.
	
Examples:
  # Validate current configuration
  tmidb-cli config validate
  
  # Validate configuration file
  tmidb-cli config validate ./config.yaml`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var config map[string]interface{}

		if len(args) > 0 {
			// 파일에서 읽기
			filename := args[0]
			fmt.Printf("📋 Validating configuration file: %s\n", filename)

			data, err := os.ReadFile(filename)
			if err != nil {
				fmt.Printf("❌ Failed to read file: %v\n", err)
				return
			}

			format := filepath.Ext(filename)
			switch format {
			case ".json":
				err = json.Unmarshal(data, &config)
			default:
				err = yaml.Unmarshal(data, &config)
			}

			if err != nil {
				fmt.Printf("❌ Failed to parse configuration: %v\n", err)
				return
			}
		} else {
			fmt.Println("📋 Validating current configuration...")
		}

		// 검증 요청
		resp, err := client.SendMessage(ipc.MessageTypeConfigValidate, map[string]interface{}{
			"config": config,
		})
		if err != nil {
			fmt.Printf("❌ Failed to validate configuration: %v\n", err)
			return
		}

		if !resp.Success {
			fmt.Printf("❌ Validation failed: %s\n", resp.Error)
			return
		}

		fmt.Println("✅ Configuration is valid")

		// 경고 표시
		if warnings, ok := resp.Data.(map[string]interface{})["warnings"].([]interface{}); ok && len(warnings) > 0 {
			fmt.Printf("\n⚠️  %d warnings:\n", len(warnings))
			for _, warning := range warnings {
				fmt.Printf("   - %v\n", warning)
			}
		}
	},
}

// 설정 출력 헬퍼
func printConfig(data interface{}, indent int) {
	prefix := strings.Repeat("  ", indent)

	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if nested, ok := value.(map[string]interface{}); ok {
				fmt.Printf("%s%s:\n", prefix, key)
				printConfig(nested, indent+1)
			} else {
				fmt.Printf("%s%s: %v\n", prefix, key, value)
			}
		}
	default:
		fmt.Printf("%s%v\n", prefix, v)
	}
}

func init() {
	// 플래그 추가
	configGetCmd.Flags().StringP("output", "o", "text", "Output format (text, json, yaml)")
	configResetCmd.Flags().Bool("all", false, "Reset all configuration")

	// 서브커맨드 추가
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configExportCmd)
	configCmd.AddCommand(configImportCmd)
	configCmd.AddCommand(configValidateCmd)

	// 루트 명령어에 추가
	rootCmd.AddCommand(configCmd)
}
