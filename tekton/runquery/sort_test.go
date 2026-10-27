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

package runquery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestKeyFromObjectFixtures(t *testing.T) {
	cases := []struct {
		name   string
		object map[string]any
		has    int
		epoch  int64
		uid    string
	}{
		{
			name: "pipeline run start time wins over creation time",
			object: map[string]any{
				"kind": "PipelineRun",
				"metadata": map[string]any{
					"uid":               "uid-b",
					"creationTimestamp": "2026-10-27T00:00:00Z",
				},
				"status": map[string]any{"startTime": "2026-10-27T06:00:00.900Z"},
			},
			has:   1,
			epoch: time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC).Unix(),
			uid:   "uid-b",
		},
		{
			name: "task run numeric offset truncates to utc seconds",
			object: map[string]any{
				"apiVersion": "tekton.dev/v1beta1",
				"kind":       "TaskRun",
				"metadata": map[string]any{
					"uid":               "uid-a",
					"creationTimestamp": "2026-10-27T14:00:00.100+08:00",
				},
			},
			has:   1,
			epoch: time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC).Unix(),
			uid:   "uid-a",
		},
		{
			name: "missing time is zero and not null",
			object: map[string]any{
				"kind":     "PipelineRun",
				"metadata": map[string]any{"uid": "uid-missing"},
			},
			uid: "uid-missing",
		},
		{
			name: "named zone is invalid",
			object: map[string]any{
				"kind": "TaskRun",
				"metadata": map[string]any{
					"uid":               "uid-zone",
					"creationTimestamp": "2026-10-27T06:00:00 America/Shanghai",
				},
			},
			uid: "uid-zone",
		},
		{
			name: "illegal timestamp is invalid",
			object: map[string]any{
				"kind": "PipelineRun",
				"status": map[string]any{
					"startTime": "not-a-time",
				},
				"metadata": map[string]any{"uid": "uid-bad"},
			},
			uid: "uid-bad",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := KeyFromObject(tc.object)
			if key.HasSortTime != tc.has || key.Epoch != tc.epoch || key.UID != tc.uid {
				t.Fatalf("key = %#v, want has=%d epoch=%d uid=%s", key, tc.has, tc.epoch, tc.uid)
			}
			payload, err := json.Marshal(tc.object)
			if err != nil {
				t.Fatal(err)
			}
			decoded, ok := KeyFromJSON(payload)
			if !ok || decoded != key {
				t.Fatalf("json key = %#v ok=%v", decoded, ok)
			}
		})
	}
}

func TestCompareOrdersHasTimeEpochAndUID(t *testing.T) {
	timed := SortKey{HasSortTime: 1, Epoch: 100, UID: "a"}
	missing := SortKey{UID: "z"}
	if !Newer(timed, missing) {
		t.Fatal("a valid time must sort before a missing time")
	}
	later := SortKey{HasSortTime: 1, Epoch: 200, UID: "a"}
	if !Newer(later, timed) {
		t.Fatal("later epoch must sort first")
	}
	higherUID := SortKey{HasSortTime: 1, Epoch: 100, UID: "b"}
	if !Newer(higherUID, timed) {
		t.Fatal("same second must sort by UID descending")
	}
	if Older(timed, higherUID) != true || Compare(timed, timed) != 0 {
		t.Fatal("older and equality do not match compare")
	}
}

func TestFunctionsNeverReturnNull(t *testing.T) {
	for _, name := range []string{"tekton_run_has_sort_time", "tekton_run_sort_epoch"} {
		if !strings.Contains(FunctionsSQL, "FUNCTION "+name) {
			t.Fatalf("missing function %s", name)
		}
	}
	if strings.Contains(FunctionsSQL, "RETURNS NULL") || !strings.Contains(FunctionsSQL, "EXCEPTION") {
		t.Fatal("SQL functions must catch errors and must not declare a NULL result")
	}
	if strings.Count(FunctionsSQL, "RETURN 0;") < 6 {
		t.Fatal("both functions need an explicit zero for null, empty, invalid, and exception paths")
	}
}

func TestInvalidJSONIsNotASortKey(t *testing.T) {
	if _, ok := KeyFromJSON([]byte("{")); ok {
		t.Fatal("invalid JSON must not look like a successful key")
	}
}

func TestSQLTruncatesFractionAndRejectsPadding(t *testing.T) {
	if strings.Contains(FunctionsSQL, "btrim(") {
		t.Fatal("SQL must not accept surrounding whitespace that Go rejects")
	}
	if strings.Count(FunctionsSQL, "regexp_replace(raw, '\\.[0-9]+', '')") != 2 {
		t.Fatal("both SQL functions must drop fractional seconds before casting")
	}
	parsed, ok := ParseSortTime("2024-01-02T03:04:05.999999999Z")
	if !ok || parsed.Unix() != 1704164645 {
		t.Fatalf("go truncation = %s ok=%v unix=%d", parsed, ok, parsed.Unix())
	}
	if _, ok := ParseSortTime(" 2024-01-02T03:04:05Z"); ok {
		t.Fatal("padded timestamp must be missing")
	}
}
