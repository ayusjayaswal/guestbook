package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestMain(m *testing.M) {
	// Setup test database in memory
	var err error
	db, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}

	// Create table
	_, err = db.Exec(`
		CREATE TABLE comments (
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
		panic(err)
	}

	// Setup temp log file
	logFile, err = ioutil.TempFile("", "test_log")
	if err != nil {
		panic(err)
	}
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	os.Exit(m.Run())
}

func TestGetIP(t *testing.T) {
	tests := []struct {
		name          string
		xForwardedFor string
		remoteAddr    string
		expected      string
	}{
		{
			name:          "No X-Forwarded-For, simple IP",
			xForwardedFor: "",
			remoteAddr:    "192.168.1.1",
			expected:      "192.168.1.1",
		},
		{
			name:          "X-Forwarded-For present",
			xForwardedFor: "203.0.113.1",
			remoteAddr:    "127.0.0.1",
			expected:      "203.0.113.1",
		},
		{
			name:          "IP with port",
			xForwardedFor: "",
			remoteAddr:    "192.168.1.1:12345",
			expected:      "192.168.1.1",
		},
		{
			name:          "IPv6 with port",
			xForwardedFor: "",
			remoteAddr:    "[::1]:8080",
			expected:      "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			req.RemoteAddr = tt.remoteAddr

			ip := getIP(req)
			if ip != tt.expected {
				t.Errorf("getIP() = %v, want %v", ip, tt.expected)
			}
		})
	}
}

func TestGetLocation(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{
			name:     "Empty IP",
			ip:       "",
			expected: "Localhost",
		},
		{
			name:     "Localhost IPv4",
			ip:       "127.0.0.1",
			expected: "Localhost",
		},
		{
			name:     "Localhost IPv6",
			ip:       "::1",
			expected: "Localhost",
		},
		{
			name:     "External IP",
			ip:       "8.8.8.8",
			expected: "Unknown Location",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location := getLocation(tt.ip)
			if location != tt.expected {
				t.Errorf("getLocation(%v) = %v, want %v", tt.ip, location, tt.expected)
			}
		})
	}
}

func TestLogRequest(t *testing.T) {
	// Clear the log file
	logFile.Truncate(0)
	logFile.Seek(0, 0)

	ip := "192.168.1.1"
	location := "Test Location"
	data := "test data"

	logRequest(ip, location, data)

	// Read the log file
	logFile.Seek(0, 0)
	content, err := io.ReadAll(logFile)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}

	line := lines[0]
	expectedParts := []string{ip, location, data}
	for _, part := range expectedParts {
		if !strings.Contains(line, part) {
			t.Errorf("Log line does not contain %q: %q", part, line)
		}
	}
}

func TestAddComment(t *testing.T) {
	// Clear table
	_, err := db.Exec("DELETE FROM comments")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		formData       string
		expectedStatus int
		expectInsert   bool
	}{
		{
			name:           "Valid comment",
			formData:       "name=John&email=john@example.com&comment=Hello world",
			expectedStatus: 201,
			expectInsert:   true,
		},
		{
			name:           "Missing name",
			formData:       "email=john@example.com&comment=Hello",
			expectedStatus: 400,
			expectInsert:   false,
		},
		{
			name:           "Missing email",
			formData:       "name=John&comment=Hello",
			expectedStatus: 400,
			expectInsert:   false,
		},
		{
			name:           "Missing comment",
			formData:       "name=John&email=john@example.com",
			expectedStatus: 400,
			expectInsert:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.formData))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()

			addComment(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, recorder.Code)
			}

			if tt.expectInsert {
				var count int
				err := db.QueryRow("SELECT COUNT(*) FROM comments").Scan(&count)
				if err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Errorf("Expected 1 comment inserted, got %d", count)
				}
			}
		})
	}
}

func TestGetComments(t *testing.T) {
	// Clear table
	_, err := db.Exec("DELETE FROM comments")
	if err != nil {
		t.Fatal(err)
	}

	// Insert test comments
	testComments := []struct {
		name  string
		email string
		text  string
		ip    string
	}{
		{"Alice", "alice@example.com", "First comment", "1.2.3.4"},
		{"Bob", "bob@example.com", "Second comment", "5.6.7.8"},
		{"Charlie", "charlie@example.com", "Third comment", "9.10.11.12"},
	}

	for _, c := range testComments {
		_, err := db.Exec("INSERT INTO comments (name, email, text, ip, location) VALUES (?, ?, ?, ?, ?)",
			c.name, c.email, c.text, c.ip, "Test Location")
		if err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		limit    int
		expected int // number of comments expected
	}{
		{
			name:     "Limit 15",
			limit:    15,
			expected: 3,
		},
		{
			name:     "Limit 2",
			limit:    2,
			expected: 2,
		},
		{
			name:     "No limit",
			limit:    -1,
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			recorder := httptest.NewRecorder()

			getComments(recorder, req, tt.limit)

			if recorder.Code != 200 {
				t.Errorf("Expected status 200, got %d", recorder.Code)
			}

			var comments []Comment
			err := json.NewDecoder(recorder.Body).Decode(&comments)
			if err != nil {
				t.Fatal(err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
			}

			// Check order (DESC by created)
			if len(comments) > 1 {
				if comments[0].Created.Before(comments[1].Created) {
					t.Error("Comments not in descending order")
				}
			}
		})
	}
}

func TestCommentsHandler(t *testing.T) {
	// Clear table
	_, err := db.Exec("DELETE FROM comments")
	if err != nil {
		t.Fatal(err)
	}

	// Insert a test comment
	_, err = db.Exec("INSERT INTO comments (name, email, text, ip, location) VALUES (?, ?, ?, ?, ?)",
		"Test", "test@example.com", "Test comment", "127.0.0.1", "Localhost")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		method   string
		body     string
		expected int
	}{
		{
			name:     "GET request",
			method:   "GET",
			body:     "",
			expected: 200,
		},
		{
			name:     "POST valid comment",
			method:   "POST",
			body:     "name=Jane&email=jane@example.com&comment=Another comment",
			expected: 201,
		},
		{
			name:     "POST invalid method",
			method:   "PUT",
			body:     "",
			expected: 405,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.method == "POST" {
				req = httptest.NewRequest(tt.method, "/", strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req = httptest.NewRequest(tt.method, "/", nil)
			}
			recorder := httptest.NewRecorder()

			commentsHandler(recorder, req)

			if recorder.Code != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, recorder.Code)
			}
		})
	}
}

func TestAllCommentsHandler(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		expected int
	}{
		{
			name:     "GET request",
			method:   "GET",
			expected: 200,
		},
		{
			name:     "POST request",
			method:   "POST",
			expected: 405,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			recorder := httptest.NewRecorder()

			allCommentsHandler(recorder, req)

			if recorder.Code != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, recorder.Code)
			}
		})
	}
}
