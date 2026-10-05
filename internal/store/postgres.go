package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/felipe/rinha/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Record struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Phone     string `json:"phone"`
	BirthDate string `json:"birth_date"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Person struct {
	ID        string   `json:"id,omitempty"`
	Nickname  string   `json:"apelido"`
	Name      string   `json:"nome"`
	BirthDate string   `json:"nascimento"`
	Stack     []string `json:"stack,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

type DB struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, cfg config.Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns
	poolCfg.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := ensureSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	return &DB{pool: pool}, nil
}

func ensureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE TABLE IF NOT EXISTS people (
			email text PRIMARY KEY,
			name text NOT NULL,
			address text NOT NULL,
			phone text NOT NULL,
			birth_date date NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS people_name_idx ON people (name);
		CREATE INDEX IF NOT EXISTS people_updated_at_idx ON people (updated_at DESC);
		CREATE TABLE IF NOT EXISTS pessoas (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			apelido varchar(32) NOT NULL UNIQUE,
			nome varchar(100) NOT NULL,
			nascimento date NOT NULL,
			stack text[] NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS pessoas_nome_idx ON pessoas (nome);
		CREATE INDEX IF NOT EXISTS pessoas_apelido_idx ON pessoas (apelido);
	`)
	return err
}

func (db *DB) CreatePerson(ctx context.Context, person Person) (Person, error) {
	err := db.pool.QueryRow(ctx, `
		INSERT INTO pessoas (apelido, nome, nascimento, stack)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, apelido, nome, nascimento::text, stack, created_at::text`,
		person.Nickname, person.Name, person.BirthDate, person.Stack).
		Scan(&person.ID, &person.Nickname, &person.Name, &person.BirthDate, &person.Stack, &person.CreatedAt)
	return person, err
}

func (db *DB) GetPerson(ctx context.Context, id string) (Person, error) {
	var person Person
	err := db.pool.QueryRow(ctx, `
		SELECT id::text, apelido, nome, nascimento::text, stack, created_at::text
		FROM pessoas WHERE id = $1::uuid`, id).
		Scan(&person.ID, &person.Nickname, &person.Name, &person.BirthDate, &person.Stack, &person.CreatedAt)
	return person, err
}

func (db *DB) SearchPeople(ctx context.Context, term string) ([]Person, error) {
	term = strings.TrimSpace(term)
	pattern := "%" + term + "%"
	rows, err := db.pool.Query(ctx, `
		SELECT p.id::text, p.apelido, p.nome, p.nascimento::text, p.stack, p.created_at::text
		FROM pessoas AS p
		WHERE p.nome ILIKE $1
		   OR p.apelido ILIKE $1
		   OR EXISTS (
			SELECT 1
			FROM unnest(COALESCE(p.stack, ARRAY[]::text[])) AS technology(value)
			WHERE technology.value ILIKE $1
		   )
		ORDER BY p.id`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	people := make([]Person, 0)
	for rows.Next() {
		var person Person
		if err := rows.Scan(&person.ID, &person.Nickname, &person.Name, &person.BirthDate, &person.Stack, &person.CreatedAt); err != nil {
			return nil, err
		}
		people = append(people, person)
	}
	return people, rows.Err()
}

func (db *DB) Close() { db.pool.Close() }

func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

func (db *DB) BulkUpsert(ctx context.Context, records []Record) error {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE people_stage (name text, address text, phone text, birth_date date, email text) ON COMMIT DROP`); err != nil {
		return err
	}
	rows := make([][]any, len(records))
	for i, record := range records {
		rows[i] = []any{record.Name, record.Address, record.Phone, record.BirthDate, record.Email}
	}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"people_stage"}, []string{"name", "address", "phone", "birth_date", "email"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("copy batch: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO people (name, address, phone, birth_date, email)
		SELECT name, address, phone, birth_date, email FROM people_stage
		ON CONFLICT (email) DO UPDATE SET
			name = EXCLUDED.name,
			address = EXCLUDED.address,
			phone = EXCLUDED.phone,
			birth_date = EXCLUDED.birth_date,
			updated_at = now()`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (db *DB) Get(ctx context.Context, email string) (Record, error) {
	var r Record
	err := db.pool.QueryRow(ctx, `SELECT name, address, phone, birth_date::text, email, created_at::text, updated_at::text FROM people WHERE email = $1`, email).
		Scan(&r.Name, &r.Address, &r.Phone, &r.BirthDate, &r.Email, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (db *DB) List(ctx context.Context, prefix string, limit, offset int) ([]Record, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT name, address, phone, birth_date::text, email, created_at::text, updated_at::text
		FROM people
		WHERE name ILIKE $1
		ORDER BY name, email
		LIMIT $2 OFFSET $3`, prefix+"%", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Record, 0, limit)
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Name, &r.Address, &r.Phone, &r.BirthDate, &r.Email, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
