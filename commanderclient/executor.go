package commanderclient

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// MigrationOperation represents a migration operation to be performed
type MigrationOperation struct {
	EntityID  string
	Operation string // Use Operation* constants from types.go
	Entity    Entity
}

// MigrationResult represents the result of a migration operation
type MigrationResult struct {
	EntityID    string
	Operation   string
	Success     bool
	Error       error
	ProcessedAt time.Time
}

// MigrationExecutor handles the execution of migration operations
type MigrationExecutor struct {
	client    *MigrationClient
	options   *MigrationOptions
	results   []MigrationResult
	resultsMu sync.Mutex
}

// NewMigrationExecutor creates a new migration executor
func NewMigrationExecutor(client *MigrationClient, options *MigrationOptions) *MigrationExecutor {
	if options == nil {
		options = DefaultMigrationOptions()
	}

	return &MigrationExecutor{
		client:  client,
		options: options,
		results: make([]MigrationResult, 0),
	}
}

// ExecuteOperation executes a single migration operation
func (me *MigrationExecutor) ExecuteOperation(ctx context.Context, op *MigrationOperation) *MigrationResult {
	result := &MigrationResult{
		EntityID:    op.EntityID,
		Operation:   op.Operation,
		ProcessedAt: time.Now(),
	}

	if me.options.Confirm {
		confirmed, err := me.confirmOperation(op)
		if err != nil {
			result.Error = err
			me.appendResult(*result)
			return result
		}
		if !confirmed {
			result.Error = fmt.Errorf("operation cancelled by user")
			log.Printf("Skipping %s on entity %s: user cancelled", op.Operation, op.EntityID)
			me.appendResult(*result)
			return result
		}
	}

	if me.options.DryRun {
		log.Printf("[DRY RUN] Would execute %s on entity %s", op.Operation, op.EntityID)
		result.Success = true
		me.appendResult(*result)
		return result
	}

	switch op.Operation {
	case OperationUpsert:
		result.Success, result.Error = me.upsertEntity(ctx, op)
	case OperationUpdate:
		result.Success, result.Error = me.updateEntity(ctx, op)
	case OperationPublish:
		result.Success, result.Error = me.publishEntity(ctx, op)
	case OperationUnpublish:
		result.Success, result.Error = me.unpublishEntity(ctx, op)
	case OperationDelete:
		result.Success, result.Error = me.deleteEntity(ctx, op)
	default:
		result.Error = fmt.Errorf("unsupported operation: %s", op.Operation)
		result.Success = false
	}

	me.appendResult(*result)
	return result
}

// appendResult safely appends a result to the results slice
func (me *MigrationExecutor) appendResult(result MigrationResult) {
	me.resultsMu.Lock()
	me.results = append(me.results, result)
	me.resultsMu.Unlock()
}

// ExecuteBatch executes multiple operations in batch with concurrency
func (me *MigrationExecutor) ExecuteBatch(ctx context.Context, operations []MigrationOperation) []MigrationResult {
	now := time.Now()
	results := make([]MigrationResult, len(operations))

	// Run sequentially if confirmation is required (stdin interaction)
	if me.options.Confirm {
		for i, op := range operations {
			results[i] = *me.ExecuteOperation(ctx, &op)
			log.Printf("Operation %d: %s %s %t %v", i, results[i].Operation, results[i].EntityID, results[i].Success, results[i].Error)
		}
		return results
	}

	// Run concurrently with semaphore to limit parallelism
	concurrency := me.client.GetConcurrency()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, op := range operations {
		wg.Add(1)
		go func(idx int, operation MigrationOperation) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			results[idx] = *me.ExecuteOperation(ctx, &operation)
			log.Printf("Operation %d: %s %s %t %v", idx, results[idx].Operation, results[idx].EntityID, results[idx].Success, results[idx].Error)
		}(i, op)
	}

	wg.Wait()
	duration := time.Since(now)
	log.Printf("Executed %d operations in %02dh:%02dm:%02ds minutes with concurrency %d", len(operations), int(duration.Hours()), int(duration.Minutes())%60, int(duration.Seconds())%60, concurrency)
	return results
}

