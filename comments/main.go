package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type CommentRequest struct {
	NewsID          int    `json:"news_id"`
	ParentCommentID *int   `json:"parent_comment_id,omitempty"`
	Content         string `json:"content"`
}
type Comment struct {
	ID              int    `json:"id"`
	NewsID          int    `json:"news_id"`
	ParentCommentID int    `json:"parent_comment_id,omitempty"`
	Content         string `json:"content"`
}
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func addComment(db *sql.DB, c CommentRequest) error {
	stmt, err := db.Prepare("INSERT INTO comments(news_id, parent_comment_id, content) VALUES(?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(c.NewsID, c.ParentCommentID, c.Content)
	return err
}

func getComments(db *sql.DB, newsID int) ([]Comment, error) {
	rows, err := db.Query("SELECT id, news_id, parent_comment_id, content FROM comments WHERE news_id = ?", newsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.NewsID, &c.ParentCommentID, &c.Content); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}

	return comments, nil
}
func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now() // Время начала обработки запроса
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			uuidObj, err := uuid.NewRandom()
			if err != nil {
				log.Printf("Error generating UUID: %v", err)
				http.Error(w, "Server error", http.StatusInternalServerError)
				return
			}
			requestID = uuidObj.String()
			r.Header.Set("X-Request-ID", requestID) // Добавляем ID в запрос для последующего использования
		}

		// Пытаемся получить IP-адрес клиента из заголовка `X-Forwarded-For` первым выбором
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			// Если заголовка нет, используем `RemoteAddr`
			clientIP = r.RemoteAddr
		}

		// Вспомогательный объект для перехвата статуса ответа
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		handler(ww, r) // Вызываем обработчик

		duration := time.Since(start) // Время выполнения запроса
		log.Printf("[%s] %s %s %d %s IP: %s", requestID, r.Method, r.URL.Path, ww.statusCode, duration, clientIP)
	}
}
func main() {
	f, err := os.OpenFile("CommentsLogFile", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(f)
	defer f.Close()
	cursor, err := sql.Open("sqlite3", "comment.db")
	if err != nil {
		panic(err)
	}
	defer cursor.Close()
	http.HandleFunc("/add-comment", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
			return
		}

		var req CommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Error parsing JSON request body", http.StatusBadRequest)
			return
		}

		if err := addComment(cursor, req); err != nil {
			http.Error(w, "Error adding comment", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	http.HandleFunc("/get-comments", logRequest(func(w http.ResponseWriter, r *http.Request) {
		newsID, err := strconv.Atoi(r.URL.Query().Get("news_id"))
		if err != nil {
			http.Error(w, "Invalid news ID", http.StatusBadRequest)
			return
		}

		comments, err := getComments(cursor, newsID)
		if err != nil {
			http.Error(w, "Error fetching comments", http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(comments); err != nil {
			http.Error(w, "Error encoding response into JSON", http.StatusInternalServerError)
			return
		}
	}))
	log.Println("Starting server on :8083")
	if err := http.ListenAndServe(":8083", nil); err != nil {
		log.Fatal(err)
	}
}
