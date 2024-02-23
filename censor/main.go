package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
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
	f, err := os.OpenFile("CensorLogFile", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(f)
	defer f.Close()
	http.HandleFunc("/censor", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		text := string(body)
		forbiddenWords := []string{"qwerty", "йцукен", "zxvbnm"}

		for _, word := range forbiddenWords {
			if strings.Contains(strings.ToLower(text), word) {
				http.Error(w, "Forbidden word found", http.StatusBadRequest)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
	}))

	log.Println("Censorship service running on port 8084...")
	if err := http.ListenAndServe(":8084", nil); err != nil {
		log.Fatal("Error starting the server: ", err)
	}
}