func (me *MigrationExecutor) confirmOperation(op *MigrationOperation) (bool, error) {
	fmt.Println("\n=== Operation Confirmation ===")
	fmt.Printf("Space: %s\n", me.client.GetSpaceID())
	fmt.Printf("Environment: %s\n", me.client.GetEnvironment())
	fmt.Printf("Entity ID: %s\n", op.EntityID)

	entityType := "unknown"
	contentType := ""
	name := "<unknown>"

	if op.Entity != nil {
		entityType = op.Entity.GetType()
		if ct := op.Entity.GetContentType(); ct != "" {
			contentType = ct
		} else if op.Entity.IsAsset() {
			contentType = "Asset"
		}

		locale := Locale("en")
		if me.client != nil && me.client.spaceModel != nil && me.client.spaceModel.DefaultLocale != "" {
			locale = me.client.spaceModel.DefaultLocale
		}

		if title := op.Entity.GetTitle(locale); title != "" {
			name = title
		} else {
			for _, fallback := range []Locale{Locale("en-US"), Locale("en")} {
				if fallback == locale {
					continue
				}
				if title := op.Entity.GetTitle(fallback); title != "" {
					name = title
					break
				}
			}
			if name == "<unknown>" && op.Entity.IsAsset() {
				if desc := op.Entity.GetDescription(locale); desc != "" {
					name = desc
				}
			}
		}

		if name == "<unknown>" {
			name = op.Entity.GetID()
		}
	}

	if contentType == "" {
		contentType = entityType
	}

	fmt.Printf("Entity Type: %s\n", entityType)
	fmt.Printf("Content Type: %s\n", contentType)
	fmt.Printf("Name: %s\n", name)
	fmt.Printf("Operation: %s\n", op.Operation)
	fmt.Printf("Dry Run: %t\n", me.options.DryRun)
	fmt.Print("Proceed? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("failed to read confirmation input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return true, nil
	}

	input = strings.ToLower(input)
	return input == "y" || input == "yes", nil
}

// GetResults returns all migration results
func (me *MigrationExecutor) GetResults() []MigrationResult {
	return me.results
}

// GetSuccessCount returns the number of successful operations
func (me *MigrationExecutor) GetSuccessCount() int {
	count := 0
	for _, result := range me.results {
		if result.Success {
			count++
		}
	}
	return count
}

// GetErrorCount returns the number of failed operations
func (me *MigrationExecutor) GetErrorCount() int {
	count := 0
	for _, result := range me.results {
		if !result.Success {
			count++
		}
	}
	return count
}

// upsertEntity updates an entity with new fields
func (me *MigrationExecutor) upsertEntity(ctx context.Context, op *MigrationOperation) (bool, error) {
	if op.Entity.GetType() == "Entry" {
		entryEntity := op.Entity.(*EntryEntity)
		entry := entryEntity.Entry

		// Update fields from entity
		fields := op.Entity.GetFields()
		if fields != nil {
			entry.Fields = fields
		}

		// Update the entry
		err := me.client.cma.Entries.Upsert(ctx, me.client.spaceID, entry)
		if err != nil {
			return false, err
		}

		// Refresh the entity in cache
		err = me.client.RefreshEntity(ctx, op.EntityID)
		return err == nil, err

	} else if op.Entity.GetType() == "Asset" {
		assetEntity := op.Entity.(*AssetEntity)
		asset := assetEntity.Asset

		// Update fields from entity
		fields := op.Entity.GetFields()
		if fields != nil {
			// Handle asset field updates
			if titleField, exists := fields["title"]; exists {
				if titleMap, ok := titleField.(map[string]string); ok {
					asset.Fields.Title = titleMap
				}
			}
			if descField, exists := fields["description"]; exists {
				if descMap, ok := descField.(map[string]string); ok {
					asset.Fields.Description = descMap
				}
			}
		}

		// Update the asset
		err := me.client.cma.Assets.Upsert(ctx, me.client.spaceID, asset)
		if err != nil {
			return false, err
		}

		// Refresh the entity in cache
		err = me.client.RefreshEntity(ctx, op.EntityID)
		return err == nil, err
	}

	return false, fmt.Errorf("unsupported entity type: %s", op.Entity.GetType())
}

