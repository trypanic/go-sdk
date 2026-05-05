package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trypanic/go-sdk/errorkit"
)

func TestStoredProcedureQueryRowJSON_MarshalFailure_ReturnsPostgresError(t *testing.T) {
	sut := StoredProcedure[map[string]any]{}

	_, err := sut.QueryRowJSON(context.Background(), "SELECT 1", make(chan int))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}

	if appErr.Code() != ERR_DB_POSTGRES_ERROR {
		t.Fatalf("expected code %s, got %s", ERR_DB_POSTGRES_ERROR, appErr.Code())
	}
}

func TestStoredProcedureQuery_ScalarRows_ReturnsValues(t *testing.T) {
	t.Parallel()

	rows := &fakeRows{
		fieldDescriptions: []pgconn.FieldDescription{{Name: "count"}},
		values: [][]any{
			{1},
			{2},
		},
	}
	sut := StoredProcedure[int]{Pool: fakeQuerier{rows: rows}}

	got, err := sut.Query(context.Background(), "SELECT * FROM count_values()")
	if err != nil {
		t.Fatalf("Query(count_values) error: %v", err)
	}

	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Query(count_values) = %#v, want %#v", got, want)
	}
}

func TestStoredProcedureQueryRow_ScalarRow_ReturnsUUID(t *testing.T) {
	t.Parallel()

	want := uuid.New()
	rows := &fakeRows{
		fieldDescriptions: []pgconn.FieldDescription{{Name: "batch_id"}},
		values:            [][]any{{want}},
	}
	sut := StoredProcedure[uuid.UUID]{Pool: fakeQuerier{rows: rows}}

	got, err := sut.QueryRow(context.Background(), "SELECT * FROM create_batch()")
	if err != nil {
		t.Fatalf("QueryRow(create_batch) error: %v", err)
	}
	if got != want {
		t.Fatalf("QueryRow(create_batch) = %v, want %v", got, want)
	}
}

func TestStoredProcedureQueryRow_StructRow_ReturnsNamedFields(t *testing.T) {
	t.Parallel()

	type jobRow struct {
		JobID uuid.UUID `db:"job_id"`
		State string    `db:"state"`
	}

	want := jobRow{
		JobID: uuid.New(),
		State: "pending",
	}
	rows := &fakeRows{
		fieldDescriptions: []pgconn.FieldDescription{
			{Name: "job_id"},
			{Name: "state"},
		},
		values: [][]any{{want.JobID, want.State}},
	}
	sut := StoredProcedure[jobRow]{Pool: fakeQuerier{rows: rows}}

	got, err := sut.QueryRow(context.Background(), "SELECT * FROM get_job()")
	if err != nil {
		t.Fatalf("QueryRow(get_job) error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRow(get_job) = %#v, want %#v", got, want)
	}
}

type fakeQuerier struct {
	rows pgx.Rows
	err  error
}

func (f fakeQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	panic("unexpected QueryRow call")
}

func (f fakeQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return f.rows, f.err
}

type fakeRows struct {
	fieldDescriptions []pgconn.FieldDescription
	values            [][]any
	err               error
	index             int
	closed            bool
}

func (r *fakeRows) Close() {
	r.closed = true
}

func (r *fakeRows) Err() error {
	return r.err
}

func (r *fakeRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	return r.fieldDescriptions
}

func (r *fakeRows) Next() bool {
	if r.err != nil {
		r.closed = true
		return false
	}
	if r.index >= len(r.values) {
		r.closed = true
		return false
	}
	r.index++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	row, err := r.currentRow()
	if err != nil {
		return err
	}
	if len(dest) != len(row) {
		return fmt.Errorf("scan destination count = %d, want %d", len(dest), len(row))
	}

	for i, target := range dest {
		if target == nil {
			continue
		}

		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
			return fmt.Errorf("scan destination %d is not a non-nil pointer", i)
		}

		sourceValue := reflect.ValueOf(row[i])
		targetElem := targetValue.Elem()
		if !sourceValue.IsValid() {
			targetElem.Set(reflect.Zero(targetElem.Type()))
			continue
		}

		if sourceValue.Type().AssignableTo(targetElem.Type()) {
			targetElem.Set(sourceValue)
			continue
		}
		if sourceValue.Type().ConvertibleTo(targetElem.Type()) {
			targetElem.Set(sourceValue.Convert(targetElem.Type()))
			continue
		}

		return fmt.Errorf("cannot assign %T to %s", row[i], targetElem.Type())
	}

	return nil
}

func (r *fakeRows) Values() ([]any, error) {
	row, err := r.currentRow()
	if err != nil {
		return nil, err
	}

	values := make([]any, len(row))
	copy(values, row)
	return values, nil
}

func (r *fakeRows) RawValues() [][]byte {
	if r.index == 0 || r.index > len(r.values) {
		return nil
	}
	return make([][]byte, len(r.values[r.index-1]))
}

func (r *fakeRows) Conn() *pgx.Conn {
	return nil
}

func (r *fakeRows) currentRow() ([]any, error) {
	if r.index == 0 || r.index > len(r.values) {
		return nil, fmt.Errorf("row is not positioned")
	}
	return r.values[r.index-1], nil
}
