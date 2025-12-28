# Guestbook

A simple Go-based guestbook web service that allows users to post comments and retrieve them.

## Features

- RESTful API for managing comments
- SQLite database for persistence
- IP-based logging of requests
- Configurable via TOML file

## Installation

1. Ensure you have Go 1.24.6 or later installed.
2. Clone or download the project.
3. Install dependencies: `go mod tidy`

## Usage

1. Configure the service in `config.toml` (see Configuration section).
2. Run the application: `go run main.go`
3. The server will start on the configured port.

## API Endpoints

- `GET /comments` - Retrieve the last 15 comments
- `POST /comments` - Add a new comment (form data: name, email, comment)
- `GET /all` - Retrieve all comments

### POST Comment

Send a POST request to `/comments` with form data:
- `name`: User's name
- `email`: User's email
- `comment`: Comment text

## Configuration

Edit `config.toml`:
- `port`: Server port (default: 9001)
- `db_path`: SQLite database file path (default: "./guestbook.db")
- `log_path`: Log file path (default: "./guestbook.log")

## Dependencies

- [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3): SQLite driver
- [github.com/BurntSushi/toml](https://github.com/BurntSushi/toml): TOML parser

## License

See LICENSE file for details.