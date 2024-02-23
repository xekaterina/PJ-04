package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Newpaper struct {
	Title   string
	Content string
	Source  string
	PubDate string
}
type News struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Source  string `json:"source"`
	PubDate string `json:"pubDate"`
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "news/news.sqlite")
	if err != nil {
		return nil, err
	}
	return db, nil
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
	f, err := os.OpenFile("testlogfile", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)
	cursor, err := sql.Open("sqlite3", "news.sqlite")
	if err != nil {
		panic(err)
	}

	defer cursor.Close()
	http.HandleFunc("/news", logRequest(func(w http.ResponseWriter, r *http.Request) {
		// Получаем параметр запроса `search`, `page` и заголовок `X-Request-ID`
		searchQuery := r.URL.Query().Get("search")
		page := r.URL.Query().Get("page")
		requestID := r.Header.Get("X-Request-ID") // Предполагаем, что ID запроса уже есть в заголовке
		newsID := r.URL.Query().Get("news_id")
		if newsID != "" {
			query := "SELECT ID, Title, Content, Source, Date FROM news WHERE ID = ?"
			row := cursor.QueryRow(query, newsID) // Переход на QueryRow, поскольку ожидаем одну запись

			var news News
			if err := row.Scan(&news.ID, &news.Title, &news.Content, &news.Source, &news.PubDate); err != nil {
				http.Error(w, "Error scanning database result", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(news)

			// Завершаем выполнение функции, чтобы не продолжать дальше
			return
		}
		const pageSize = 5

		pageNum, err := strconv.Atoi(page)
		if err != nil || pageNum < 1 {
			pageNum = 1 // Если нет номера страницы или произошла ошибка, используем первую страницу
		}

		offset := (pageNum - 1) * pageSize

		// Определение общего количества новостей удовлетворяющих запросу
		var totalNews int
		countQuery := "SELECT COUNT(*) FROM news WHERE Title LIKE ?"
		err = cursor.QueryRow(countQuery, "%"+searchQuery+"%").Scan(&totalNews)
		if err != nil {
			http.Error(w, "Database error when counting news", http.StatusInternalServerError)
			return
		}

		// Вычисление общего количества страниц
		totalPages := int(math.Ceil(float64(totalNews) / float64(pageSize)))

		// Подготовка SQL запроса для выборки новостей с учётом пагинации
		query := "SELECT ID, Title, Content, Source, Date FROM news WHERE Title LIKE ? LIMIT ? OFFSET ?"
		rows, err := cursor.Query(query, "%"+searchQuery+"%", pageSize, offset)
		if err != nil {
			http.Error(w, "Database error during querying news", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var newsList []News
		for rows.Next() {
			var news News
			if err := rows.Scan(&news.ID, &news.Title, &news.Content, &news.Source, &news.PubDate); err != nil {
				http.Error(w, "Error scanning database result", http.StatusInternalServerError)
				return
			}
			newsList = append(newsList, news)
		}

		// Формирование и отправка ответа
		response := struct {
			News       []News `json:"news"`
			TotalNews  int    `json:"totalNews"`
			Page       int    `json:"page"`
			TotalPages int    `json:"totalPages"`
			RequestID  string `json:"requestId"`
		}{
			News:       newsList,
			TotalNews:  totalNews,
			Page:       pageNum,
			TotalPages: totalPages,
			RequestID:  requestID,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(response)
	}))
	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
