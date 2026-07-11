-- Creates the dedicated test database for the Go test harness
-- (STRAW_TEST_POSTGRES_DSN). The harness refuses any database whose name
-- does not end in _test, so tests can never truncate the live straw
-- database. This script only runs when the postgres_data volume is first
-- initialized; on an existing volume, create the database manually:
--   docker compose exec postgres createdb -U postgres straw_test
CREATE DATABASE straw_test;
