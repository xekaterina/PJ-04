package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

type NewsFullDetailed struct {
	ID       int       `json:"id"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	Author   string    `json:"author"`
	Comments []Comment `json:"comments"`
}

type NewsShortDetailed struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type Comment struct {
	ID      int    `json:"id"`
	NewsID  int    `json:"newsId"`
	Author  string `json:"author"`
	Content string `json:"content"`
}

func main() {
	http.HandleFunc("/news", func(w http.ResponseWriter, r *http.Request) {
		// Создаём клиента для выполнения запроса
		client := &http.Client{}
		newsID := r.URL.Query().Get("news_id")

		if newsID != "" {
			// Обновляем URL, убираем /get-detailed, используем существующий обработчик
			backendNewsURL := fmt.Sprintf("http://localhost:8080/news?news_id=%s", newsID)

			respNews, err := client.Get(backendNewsURL) // Прямо используем client.Get для упрощения
			if err != nil {
				http.Error(w, "Error making request to backend for news detail", http.StatusInternalServerError)
				return
			}
			defer respNews.Body.Close()

			newsBody, err := io.ReadAll(respNews.Body)
			if err != nil {
				http.Error(w, "Error reading response from backend for news detail", http.StatusInternalServerError)
				return
			}

			// Продолжаем с запросом комментариев
			backendCommentsURL := fmt.Sprintf("http://localhost:8083/get-comments?news_id=%s", newsID)
			respComments, err := client.Get(backendCommentsURL)
			if err != nil {
				http.Error(w, "Error making request to backend for comments", http.StatusInternalServerError)
				return
			}
			defer respComments.Body.Close()

			commentsBody, err := io.ReadAll(respComments.Body)
			if err != nil {
				http.Error(w, "Error reading response from backend for comments", http.StatusInternalServerError)
				return
			}

			// Объединяем в JSON на клиентской стороне
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Write([]byte(fmt.Sprintf("{\"news\":%s,\"comments\":%s}", newsBody, commentsBody)))
			return
		}
		// Перенаправляем исходные параметры запроса
		searchQuery := r.URL.Query().Get("search")
		page := r.URL.Query().Get("page")

		// Собираем URL бэкенда
		backendURL := "http://localhost:8080/news?" + url.Values{"search": []string{searchQuery}, "page": []string{page}}.Encode()

		// Создаём новый запрос к бэкенду
		req, err := http.NewRequest("GET", backendURL, nil)
		if err != nil {
			http.Error(w, "Error creating request", http.StatusBadRequest)
			return
		}

		// Если есть, переносим уникальный ID запроса
		if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
			req.Header.Add("X-Request-ID", requestID)
		}

		// Выполняем запрос к бэкенду
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Error making request to backend", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Читаем ответ от бэкенда
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Error reading response from backend", http.StatusInternalServerError)
			return
		}

		// Устанавливаем Content-Type в соответствии с ответом бэкенда
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// Устанавливаем статус-код ответа в соответствии с ответом бэкенда
		w.WriteHeader(resp.StatusCode)
		// Передаём тело ответа от бэкенда клиенту
		w.Write(body)
	})
	http.HandleFunc("/add-comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Проверка комментария в сервисе цензурирования
		censorshipServiceURL := "http://localhost:8084/censor"
		censorReq, err := http.NewRequest("POST", censorshipServiceURL, bytes.NewBuffer(body))
		if err != nil {
			http.Error(w, "Error creating request to censorship service", http.StatusInternalServerError)
			return
		}

		censorReq.Header.Set("Content-Type", "application/json")
		censorClient := &http.Client{}
		censorResp, err := censorClient.Do(censorReq)
		if err != nil {
			http.Error(w, "Error sending request to censorship service", http.StatusInternalServerError)
			return
		}
		defer censorResp.Body.Close()

		if censorResp.StatusCode != http.StatusOK {
			http.Error(w, "Comment contains forbidden words", http.StatusBadRequest)
			return
		}

		// Отправка запроса в сервис комментариев после успешной проверки цензурирования
		backendURL := "http://localhost:8083/add-comment"
		backendReq, err := http.NewRequest("POST", backendURL, bytes.NewBuffer(body))
		if err != nil {
			http.Error(w, "Error creating request to backend", http.StatusInternalServerError)
			return
		}

		backendReq.Header = r.Header

		backendClient := &http.Client{}
		backendResp, err := backendClient.Do(backendReq)
		if err != nil {
			http.Error(w, "Error sending request to backend", http.StatusInternalServerError)
			return
		}
		defer backendResp.Body.Close()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(backendResp.StatusCode)
		io.Copy(w, backendResp.Body)
	})

	http.HandleFunc("/get-comments", func(w http.ResponseWriter, r *http.Request) {
		newsID := r.URL.Query().Get("news_id")

		// Проверка наличия параметра news_id в запросе
		if newsID == "" {
			http.Error(w, "Missing news ID", http.StatusBadRequest)
			return
		}

		// Формирование URL для запроса к бэкенду
		backendURL := "http://localhost:8083/get-comments"
		params := url.Values{}
		params.Add("news_id", newsID)
		completeURL := backendURL + "?" + params.Encode()

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Отправка полученного ответа клиенту API-прослойки
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		w.Write(data)
	})

	log.Println("API Gateway running on port 8081")
	http.ListenAndServe(":8081", nil)
}
