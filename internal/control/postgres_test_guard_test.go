package control

import "testing"

// The guard needs no live database: it must reject any DSN whose database
// name is not explicitly test-designated, regardless of DSN syntax.
func TestCheckTestDatabaseDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{
			name:    "compose live database rejected",
			dsn:     "postgres://postgres@localhost:5432/straw?sslmode=disable",
			wantErr: true,
		},
		{
			name:    "test database accepted",
			dsn:     "postgres://postgres@localhost:5432/straw_test?sslmode=disable",
			wantErr: false,
		},
		{
			name:    "keyword DSN live database rejected",
			dsn:     "host=localhost user=postgres dbname=straw sslmode=disable",
			wantErr: true,
		},
		{
			name:    "keyword DSN test database accepted",
			dsn:     "host=localhost user=postgres dbname=straw_test sslmode=disable",
			wantErr: false,
		},
		{
			name: "missing database name rejected",
			// No dbname: pgx defaults the database to the user name, which
			// is never a designated test database.
			dsn:     "postgres://postgres@localhost:5432/?sslmode=disable",
			wantErr: true,
		},
		{
			name:    "unparseable DSN rejected",
			dsn:     "://not-a-dsn",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTestDatabaseDSN(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkTestDatabaseDSN(%q) error = %v, wantErr %v", tt.dsn, err, tt.wantErr)
			}
		})
	}
}
