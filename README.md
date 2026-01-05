# Blog Aggregator (Gator)

A command-line RSS feed aggregator written in Go.  
Gator periodically fetches RSS feeds, stores posts in PostgreSQL, and lets users browse posts from the feeds they follow.

This project was built as part of a backend-focused learning exercise and emphasizes:

- SQL schema design and migrations
- Safe database access with `sqlc`
- Background workers with Go
- Real-world RSS parsing quirks

---

## Features

- User accounts (register / login)
- Add RSS feeds
- Follow and unfollow feeds
- Periodically scrape feeds on a timer
- Store posts in PostgreSQL
- Ignore duplicate posts automatically
- Browse recent posts from followed feeds

---

## Requirements

To run Gator locally, you will need:

- Go 1.21 or newer
- PostgreSQL
- `goose` for database migrations
- `sqlc` for generating type-safe database queries

Install the required CLI tools:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
````

---

## Setup

### Database

Create a PostgreSQL database for the application:

```sql
CREATE DATABASE gator;
```

---

### Configuration

Gator uses a local configuration file to store the database connection string and the currently logged-in user.

Create a file named `.gatorconfig.json` in the project root:

```json
{
  "db_url": "postgres://postgres:postgres@localhost:5432/gator?sslmode=disable",
  "current_user_name": ""
}
```

* `db_url` must point to your PostgreSQL database
* `current_user_name` is managed automatically by the CLI
  **Do not edit this field manually**

---

### Database Migrations

Run all database migrations to create the required tables:

```bash
goose -dir sql/schema postgres "postgres://postgres:postgres@localhost:5432/gator?sslmode=disable" up
```

You can verify that tables were created successfully:

```bash
psql "postgres://postgres:postgres@localhost:5432/gator?sslmode=disable" -c "\dt"
```

---

### Generate Database Code

Gator uses `sqlc` to generate type-safe Go code from SQL queries.

Run:

```bash
sqlc generate
```

This will populate `internal/database/` with generated query code.

---

## Usage

### Register a User

Create a new user account:

```bash
go run . register bob
```

This also sets the current user automatically.

---

### Log In

Switch to an existing user:

```bash
go run . login bob
```

---

### Add a Feed

Add an RSS feed to the system:

```bash
go run . addfeed wagslane https://www.wagslane.dev/index.xml
```

The user who adds a feed automatically follows it.

---

### Follow and Unfollow Feeds

Follow an existing feed:

```bash
go run . follow https://hnrss.org/newest
```

Unfollow a feed:

```bash
go run . unfollow https://hnrss.org/newest
```

---

### Run the Aggregator

Start the background feed scraper:

```bash
go run . agg 10s
```

The aggregator will:

* Wake up every 10 seconds
* Fetch the next feed
* Parse new posts
* Store them in the database

Leave this running in a terminal while browsing posts.

---

### Browse Posts

View recent posts from followed feeds:

```bash
go run . browse
```

By default, this shows the 2 most recent posts.

Specify a custom limit:

```bash
go run . browse 10
```

---

## Project Structure

```
.
├── main.go                # CLI commands and aggregator loop
├── rss.go                 # RSS fetching and parsing
├── internal/
│   ├── config/            # Config file handling
│   └── database/          # sqlc-generated queries
├── sql/
│   ├── schema/            # goose migrations
│   └── queries/           # sqlc query definitions
├── sqlc.yaml
└── .gatorconfig.json
```

---

## Notes

* RSS feeds use inconsistent date formats; Gator attempts multiple layouts when parsing publication times
* Duplicate posts are ignored using a unique constraint on post URLs
* The aggregator is resilient: one failing feed will not stop the process

---

## Future Improvements ?

* Mark posts as read
* Per-user feed fetch frequency
* Improved HTML cleanup for descriptions
* Terminal UI or web interface

---
