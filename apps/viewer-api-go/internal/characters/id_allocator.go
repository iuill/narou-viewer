package characters

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

func NewGeneratedCharacterIDAllocator(novelID string, existing []GeneratedCharacter) *GeneratedCharacterIDAllocator {
	used := generatedCharacterIDSet(existing)
	nextOrdinal := 1
	advanceNextStableCharacterOrdinal(novelID, &nextOrdinal, used)
	return &GeneratedCharacterIDAllocator{
		novelID:     novelID,
		nextOrdinal: nextOrdinal,
		usedIDs:     used,
		issuedIDs:   map[string]bool{},
		retiredIDs:  map[string]string{},
	}
}

func LoadGeneratedCharacterIDAllocator(stateDir string, novelID string, existing []GeneratedCharacter) (*GeneratedCharacterIDAllocator, error) {
	doc, _, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil {
		return nil, err
	}
	used := generatedCharacterIDSet(existing)
	for _, record := range doc.Characters {
		id := strings.TrimSpace(record.CharacterID)
		if id != "" {
			used[id] = true
		}
	}
	for _, retired := range doc.RetiredCharacterIDs {
		id := strings.TrimSpace(retired.CharacterID)
		if id != "" {
			used[id] = true
		}
	}
	nextOrdinal := doc.NextCharacterOrdinal
	if nextOrdinal <= 0 {
		nextOrdinal = 1
	}
	advanceNextStableCharacterOrdinal(novelID, &nextOrdinal, used)
	return &GeneratedCharacterIDAllocator{
		novelID:     novelID,
		nextOrdinal: nextOrdinal,
		usedIDs:     used,
		issuedIDs:   map[string]bool{},
		retiredIDs:  map[string]string{},
	}, nil
}

func (allocator *GeneratedCharacterIDAllocator) Assign(incoming []GeneratedCharacter) []GeneratedCharacter {
	if allocator == nil {
		return incoming
	}
	result := make([]GeneratedCharacter, 0, len(incoming))
	resultIDs := map[string]bool{}
	for _, item := range incoming {
		id := strings.TrimSpace(item.CharacterID)
		if id == "" || resultIDs[id] {
			item.CharacterID = nextStableCharacterID(allocator.novelID, &allocator.nextOrdinal, allocator.usedIDs)
			allocator.issuedIDs[item.CharacterID] = true
		}
		allocator.usedIDs[item.CharacterID] = true
		resultIDs[item.CharacterID] = true
		result = append(result, item)
	}
	return result
}

func (allocator *GeneratedCharacterIDAllocator) Retire(characterID string, mergedInto string) {
	if allocator == nil {
		return
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return
	}
	allocator.usedIDs[characterID] = true
	allocator.retiredIDs[characterID] = strings.TrimSpace(mergedInto)
}

func (allocator *GeneratedCharacterIDAllocator) ApplyState(nextOrdinal int, issuedIDs []string, retiredIDs []GeneratedRetiredCharacterID) {
	if allocator == nil {
		return
	}
	for _, id := range issuedIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		allocator.usedIDs[id] = true
		allocator.issuedIDs[id] = true
	}
	for _, retired := range retiredIDs {
		allocator.Retire(retired.CharacterID, retired.MergedInto)
	}
	if nextOrdinal > allocator.nextOrdinal {
		allocator.nextOrdinal = nextOrdinal
	}
	advanceNextStableCharacterOrdinal(allocator.novelID, &allocator.nextOrdinal, allocator.usedIDs)
}

func (allocator *GeneratedCharacterIDAllocator) IssuedCharacterIDs() []string {
	if allocator == nil {
		return nil
	}
	return sortedStringKeys(allocator.issuedIDs)
}

func (allocator *GeneratedCharacterIDAllocator) RetiredCharacterIDs() []GeneratedRetiredCharacterID {
	if allocator == nil {
		return nil
	}
	result := make([]GeneratedRetiredCharacterID, 0, len(allocator.retiredIDs))
	for id, mergedInto := range allocator.retiredIDs {
		result = append(result, GeneratedRetiredCharacterID{CharacterID: id, MergedInto: mergedInto})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CharacterID < result[j].CharacterID
	})
	return result
}

func (allocator *GeneratedCharacterIDAllocator) NextCharacterOrdinal() int {
	if allocator == nil {
		return 0
	}
	return allocator.nextOrdinal
}

func generatedCharacterIDSet(values []GeneratedCharacter) map[string]bool {
	used := map[string]bool{}
	for _, item := range values {
		id := strings.TrimSpace(item.CharacterID)
		if id != "" {
			used[id] = true
		}
	}
	return used
}

func sortedStringKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value, ok := range values {
		if ok && strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	sort.Strings(result)
	return result
}

func normalizeStringList(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stableCharacterIDForOrdinal(novelID string, ordinal int) string {
	sum := sha1.Sum([]byte(novelID + ":stable:" + strconv.Itoa(ordinal)))
	return "char_" + hex.EncodeToString(sum[:])[:12]
}

func advanceNextStableCharacterOrdinal(novelID string, ordinal *int, used map[string]bool) {
	for {
		current := *ordinal
		if current <= 0 {
			current = 1
		}
		if !used[stableCharacterIDForOrdinal(novelID, current)] {
			*ordinal = current
			return
		}
		*ordinal = current + 1
	}
}

func nextStableCharacterID(novelID string, ordinal *int, used map[string]bool) string {
	for {
		current := *ordinal
		if current <= 0 {
			current = 1
		}
		*ordinal = current + 1
		id := stableCharacterIDForOrdinal(novelID, current)
		if !used[id] {
			return id
		}
	}
}

func createCharacterID(novelID string, canonicalName string) string {
	sum := sha1.Sum([]byte(novelID + ":" + canonicalName))
	return "char_" + hex.EncodeToString(sum[:])[:12]
}
