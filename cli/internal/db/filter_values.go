package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type filterValueTable struct {
	name    string
	harness string
}

var filterValueTables = []filterValueTable{
	{name: TableTokenEvents, harness: "oc"},
	{name: TableTpsSamples, harness: "oc"},
	{name: TableLLMRequests, harness: "oc"},
	{name: TableToolCalls, harness: "oc"},
	{name: TablePiTokenEvents, harness: "pi"},
	{name: TablePiTpsSamples, harness: "pi"},
	{name: TablePiLLMRequests, harness: "pi"},
	{name: TablePiToolCalls, harness: "pi"},
}

// AvailableProviders returns sorted provider values across all table families for the current filter context.
func AvailableProviders(ctx context.Context, db *sql.DB, f Filter) ([]string, error) {
	providerFilter := f
	providerFilter.Providers = nil

	values := make(map[string]bool)
	for _, table := range filterValueTables {
		if !harnessAllowed(providerFilter, table.harness) {
			continue
		}
		exists, err := tableExists(ctx, db, table.name)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if err := queryDistinctProviders(ctx, db, providerFilter, table.name, values); err != nil {
			return nil, err
		}
	}
	return sortedKeys(values), nil
}

func queryDistinctProviders(ctx context.Context, db *sql.DB, f Filter, table string, values map[string]bool) error {
	whereClause, args := buildWhereClause(f)
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s %s`, ColProvider, table, whereClause)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return err
		}
		if value != "" {
			values[value] = true
		}
	}
	return rows.Err()
}

// AvailableHarnesses returns sorted harness values that have rows for the current filter context.
func AvailableHarnesses(ctx context.Context, db *sql.DB, f Filter) ([]string, error) {
	harnessFilter := f
	harnessFilter.Harnesses = nil

	values := make(map[string]bool)
	for _, table := range filterValueTables {
		exists, err := tableExists(ctx, db, table.name)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		matched, err := tableHasMatchingRow(ctx, db, harnessFilter, table.name)
		if err != nil {
			return nil, err
		}
		if matched {
			values[table.harness] = true
		}
	}
	return sortedKeys(values), nil
}

func tableHasMatchingRow(ctx context.Context, db *sql.DB, f Filter, table string) (bool, error) {
	whereClause, args := buildWhereClause(f)
	query := fmt.Sprintf(`SELECT 1 FROM %s %s LIMIT 1`, table, whereClause)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
