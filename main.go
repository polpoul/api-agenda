package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

var db *sql.DB
var authServiceURL string

// ─── MODELS ────────────────────────────────────────────────

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Shape     string    `json:"shape"`
	CreatedAt time.Time `json:"created_at"`
}

type Instance struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

type Presence struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instance_id"`
	UserID     string    `json:"user_id"`
	Date       string    `json:"date"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
}

// ─── AUTH ──────────────────────────────────────────────────

func getUserIDFromToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", fmt.Errorf("missing token")
	}
	token := strings.TrimPrefix(auth, "Bearer ")

	req, _ := http.NewRequest("GET", authServiceURL+"/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth service unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unauthorized")
	}
	var data struct {
		UserID string `json:"user_id"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	if data.UserID == "" {
		return "", fmt.Errorf("invalid token")
	}
	return data.UserID, nil
}

// ─── HELPERS ───────────────────────────────────────────────

func jsonResp(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func errResp(w http.ResponseWriter, status int, msg string) {
	jsonResp(w, status, map[string]string{"error": msg})
}

func corsOnly(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

// ─── HANDLERS USERS ────────────────────────────────────────

func handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		corsOnly(w)
		return
	}

	switch r.Method {

	case "GET":
		// GET /users → liste tous
		// GET /users?id=eq.xxx → un seul
		idFilter := r.URL.Query().Get("id")
		var rows *sql.Rows
		var err error
		if idFilter != "" {
			id := strings.TrimPrefix(idFilter, "eq.")
			rows, err = db.Query("SELECT id, name, color, shape, created_at FROM agenda_users WHERE id = ?", id)
		} else {
			rows, err = db.Query("SELECT id, name, color, shape, created_at FROM agenda_users")
		}
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}
		defer rows.Close()
		users := []User{}
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Name, &u.Color, &u.Shape, &u.CreatedAt)
			users = append(users, u)
		}
		jsonResp(w, 200, users)

	case "POST":
		userID, err := getUserIDFromToken(r)
		if err != nil {
			errResp(w, 401, err.Error())
			return
		}

		var u User
		json.NewDecoder(r.Body).Decode(&u)
		u.ID = userID // on force l'ID depuis le token
		if u.Shape == "" {
			u.Shape = "circle"
		}

		_, err = db.Exec(
			"INSERT INTO agenda_users (id, name, color, shape) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE name=VALUES(name)",
			u.ID, u.Name, u.Color, u.Shape,
		)
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}

		db.QueryRow("SELECT id, name, color, shape, created_at FROM agenda_users WHERE id = ?", u.ID).
			Scan(&u.ID, &u.Name, &u.Color, &u.Shape, &u.CreatedAt)
		jsonResp(w, 201, []User{u})

	default:
		errResp(w, 405, "method not allowed")
	}
}

// ─── HANDLERS INSTANCES ────────────────────────────────────

func handleInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		corsOnly(w)
		return
	}

	switch r.Method {

	case "GET":
		rows, err := db.Query("SELECT id, name, owner_id, color, created_at FROM agenda_instances ORDER BY created_at ASC")
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}
		defer rows.Close()
		instances := []Instance{}
		for rows.Next() {
			var i Instance
			rows.Scan(&i.ID, &i.Name, &i.OwnerID, &i.Color, &i.CreatedAt)
			instances = append(instances, i)
		}
		jsonResp(w, 200, instances)

	case "POST":
		userID, err := getUserIDFromToken(r)
		if err != nil {
			errResp(w, 401, err.Error())
			return
		}

		var inst Instance
		json.NewDecoder(r.Body).Decode(&inst)
		inst.ID = uuid.New().String()
		inst.OwnerID = userID

		_, err = db.Exec(
			"INSERT INTO agenda_instances (id, name, owner_id, color) VALUES (?, ?, ?, ?)",
			inst.ID, inst.Name, inst.OwnerID, inst.Color,
		)
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}

		db.QueryRow("SELECT id, name, owner_id, color, created_at FROM agenda_instances WHERE id = ?", inst.ID).
			Scan(&inst.ID, &inst.Name, &inst.OwnerID, &inst.Color, &inst.CreatedAt)
		jsonResp(w, 201, []Instance{inst})

	default:
		errResp(w, 405, "method not allowed")
	}
}

// ─── HANDLERS PRESENCES ────────────────────────────────────

func handlePresences(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		corsOnly(w)
		return
	}

	switch r.Method {

	case "GET":
		rows, err := db.Query("SELECT id, instance_id, user_id, date, state, created_at FROM agenda_presences")
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}
		defer rows.Close()
		presences := []Presence{}
		for rows.Next() {
			var p Presence
			var d time.Time
			rows.Scan(&p.ID, &p.InstanceID, &p.UserID, &d, &p.State, &p.CreatedAt)
			p.Date = d.Format("2006-01-02")
			presences = append(presences, p)
		}
		jsonResp(w, 200, presences)

	case "POST":
		userID, err := getUserIDFromToken(r)
		if err != nil {
			errResp(w, 401, err.Error())
			return
		}

		var p Presence
		json.NewDecoder(r.Body).Decode(&p)
		p.UserID = userID // on force depuis le token
		if p.ID == "" {
			p.ID = uuid.New().String()
		}

		_, err = db.Exec(`
			INSERT INTO agenda_presences (id, instance_id, user_id, date, state)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE state = VALUES(state), id = id
		`, p.ID, p.InstanceID, p.UserID, p.Date, p.State)
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}

		jsonResp(w, 201, []Presence{p})

	case "DELETE":
		userID, err := getUserIDFromToken(r)
		if err != nil {
			errResp(w, 401, err.Error())
			return
		}

		q := r.URL.Query()
		instanceID := strings.TrimPrefix(q.Get("instance_id"), "eq.")
		date := strings.TrimPrefix(q.Get("date"), "eq.")

		// On force user_id depuis le token (sécurité)
		_, err = db.Exec(
			"DELETE FROM agenda_presences WHERE instance_id = ? AND user_id = ? AND date = ?",
			instanceID, userID, date,
		)
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusNoContent)

	default:
		errResp(w, 405, "method not allowed")
	}
}

// ─── MAIN ──────────────────────────────────────────────────

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("MYSQL_DSN not set")
	}
	authServiceURL = os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		authServiceURL = "https://auth.vivalink.top"
	}

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Minute * 3)

	if err = db.Ping(); err != nil {
		log.Fatal("DB unreachable:", err)
	}
	log.Println("DB connected")

	http.HandleFunc("/users", handleUsers)
	http.HandleFunc("/instances", handleInstances)
	http.HandleFunc("/presences", handlePresences)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, 200, map[string]string{"status": "ok"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("api-agenda listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
