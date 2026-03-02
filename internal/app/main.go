package app

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"foodcli/internal/core"
	"foodcli/internal/providers"
	"gopkg.in/yaml.v3"
)

type Config = core.Config

// YamlConfig is an intermediate struct to parse YAML `timeout_seconds`.
type YamlConfig struct {
	SuccessURLPattern string `yaml:"success_url_pattern"`
	SuccessSelector   string `yaml:"success_selector"`
	UserDataDir       string `yaml:"user_data_dir"`
	VenueBaseURL      string `yaml:"venue_base_url"`
	Provider          string `yaml:"provider"`
	Headless          bool   `yaml:"headless"`
	TimeoutSeconds    int    `yaml:"timeout_seconds"`
}

func resolveUserDataDir(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("user_data_dir must be set and cannot be empty")
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for user_data_dir: %w", err)
	}
	cleaned := filepath.Clean(absPath)
	if filepath.Dir(cleaned) == cleaned {
		return "", fmt.Errorf("user_data_dir cannot be a filesystem root path: %s", cleaned)
	}

	return cleaned, nil
}

func resolveVenueBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("venue_base_url must be set and cannot be empty")
	}

	cleaned := strings.TrimRight(trimmed, "/")
	parsed, err := url.Parse(cleaned)
	if err != nil {
		return "", fmt.Errorf("invalid venue_base_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("venue_base_url must include scheme and host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("venue_base_url cannot include query parameters or fragments")
	}

	return cleaned, nil
}

func validateEraseUserDataDir(path string) error {
	return core.ValidateEraseUserDataDir(path)
}

// loadConfig reads a YAML file from the given path and returns a Config struct.
func loadConfig(path string) (Config, error) {
	var cfg Config
	var yamlCfg YamlConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &yamlCfg); err != nil {
		return cfg, fmt.Errorf("unmarshal yaml: %w", err)
	}

	cfg.SuccessURLPattern = yamlCfg.SuccessURLPattern
	cfg.SuccessSelector = yamlCfg.SuccessSelector
	resolvedUserDataDir, err := resolveUserDataDir(yamlCfg.UserDataDir)
	if err != nil {
		return cfg, err
	}
	cfg.UserDataDir = resolvedUserDataDir
	resolvedVenueBaseURL, err := resolveVenueBaseURL(yamlCfg.VenueBaseURL)
	if err != nil {
		return cfg, err
	}
	cfg.VenueBaseURL = resolvedVenueBaseURL
	resolvedProvider, err := providers.ResolveProviderName(yamlCfg.Provider)
	if err != nil {
		return cfg, err
	}
	cfg.Provider = resolvedProvider
	cfg.Headless = yamlCfg.Headless
	cfg.Timeout = time.Duration(yamlCfg.TimeoutSeconds) * time.Second

	return cfg, nil
}

func Main() {
	rand.Seed(time.Now().UnixNano())
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]
	var configFile string
	var eraseData bool
	var queryParts []string

	// Handle flags and arguments
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--erase-data" {
			if command != "auth" {
				fmt.Fprintln(os.Stderr, "Error: --erase-data flag is only valid for the 'auth' command.")
				printUsage()
				os.Exit(1)
			}
			eraseData = true
			continue
		}

		if strings.HasSuffix(args[i], ".yml") || strings.HasSuffix(args[i], ".yaml") {
			configFile = args[i]
			continue
		}

		queryParts = append(queryParts, args[i])
	}

	if configFile == "" {
		configFile = "config.yml" // Default filename
	}

	cfg, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load config from '%s': %v", configFile, err)
	}
	provider, err := providers.New(cfg.Provider)
	if err != nil {
		log.Fatalf("Failed to initialize provider '%s': %v", cfg.Provider, err)
	}

	switch command {
	case "auth":
		if err := provider.Auth(cfg, eraseData); err != nil {
			resetTerminalInputModes(os.Stderr)
			printAuthStatus("failed", err)
			os.Exit(1)
		}
		resetTerminalInputModes(os.Stderr)
		printAuthStatus("success", nil)
	case "search":
		if len(queryParts) == 0 {
			log.Fatalf("Search command requires at least one argument.")
		}
		query := strings.Join(queryParts, " ")
		if err := provider.Search(cfg, query); err != nil {
			log.Fatalf("Search failed: %v", err)
		}
	case "basket":
		if len(queryParts) == 0 {
			if err := provider.Basket(cfg); err != nil {
				log.Fatalf("Basket failed: %v", err)
			}
			break
		}
		switch strings.ToLower(queryParts[0]) {
		case "add":
			if len(queryParts) != 3 {
				log.Fatalf("Basket add requires exactly 2 arguments: <venue_slug> <item_id>.")
			}
			venueSlug := queryParts[1]
			itemID := queryParts[2]
			if err := provider.BasketAdd(cfg, venueSlug, itemID); err != nil {
				log.Fatalf("Basket add failed: %v", err)
			}
		case "remove":
			if len(queryParts) != 3 {
				log.Fatalf("Basket remove requires exactly 2 arguments: <venue_slug> <item_id>.")
			}
			venueSlug := queryParts[1]
			itemID := queryParts[2]
			if err := provider.BasketRemove(cfg, venueSlug, itemID); err != nil {
				log.Fatalf("Basket remove failed: %v", err)
			}
		default:
			log.Fatalf("Unknown basket subcommand: %s. Use 'basket', 'basket add <venue_slug> <item_id>' or 'basket remove <venue_slug> <item_id>'.", queryParts[0])
		}
	case "checkout":
		if len(queryParts) != 1 {
			log.Fatalf("Checkout command requires exactly 1 argument: <venue_slug>.")
		}
		venueSlug := queryParts[0]
		if err := provider.Checkout(cfg, venueSlug); err != nil {
			log.Fatalf("Checkout failed: %v", err)
		}
	default:
		log.Fatalf("Unknown command: %s. Use 'auth', 'search', 'basket', or 'checkout'.", command)
	}
}

func printAuthStatus(status string, authErr error) {
	result := map[string]interface{}{
		"auth_status": status,
	}
	if authErr != nil {
		result["error"] = authErr.Error()
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		fmt.Printf("{\"auth_status\":%q}\n", status)
		return
	}
	fmt.Println(string(resultJSON))
}

func resetTerminalInputModes(file *os.File) {
	if file == nil {
		return
	}
	info, err := file.Stat()
	if err != nil {
		return
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return
	}
	// Reset common interactive terminal input modes so arrow keys continue to work.
	fmt.Fprint(file, "\x1b[?1l\x1b[?2004l\x1b>")
}
