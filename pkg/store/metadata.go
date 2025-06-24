package store

import (
	"context"
	"fmt"
)

// MetadataKey represents a metadata key with its description
type MetadataKey struct {
	Key         string
	Description string
}

// MetadataEntry represents a metadata key-value pair with description
type MetadataEntry struct {
	Key         string
	Value       []byte
	Description string
}

// WellKnownMetadataKeys defines all the known metadata keys used by Rollkit
var WellKnownMetadataKeys = []MetadataKey{
	{
		Key:         "d",
		Description: "DA included height - the highest DA layer height that has been processed",
	},
	{
		Key:         "l", 
		Description: "Last batch data - the most recent batch data submitted to DA layer",
	},
	{
		Key:         "rhb",
		Description: "Rollkit height to DA height mapping - maps rollkit heights to DA heights",
	},
	{
		Key:         "last-submitted-header-height",
		Description: "Last submitted header height - the height of the last header submitted to DA layer",
	},
	{
		Key:         "last-submitted-data-height", 
		Description: "Last submitted data height - the height of the last data submitted to DA layer",
	},
}

// GetAllMetadata returns all available metadata entries from the store
func (s *DefaultStore) GetAllMetadata(ctx context.Context) ([]MetadataEntry, error) {
	var entries []MetadataEntry
	
	for _, metaKey := range WellKnownMetadataKeys {
		value, err := s.GetMetadata(ctx, metaKey.Key)
		if err != nil {
			// Skip keys that don't exist rather than failing entirely
			continue
		}
		
		entries = append(entries, MetadataEntry{
			Key:         metaKey.Key,
			Value:       value,
			Description: metaKey.Description,
		})
	}
	
	// Also check for dynamic keys with known patterns
	entries = append(entries, s.getDynamicMetadataEntries(ctx)...)
	
	return entries, nil
}

// getDynamicMetadataEntries gets metadata entries that follow known patterns
func (s *DefaultStore) getDynamicMetadataEntries(ctx context.Context) []MetadataEntry {
	var entries []MetadataEntry
	
	// Get the current height to check for rhb patterns
	height, err := s.Height(ctx)
	if err != nil {
		return entries
	}
	
	// Check for rollkit height to DA height mappings (pattern: "rhb/{height}/h" and "rhb/{height}/d")
	for h := uint64(1); h <= height; h++ {
		headerKey := fmt.Sprintf("rhb/%d/h", h)
		if value, err := s.GetMetadata(ctx, headerKey); err == nil {
			entries = append(entries, MetadataEntry{
				Key:         headerKey,
				Value:       value,
				Description: fmt.Sprintf("DA height for rollkit height %d header", h),
			})
		}
		
		dataKey := fmt.Sprintf("rhb/%d/d", h)
		if value, err := s.GetMetadata(ctx, dataKey); err == nil {
			entries = append(entries, MetadataEntry{
				Key:         dataKey,
				Value:       value,
				Description: fmt.Sprintf("DA height for rollkit height %d data", h),
			})
		}
	}
	
	return entries
}