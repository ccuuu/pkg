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

// FunctionsSQL is the immutable PostgreSQL contract. Results copies this text
// into its patch and does not import this package to write columns. Both
// functions accept any input, never raise, and never return NULL.
const FunctionsSQL = `
CREATE OR REPLACE FUNCTION tekton_run_sort_epoch(data jsonb)
RETURNS bigint
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
AS $tekton_run_sort_epoch$
DECLARE
  raw text;
BEGIN
  IF data IS NULL THEN
    RETURN 0;
  END IF;
  BEGIN
    raw := data->'status'->>'startTime';
    IF raw IS NULL OR raw = '' THEN
      raw := data->'metadata'->>'creationTimestamp';
    END IF;
    IF raw IS NULL OR raw = '' THEN
      RETURN 0;
    END IF;
    IF raw !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' THEN
      RETURN 0;
    END IF;
    -- timestamptz rounds fractional seconds; Go truncates to the UTC second.
    raw := regexp_replace(raw, '\.[0-9]+', '');
    RETURN trunc(extract(epoch FROM raw::timestamptz))::bigint;
  EXCEPTION
    WHEN OTHERS THEN
      RETURN 0;
  END;
END;
$tekton_run_sort_epoch$;

CREATE OR REPLACE FUNCTION tekton_run_has_sort_time(data jsonb)
RETURNS integer
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
AS $tekton_run_has_sort_time$
DECLARE
  raw text;
BEGIN
  IF data IS NULL THEN
    RETURN 0;
  END IF;
  BEGIN
    raw := data->'status'->>'startTime';
    IF raw IS NULL OR raw = '' THEN
      raw := data->'metadata'->>'creationTimestamp';
    END IF;
    IF raw IS NULL OR raw = '' THEN
      RETURN 0;
    END IF;
    IF raw !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' THEN
      RETURN 0;
    END IF;
    raw := regexp_replace(raw, '\.[0-9]+', '');
    PERFORM raw::timestamptz;
    RETURN 1;
  EXCEPTION
    WHEN OTHERS THEN
      RETURN 0;
  END;
END;
$tekton_run_has_sort_time$;
`
