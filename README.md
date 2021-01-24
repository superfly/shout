# Shout

`shout`ing is an efficient way to communicate between hot air balloons.

## Raison d'être

`shout` was created out of a need of propagating state without consensus. 

## How

Using SQL queries to alter a local SQLite database via user events. Such events are propagated by gossiping (SWIM protocol). More precisely: `shout` embeds `serf`, matches user events to `.sql` files and executes them using the provided payload (JSON).

## Status

Alpha-quality, at best. Documentation might be outdated, inaccurate and/or completely missing.

## Basic usage

**1. Create a migrations**

```bash
# create a migration named "init" using github.com/golang-migrate/migrate
$ migrate create -ext sql -dir ./migrations -seq init
```

```sql
-- ./migrations/000001_init_up.sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
);
```

**2. Create an event handler**

```sql
-- ./handlers/events/users/add.sql
INSERT INTO users(id, name) VALUES (@id, @name);
```

**3. Launch `shout`**

```bash
$ shout run
```

It binds automatically on the serf default RPC port. Use `--help` to see options and override.

**4. Send an event**

We're using `serf` here, for now. Eventually we'll prefer to use `shout` directly as we build in more commands.

```bash
$ serf event users/add '{"id": 12345, "name": "Little Bobby Tables"}'
# Event 'users/add' dispatched!
```

**5. Inspect state**

```bash
$ sqlite3 db.sqlite "SELECT * FROM USERS"
12345|Little Body Tables
```