// updateEntity upserts an entity with new fields and then publishes it only if it's already in published status
func (me *MigrationExecutor) updateEntity(ctx context.Context, op *MigrationOperation) (bool, error) {
	wasPublished := op.Entity.IsPublished()
	success, err := me.upsertEntity(ctx, op)
	if err != nil {
		return false, err
	}
	if success {
		if wasPublished {
			return me.publishEntity(ctx, op)
		}
		return true, nil
	}
	return true, nil
}

// publishEntity publishes an entity
func (me *MigrationExecutor) publishEntity(ctx context.Context, op *MigrationOperation) (bool, error) {
	if op.Entity.GetType() == "Entry" {
		entryEntity := op.Entity.(*EntryEntity)
		entry := entryEntity.Entry

		err := me.client.cma.Entries.Publish(ctx, me.client.spaceID, entry)
		if err != nil {
			return false, err
		}

		// Refresh the entity in cache
		err = me.client.RefreshEntity(ctx, op.EntityID)
		return err == nil, err

	} else if op.Entity.GetType() == "Asset" {
		assetEntity := op.Entity.(*AssetEntity)
		asset := assetEntity.Asset

		err := me.client.cma.Assets.Publish(ctx, me.client.spaceID, asset)
		if err != nil {
			return false, err
		}

		// Refresh the entity in cache
		err = me.client.RefreshEntity(ctx, op.EntityID)
		return err == nil, err
	}

	return false, fmt.Errorf("unsupported entity type: %s", op.Entity.GetType())
}

// unpublishEntity unpublishes an entity
func (me *MigrationExecutor) unpublishEntity(ctx context.Context, op *MigrationOperation) (bool, error) {
	if op.Entity.GetType() == "Entry" {
		entryEntity := op.Entity.(*EntryEntity)
		entry := entryEntity.Entry

		err := me.client.cma.Entries.Unpublish(ctx, me.client.spaceID, entry)
		if err != nil {
			return false, err
		}

		// Refresh the entity in cache
		err = me.client.RefreshEntity(ctx, op.EntityID)
		return err == nil, err

	} else if op.Entity.GetType() == "Asset" {
		assetEntity := op.Entity.(*AssetEntity)
		asset := assetEntity.Asset

		err := me.client.cma.Assets.Unpublish(ctx, me.client.spaceID, asset)
		if err != nil {
			return false, err
		}

		// Refresh the entity in cache
		err = me.client.RefreshEntity(ctx, op.EntityID)
		return err == nil, err
	}

	return false, fmt.Errorf("unsupported entity type: %s", op.Entity.GetType())
}

// deleteEntity deletes an entity
func (me *MigrationExecutor) deleteEntity(ctx context.Context, op *MigrationOperation) (bool, error) {
	if op.Entity.GetType() == "Entry" {
		err := me.client.cma.Entries.Delete(ctx, me.client.spaceID, op.EntityID)
		if err != nil {
			return false, err
		}

		// Remove from cache
		me.client.RemoveEntity(op.EntityID)
		return true, nil

	} else if op.Entity.GetType() == "Asset" {
		assetEntity := op.Entity.(*AssetEntity)
		asset := assetEntity.Asset

		err := me.client.cma.Assets.Delete(ctx, me.client.spaceID, asset)
		if err != nil {
			return false, err
		}

		// Remove from cache
		me.client.RemoveEntity(op.EntityID)
		return true, nil
	}

	return false, fmt.Errorf("unsupported entity type: %s", op.Entity.GetType())
}

// CreateUpdateOperation creates a migration operation
func CreateUpdateOperation(entityID string, entity Entity) *MigrationOperation {
	return &MigrationOperation{
		EntityID:  entityID,
		Operation: OperationUpdate,
		Entity:    entity,
	}
}

// CreateFieldUpdate creates a field update for a specific field and locale
func CreateFieldUpdate(fieldName string, locale Locale, value any) map[string]any {
	fields := make(map[string]any)
	fieldMap := make(map[string]any)
	fieldMap[string(locale)] = value
	fields[fieldName] = fieldMap
	return fields
}
