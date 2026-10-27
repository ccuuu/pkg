/*
Copyright 2026 The AlaudaDevops Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package runquery is the shared ordering contract for live and archived
// PipelineRuns and TaskRuns. PostgreSQL functions must follow the same rules
// and fixtures; Results does not import this package to materialize columns.
package runquery

import (
	"encoding/json"
	"regexp"
	"time"
)

// RuleVersion identifies the sort contract. A rule change needs a new SQL
// function and index before mixed reads use it.
const RuleVersion = 1

// sortTimePattern accepts RFC3339 timestamps with a Z or numeric offset.
// Named zones and missing offsets are invalid. Fractional seconds are kept
// only long enough to truncate them.
var sortTimePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$`)

// SortKey is the descending order key shared by memory and SQL.
// Missing or invalid times use HasSortTime 0 and Epoch 0. The key never uses
// NULL, and Epoch is truncated to UTC seconds.
type SortKey struct {
	HasSortTime int
	Epoch       int64
	UID         string
}

// Compare reports descending order: -1 when a is newer, 1 when a is older.
func Compare(a, b SortKey) int {
	if a.HasSortTime != b.HasSortTime {
		if a.HasSortTime > b.HasSortTime {
			return -1
		}
		return 1
	}
	if a.Epoch != b.Epoch {
		if a.Epoch > b.Epoch {
			return -1
		}
		return 1
	}
	switch {
	case a.UID > b.UID:
		return -1
	case a.UID < b.UID:
		return 1
	default:
		return 0
	}
}

// Newer reports whether a sorts before b.
func Newer(a, b SortKey) bool {
	return Compare(a, b) < 0
}

// Older reports whether a sorts after b. Keyset pages request older keys.
func Older(a, b SortKey) bool {
	return Compare(a, b) > 0
}

// ParseSortTime accepts one RFC3339 value and truncates it to a UTC second.
func ParseSortTime(raw string) (time.Time, bool) {
	if !sortTimePattern.MatchString(raw) {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC().Truncate(time.Second), true
}

// KeyFromJSON returns the sort key of one Run object. Invalid JSON is reported
// separately so callers can treat the record as bad data. A missing or illegal
// time is a valid key with HasSortTime 0.
func KeyFromJSON(payload []byte) (SortKey, bool) {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return SortKey{}, false
	}
	return KeyFromObject(object), true
}

// KeyFromObject returns the sort key of a decoded Run object.
func KeyFromObject(object map[string]any) SortKey {
	key := SortKey{UID: nestedString(object, "metadata", "uid")}
	raw := nestedString(object, "status", "startTime")
	if raw == "" {
		raw = nestedString(object, "metadata", "creationTimestamp")
	}
	parsed, ok := ParseSortTime(raw)
	if !ok {
		return key
	}
	key.HasSortTime = 1
	key.Epoch = parsed.Unix()
	return key
}

func nestedString(object map[string]any, fields ...string) string {
	var current any = object
	for _, field := range fields {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = next[field]
	}
	value, _ := current.(string)
	return value
}
