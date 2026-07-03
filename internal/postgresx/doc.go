// Package postgresx provides a Postgres connection pool helper and
// migration application for the Straw control service.
//
// It is the single point where github.com/jackc/pgx/v5 is used.
// All other packages interact with Postgres through interfaces defined
// in internal/control.
package postgresx
