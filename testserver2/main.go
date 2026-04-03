// testserver2 is a task/project management API for testing multi-spec curlx functionality.
// It implements the API described in openapi.yaml and runs on :9091.
//
// Usage:
//
//	go run ./testserver2
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

const validToken = "task-token-xyz789"

// --- Models ---

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UserInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Done        bool   `json:"done"`
}

type TaskInput struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Done        bool   `json:"done"`
}

// --- In-memory store ---

type store struct {
	mu       sync.RWMutex
	users    map[string]User
	projects map[string]Project
	tasks    map[string]Task
	nextID   int
}

func newStore() *store {
	s := &store{
		users:    map[string]User{},
		projects: map[string]Project{},
		tasks:    map[string]Task{},
		nextID:   1,
	}
	// Seed data
	s.users["1"] = User{ID: "1", Name: "Jane Doe", Email: "jane@example.com"}
	s.users["2"] = User{ID: "2", Name: "John Smith", Email: "john@example.com"}
	s.nextID = 3

	s.projects["1"] = Project{ID: "1", Name: "Website Redesign", Description: "Redesign the company website"}
	s.projects["2"] = Project{ID: "2", Name: "API v2", Description: "Build the next API version"}

	s.tasks["1"] = Task{ID: "1", Title: "Write unit tests", ProjectID: "1", Done: false}
	s.tasks["2"] = Task{ID: "2", Title: "Update docs", ProjectID: "1", Done: true}
	s.tasks["3"] = Task{ID: "3", Title: "Design endpoints", ProjectID: "2", Done: false}

	return s
}

func (s *store) nextIDStr() string {
	s.nextID++
	return strconv.Itoa(s.nextID)
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
	return r.Header.Get("Authorization") == "Bearer "+validToken
}

func decodeBody(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func idFromPath(r *http.Request, prefix string) string {
	return strings.TrimPrefix(r.URL.Path, prefix)
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

func usersHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.mu.RLock()
			users := make([]User, 0, len(s.users))
			for _, u := range s.users {
				users = append(users, u)
			}
			s.mu.RUnlock()
			writeJSON(w, http.StatusOK, users)
		case http.MethodPost:
			var input UserInput
			if err := decodeBody(r, &input); err != nil || input.Name == "" {
				writeError(w, http.StatusBadRequest, "name and email are required")
				return
			}
			s.mu.Lock()
			id := s.nextIDStr()
			u := User{ID: id, Name: input.Name, Email: input.Email}
			s.users[id] = u
			s.mu.Unlock()
			writeJSON(w, http.StatusCreated, u)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func userHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := idFromPath(r, "/users/")
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.mu.RLock()
		u, ok := s.users[id]
		s.mu.RUnlock()
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

func projectsHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.mu.RLock()
			projects := make([]Project, 0, len(s.projects))
			for _, p := range s.projects {
				projects = append(projects, p)
			}
			s.mu.RUnlock()
			writeJSON(w, http.StatusOK, projects)
		case http.MethodPost:
			var input ProjectInput
			if err := decodeBody(r, &input); err != nil || input.Name == "" {
				writeError(w, http.StatusBadRequest, "name is required")
				return
			}
			s.mu.Lock()
			id := s.nextIDStr()
			p := Project{ID: id, Name: input.Name, Description: input.Description}
			s.projects[id] = p
			s.mu.Unlock()
			writeJSON(w, http.StatusCreated, p)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func projectHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := idFromPath(r, "/projects/")
		switch r.Method {
		case http.MethodGet:
			s.mu.RLock()
			p, ok := s.projects[id]
			s.mu.RUnlock()
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSON(w, http.StatusOK, p)
		case http.MethodDelete:
			s.mu.Lock()
			_, ok := s.projects[id]
			if ok {
				delete(s.projects, id)
			}
			s.mu.Unlock()
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func tasksHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		switch r.Method {
		case http.MethodGet:
			projectFilter := r.URL.Query().Get("project_id")
			s.mu.RLock()
			tasks := make([]Task, 0, len(s.tasks))
			for _, t := range s.tasks {
				if projectFilter == "" || t.ProjectID == projectFilter {
					tasks = append(tasks, t)
				}
			}
			s.mu.RUnlock()
			writeJSON(w, http.StatusOK, tasks)
		case http.MethodPost:
			var input TaskInput
			if err := decodeBody(r, &input); err != nil || input.Title == "" {
				writeError(w, http.StatusBadRequest, "title is required")
				return
			}
			s.mu.Lock()
			id := s.nextIDStr()
			t := Task{ID: id, Title: input.Title, Description: input.Description, ProjectID: input.ProjectID, Done: input.Done}
			s.tasks[id] = t
			s.mu.Unlock()
			writeJSON(w, http.StatusCreated, t)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func taskHandler(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id := idFromPath(r, "/tasks/")
		switch r.Method {
		case http.MethodGet:
			s.mu.RLock()
			t, ok := s.tasks[id]
			s.mu.RUnlock()
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeJSON(w, http.StatusOK, t)
		case http.MethodPut:
			var input TaskInput
			if err := decodeBody(r, &input); err != nil || input.Title == "" {
				writeError(w, http.StatusBadRequest, "title is required")
				return
			}
			s.mu.Lock()
			_, ok := s.tasks[id]
			if ok {
				s.tasks[id] = Task{ID: id, Title: input.Title, Description: input.Description, ProjectID: input.ProjectID, Done: input.Done}
			}
			s.mu.Unlock()
			if !ok {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			s.mu.RLock()
			t := s.tasks[id]
			s.mu.RUnlock()
			writeJSON(w, http.StatusOK, t)
		case http.MethodDelete:
			s.mu.Lock()
			_, ok := s.tasks[id]
			if ok {
				delete(s.tasks, id)
			}
			s.mu.Unlock()
			if !ok {
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
	mux.HandleFunc("/users", usersHandler(s))
	mux.HandleFunc("/users/", userHandler(s))
	mux.HandleFunc("/projects", projectsHandler(s))
	mux.HandleFunc("/projects/", projectHandler(s))
	mux.HandleFunc("/tasks", tasksHandler(s))
	mux.HandleFunc("/tasks/", taskHandler(s))

	addr := ":9091"
	fmt.Printf("curlx test server 2 (Task Manager) running on %s\n", addr)
	fmt.Printf("Credentials: admin / password\n")
	fmt.Printf("Token: %s\n\n", validToken)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST   /auth/login\n")
	fmt.Printf("  GET    /users\n")
	fmt.Printf("  POST   /users\n")
	fmt.Printf("  GET    /users/{id}\n")
	fmt.Printf("  GET    /projects\n")
	fmt.Printf("  POST   /projects\n")
	fmt.Printf("  GET    /projects/{id}\n")
	fmt.Printf("  DELETE /projects/{id}\n")
	fmt.Printf("  GET    /tasks\n")
	fmt.Printf("  POST   /tasks\n")
	fmt.Printf("  GET    /tasks/{id}\n")
	fmt.Printf("  PUT    /tasks/{id}\n")
	fmt.Printf("  DELETE /tasks/{id}\n")

	log.Fatal(http.ListenAndServe(addr, mux))
}
