// testserver is a simple HTTP server for verifying curlx functionality.
// It implements the API described in openapi.yaml.
//
// Usage:
//
//	go run ./testserver
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const validToken = "test-token-abc123"

// --- Models ---

type Book struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Year   int    `json:"year,omitempty"`
}

type BookInput struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Year   int    `json:"year,omitempty"`
}

// --- In-memory store ---

type store struct {
	mu     sync.RWMutex
	books  map[string]Book
	nextID int
}

func newStore() *store {
	s := &store{books: map[string]Book{}, nextID: 1}
	// Seed with a couple of books.
	s.add(BookInput{Title: "The Go Programming Language", Author: "Alan Donovan", Year: 2015})
	s.add(BookInput{Title: "Clean Code", Author: "Robert Martin", Year: 2008})
	return s
}

func (s *store) add(input BookInput) Book {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.Itoa(s.nextID)
	s.nextID++
	b := Book{ID: id, Title: input.Title, Author: input.Author, Year: input.Year}
	s.books[id] = b
	return b
}

func (s *store) get(id string) (Book, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.books[id]
	return b, ok
}

func (s *store) list() []Book {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Book, 0, len(s.books))
	for _, b := range s.books {
		out = append(out, b)
	}
	return out
}

func (s *store) update(id string, input BookInput) (Book, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.books[id]; !ok {
		return Book{}, false
	}
	b := Book{ID: id, Title: input.Title, Author: input.Author, Year: input.Year}
	s.books[id] = b
	return b, true
}

func (s *store) delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.books[id]; !ok {
		return false
	}
	delete(s.books, id)
	return true
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func requireAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return auth == "Bearer "+validToken
}

func decodeBody(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// --- Handlers ---

func authHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Username == "admin" && req.Password == "password" {
		writeJSON(w, http.StatusOK, map[string]string{"token": validToken})
		return
	}
	writeError(w, http.StatusUnauthorized, "invalid credentials")
}

func booksHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.list())

		case http.MethodPost:
			var input BookInput
			if err := decodeBody(r, &input); err != nil || input.Title == "" {
				writeError(w, http.StatusBadRequest, "title and author are required")
				return
			}
			writeJSON(w, http.StatusCreated, s.add(input))

		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func bookHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Extract ID from path: /books/{id}
		id := strings.TrimPrefix(r.URL.Path, "/books/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing id")
			return
		}

		switch r.Method {
		case http.MethodGet:
			b, ok := s.get(id)
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSON(w, http.StatusOK, b)

		case http.MethodPut:
			var input BookInput
			if err := decodeBody(r, &input); err != nil || input.Title == "" {
				writeError(w, http.StatusBadRequest, "title and author are required")
				return
			}
			b, ok := s.update(id, input)
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSON(w, http.StatusOK, b)

		case http.MethodDelete:
			if !s.delete(id) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func main() {
	s := newStore()
	mux := http.NewServeMux()

	mux.HandleFunc("/auth/login", authHandler)
	mux.HandleFunc("/books", booksHandler(s))
	mux.HandleFunc("/books/", bookHandler(s))

	addr := ":9090"
	fmt.Printf("curlx test server running on %s\n", addr)
	fmt.Printf("Credentials: admin / password\n")
	fmt.Printf("Token: %s\n\n", validToken)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST   /auth/login\n")
	fmt.Printf("  GET    /books\n")
	fmt.Printf("  POST   /books\n")
	fmt.Printf("  GET    /books/{id}\n")
	fmt.Printf("  PUT    /books/{id}\n")
	fmt.Printf("  DELETE /books/{id}\n")

	log.Fatal(http.ListenAndServe(addr, mux))
}
