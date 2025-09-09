package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/BurntSushi/toml"
)

type Config struct {
	Port    int    `toml:"port"`
	DBPath  string `toml:"db_path"`
	LogPath string `toml:"log_path"`
}

type Comment struct {
	ID       int       `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Text     string    `json:"text"`
	IP       string    `json:"ip"`
	Location string    `json:"location"`
	Created  time.Time `json:"created"`
}

var db *sql.DB
var logFile *os.File
var config Config

func main() {
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		log.Fatal("Error loading config.toml:", err)
	}

	var err error
	logFile, err = os.OpenFile(config.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Error opening log file:", err)
	}

	defer logFile.Close()

	db, err = sql.Open("sqlite3", config.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			email TEXT,
			text TEXT,
			ip TEXT,
			location TEXT,
			created DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/comments", commentsHandler)
	http.HandleFunc("/all", allCommentsHandler)

	fmt.Printf("Guestbook started :)")
	log.Fatal(http.ListenAndServe(addr, nil))
}

// --- Handlers ---
func commentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		getComments(w, r, 15)
	} else if r.Method == http.MethodPost {
		addComment(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func allCommentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		getComments(w, r, -1)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// limit = N, or -1 is all brawtherrr
func getComments(w http.ResponseWriter, r *http.Request, limit int) {
	query := `
		SELECT id, name, email, text, ip, location, created
		FROM comments
		ORDER BY created DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		var created string
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Text, &c.IP, &c.Location, &created); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		c.Created, _ = time.Parse("2006-01-02 15:04:05", created)
		comments = append(comments, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

func addComment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", 400)
		return
	}
	name := r.FormValue("name")
	email := r.FormValue("email")
	text := r.FormValue("comment")

	if name == "" || email == "" || text == "" {
		http.Error(w, "All fields (name, email, comment) are required", 400)
		return
	}

	ip := getIP(r)
	location := getLocation(ip)

	_, err := db.Exec(
		"INSERT INTO comments (name, email, text, ip, location) VALUES (?, ?, ?, ?, ?)",
		name, email, text, ip, location,
	)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	logRequest(ip, location, fmt.Sprintf("name=%s email=%s comment=%s", name, email, text))

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintln(w, "Comment added successfully")
}

func getIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	if strings.Contains(ip, ":") {
		host, _, err := net.SplitHostPort(ip)
		if err == nil {
			return host
		}
	}
	return ip
}

func getLocation(ip string) string {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return "Localhost"
	}
	return "Unknown Location"
}

func logRequest(ip, location, data string) {
	entry := fmt.Sprintf("[%s] [%s] [%s] [%s]\n",
		ip, time.Now().Format(time.RFC3339), location, data)
	io.WriteString(logFile, entry)
}
