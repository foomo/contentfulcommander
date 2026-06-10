package commanderclient

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestUpdateSpaceModelDebug is an interactive test for debugging UpdateSpaceModel.
// It loads the space model, then loops waiting for you to edit entities in Contentful.
// After each Enter keypress it calls UpdateSpaceModel and prints what changed.
//
// Run with:
//
//	go test ./commanderclient -run TestUpdateSpaceModelDebug -v -count=1
//
// Requires CONTENTFUL_CMAKEY, CONTENTFUL_SPACE_ID (and optionally CONTENTFUL_CDAKEY).
func TestUpdateSpaceModelDebug(t *testing.T) {
	if os.Getenv("CONTENTFUL_CMAKEY") == "" || os.Getenv("CONTENTFUL_SPACE_ID") == "" {
		t.Skip("skipping: CONTENTFUL_CMAKEY and CONTENTFUL_SPACE_ID must be set")
	}

	config := LoadConfigFromEnv()
	client, logger, err := Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	t.Logf("Loaded %d entities (last updated: %s)", len(client.cache), client.spaceModel.LastUpdated.Format("15:04:05.000"))

	// Take a snapshot of current state
	type snapshot struct {
		version   int
		updatedAt string
	}
	snap := make(map[string]snapshot, len(client.cache))
	for id, entity := range client.cache {
		snap[id] = snapshot{
			version:   entity.GetVersion(),
			updatedAt: entity.GetSys().UpdatedAt,
		}
	}

	reader := bufio.NewReader(os.Stdin)
	for i := 1; ; i++ {
		fmt.Printf("\n--- Iteration %d ---\n", i)
		fmt.Print("Edit entries in Contentful, then press Enter to update (or type 'q' + Enter to quit)... ")

		input, readErr := reader.ReadString('\n')
		if strings.TrimSpace(input) == "q" {
			break
		}
		if readErr != nil {
			// EOF (e.g. non-interactive run) — stop instead of looping forever.
			break
		}

		if err := client.UpdateSpaceModel(context.Background(), logger); err != nil {
			t.Fatalf("UpdateSpaceModel failed: %v", err)
		}

		// Diff against snapshot
		changed := 0
		added := 0
		for id, entity := range client.cache {
			prev, existed := snap[id]
			if !existed {
				added++
				t.Logf("  + NEW  %-8s %-20s %s (v%d)",
					entity.GetType(), entity.GetContentType(), id, entity.GetVersion())
				continue
			}
			if entity.GetVersion() != prev.version || entity.GetSys().UpdatedAt != prev.updatedAt {
				changed++
				label := entity.GetTitle(client.GetDefaultLocale())
				if label == "" {
					label = id
				}
				t.Logf("  ~ CHG  %-8s %-20s %s  v%d→v%d  %s→%s",
					entity.GetType(), entity.GetContentType(), label,
					prev.version, entity.GetVersion(),
					prev.updatedAt, entity.GetSys().UpdatedAt)
			}
		}

		if changed == 0 && added == 0 {
			t.Log("  (no changes detected)")
		} else {
			t.Logf("  Total: %d changed, %d new", changed, added)
		}
		t.Logf("  Cache size: %d entities (last updated: %s)", len(client.cache), client.spaceModel.LastUpdated.Format("15:04:05.000"))

		// Refresh snapshot
		snap = make(map[string]snapshot, len(client.cache))
		for id, entity := range client.cache {
			snap[id] = snapshot{
				version:   entity.GetVersion(),
				updatedAt: entity.GetSys().UpdatedAt,
			}
		}
	}
}
