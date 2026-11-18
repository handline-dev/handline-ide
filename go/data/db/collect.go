package db

import "database/sql"

// Collect is every row of a query, each read by scan: Collect(q.Query(...))(scan). The rows are closed.
func Collect[T any](rows *sql.Rows, err error) func(scan func(*sql.Rows) (T, error)) ([]T, error) {
	return func(scan func(*sql.Rows) (T, error)) ([]T, error) {
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []T
		for rows.Next() {
			v, err := scan(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, rows.Err()
	}
}
