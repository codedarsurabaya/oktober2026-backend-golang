package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	_ "time/tzdata"
)

var db *sql.DB

type Clock struct {
	ID          int    `json:"id"`
	Country     string `json:"country"`
	Timezone    string `json:"timezone"`
	CreatedAt   string `json:"created_at"`
	CurrentTime string `json:"current_time"`
}

type clockInput struct {
	Country  string `json:"country"`
	Timezone string `json:"timezone"`
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func connect() {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		env("DB_HOST", "postgres"), env("DB_PORT", "5432"), env("DB_USER", "postgres"),
		env("DB_PASSWORD", "postgres"), env("DB_NAME", "worldclock"))

	var err error
	for i := 0; i < 20; i++ {
		db, err = sql.Open("postgres", dsn)
		if err == nil {
			if err = db.Ping(); err == nil {
				return
			}
		}
		log.Printf("waiting for db (%d): %v", i, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("could not connect to db: %v", err)
}

func currentTime(tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "unknown"
	}
	return time.Now().In(loc).Format("2006-01-02 15:04:05")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "go"})
}

func listHandler(w http.ResponseWriter, _ *http.Request) {
	rows, err := db.Query("SELECT id, country, timezone, created_at FROM world_clocks ORDER BY id")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	list := []Clock{}
	for rows.Next() {
		var c Clock
		if err := rows.Scan(&c.ID, &c.Country, &c.Timezone, &c.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		c.CurrentTime = currentTime(c.Timezone)
		list = append(list, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "go", "data": list})
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	var in clockInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timezone"})
		return
	}

	var c Clock
	err := db.QueryRow(
		"INSERT INTO world_clocks (country, timezone) VALUES ($1, $2) "+
			"RETURNING id, country, timezone, created_at",
		in.Country, in.Timezone,
	).Scan(&c.ID, &c.Country, &c.Timezone, &c.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.CurrentTime = currentTime(c.Timezone)
	writeJSON(w, http.StatusCreated, map[string]any{"service": "go", "data": c})
}

func methodHandler(get http.HandlerFunc, post http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			get(w, r)
		case http.MethodPost:
			post(w, r)
		case http.MethodOptions:
			writeJSON(w, http.StatusNoContent, nil)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func main() {
	connect()
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/clocks", methodHandler(listHandler, addHandler))

	addr := ":5003"
	log.Printf("go world clock api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
