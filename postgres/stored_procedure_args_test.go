package database

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type recordingQuerier struct {
	rows pgx.Rows
	sql  string
	args []any
}

func (q *recordingQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected QueryRow call")
}

func (q *recordingQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.sql, q.args = sql, args
	return q.rows, nil
}

func scalarRows(values ...int) *fakeRows {
	rows := &fakeRows{fieldDescriptions: []pgconn.FieldDescription{{Name: "v"}}}
	for _, v := range values {
		rows.values = append(rows.values, []any{v})
	}
	return rows
}

// TestStoredProcedureJSONArgumentOrder locks in the documented placement of
// the JSON argument: after the positional arguments, as $N+1 (C4-009).
func TestStoredProcedureJSONArgumentOrder(t *testing.T) {
	t.Parallel()

	const query = "SELECT * FROM save_items($1, $2, $3::jsonb)"
	jsonArg := map[string]int{"a": 1}
	want := []any{"tenant", 7, `{"a":1}`}

	t.Run("QueryJSON", func(t *testing.T) {
		t.Parallel()
		querier := &recordingQuerier{rows: scalarRows(1)}
		if _, err := (StoredProcedure[int]{Pool: querier}).QueryJSON(t.Context(), query, jsonArg, "tenant", 7); err != nil {
			t.Fatalf("QueryJSON: %v", err)
		}
		if querier.sql != query || !reflect.DeepEqual(querier.args, want) {
			t.Fatalf("Query(%q, %#v), want (%q, %#v)", querier.sql, querier.args, query, want)
		}
	})

	t.Run("QueryRowJSON", func(t *testing.T) {
		t.Parallel()
		querier := &recordingQuerier{rows: scalarRows(1)}
		if _, err := (StoredProcedure[int]{Pool: querier}).QueryRowJSON(t.Context(), query, jsonArg, "tenant", 7); err != nil {
			t.Fatalf("QueryRowJSON: %v", err)
		}
		if querier.sql != query || !reflect.DeepEqual(querier.args, want) {
			t.Fatalf("Query(%q, %#v), want (%q, %#v)", querier.sql, querier.args, query, want)
		}
	})
}

// TestStoredProcedureSuccessClosesRows asserts that rows are closed on the
// success paths too (C4-009: fakeRows.closed was set but never asserted).
func TestStoredProcedureSuccessClosesRows(t *testing.T) {
	t.Parallel()

	t.Run("Query", func(t *testing.T) {
		t.Parallel()
		rows := scalarRows(1, 2, 3)
		// nosemgrep: defer-rows-close -- StoredProcedure.Query returns []T, not rows.
		got, err := (StoredProcedure[int]{Pool: fakeQuerier{rows: rows}}).Query(t.Context(), "SELECT * FROM list_values()")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Fatalf("Query = %v, want [1 2 3]", got)
		}
		if !rows.closed {
			t.Fatal("rows were not closed")
		}
	})

	t.Run("QueryRow", func(t *testing.T) {
		t.Parallel()
		rows := scalarRows(42)
		got, err := (StoredProcedure[int]{Pool: fakeQuerier{rows: rows}}).QueryRow(t.Context(), "SELECT * FROM get_value()")
		if err != nil {
			t.Fatalf("QueryRow: %v", err)
		}
		if got != 42 {
			t.Fatalf("QueryRow = %d, want 42", got)
		}
		if !rows.closed {
			t.Fatal("rows were not closed")
		}
	})
}

func TestProcSpanName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sql  string
		want string
	}{
		{"SELECT * FROM check_or_create_job($1, $2)", "postgres.check_or_create_job"},
		{"select * from Get_Job ($1)", "postgres.Get_Job"},
		{"SELECT *\n  FROM\tlist_jobs()", "postgres.list_jobs"},
		{"SELECT 1", "postgres.query"},
		{"SELECT * FROM jobs WHERE id = $1", "postgres.query"},
		{"", "postgres.query"},
	}
	for _, tt := range tests {
		if got := procSpanName(tt.sql); got != tt.want {
			t.Errorf("procSpanName(%q) = %q, want %q", tt.sql, got, tt.want)
		}
	}
}
