package commanderclient

import (
	"context"
	"fmt"
	"log"
	"os"
)

// Config holds configuration for the migration library
type Config struct {
	CMAToken    string
	SpaceID     string
	Environment string
	Verbose     bool
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	return &Config{
		CMAToken:    os.Getenv("CONTENTFUL_CMAKEY"),
		SpaceID:     os.Getenv("CONTENTFUL_SPACE_ID"),
		Environment: getEnvOrDefault("CONTENTFUL_ENVIRONMENT", "dev"),
		Verbose:     getEnvOrDefault("CONTENTFUL_VERBOSE", "true") == "true",
	}
}

// ValidateConfig validates the configuration
func (c *Config) ValidateConfig() error {
	if c.CMAToken == "" {
		return fmt.Errorf("CMA token is required")
	}
	if c.SpaceID == "" {
		return fmt.Errorf("space ID is required")
	}
	return nil
}

// Init creates a ready-to-use migration client with logger and loaded space model
func Init(config *Config) (*MigrationClient, *Logger, error) {
	if err := config.ValidateConfig(); err != nil {
		return nil, nil, err
	}

	// Create client
	client := newMigrationClient(config.CMAToken, config.SpaceID, config.Environment)

	// Create logger
	logger := NewLogger(config.Verbose)

	if config.Verbose {
		logger.Info("Created migration client for space %s in environment %s", config.SpaceID, config.Environment)
	}

	// Load space model
	ctx := context.Background()
	if err := client.LoadSpaceModel(ctx, logger); err != nil {
		return nil, logger, fmt.Errorf("failed to load space model: %w", err)
	}

	if config.Verbose {
		logger.Info("Successfully loaded space")
		logger.Info(client.GetStats().Printf())
	}

	return client, logger, nil
}

// Utility functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Logger provides structured logging for migrations
type Logger struct {
	verbose bool
}

// NewLogger creates a new logger
func NewLogger(verbose bool) *Logger {
	return &Logger{verbose: verbose}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...any) {
	log.Printf("[INFO] "+format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...any) {
	log.Printf("[WARN] "+format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...any) {
	log.Printf("[ERROR] "+format, args...)
}

// Debug logs a debug message (only if verbose is enabled)
func (l *Logger) Debug(format string, args ...any) {
	if l.verbose {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Migration helpers

// PrintStats prints migration statistics
func PrintStats(stats *MigrationStats) {
	fmt.Printf("\n=== Migration Statistics ===\n")
	fmt.Printf("Total Entities: %d\n", stats.TotalEntities)
	fmt.Printf("Processed Entries: %d\n", stats.ProcessedEntries)
	fmt.Printf("Processed Assets: %d\n", stats.ProcessedAssets)
	fmt.Printf("Errors: %d\n", stats.Errors)
	fmt.Printf("Duration: %v\n", stats.EndTime.Sub(stats.StartTime))
	fmt.Printf("===========================\n")
}

// PrintResults prints migration results
func PrintResults(results []MigrationResult) {
	fmt.Printf("\n=== Migration Results ===\n")
	successCount := 0
	errorCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
			fmt.Printf("✓ %s %s\n", result.Operation, result.EntityID)
		} else {
			errorCount++
			fmt.Printf("✗ %s %s: %v\n", result.Operation, result.EntityID, result.Error)
		}
	}

	fmt.Printf("\nSummary: %d successful, %d failed\n", successCount, errorCount)
	fmt.Printf("==========================\n")
}
