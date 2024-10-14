package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type User struct {
	ChatID int64  `json:"chat_id"`
	Phone  string `json:"phone,omitempty"` 
}

type SendMessageRequest struct {
	Username string `json:"user"`
	Message  string `json:"message"`
}


var users = make(map[string]User)
var mu sync.Mutex 

var bot *tgbotapi.BotAPI
var userFile = "users.json"


func loadUsers() {
	file, err := os.Open(userFile)
	if os.IsNotExist(err) {
		fmt.Println("Файл с пользователями не найден, создаем новый.")
		saveUsers() 
		return
	}
	defer file.Close()


	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Ошибка при проверке файла: %v", err)
		return
	}

	if fileInfo.Size() == 0 {
		fmt.Println("Файл пустой, инициализируем пустую базу данных пользователей.")
		users = make(map[string]User) 
		saveUsers()                   
		return
	}

	err = json.NewDecoder(file).Decode(&users)
	if err != nil {
		log.Printf("Ошибка при загрузке данных пользователей: %v", err)
	}
}


func saveUsers() {
	file, err := os.Create(userFile)
	if err != nil {
		log.Printf("Ошибка при создании файла для сохранения пользователей: %v", err)
		return
	}
	defer file.Close()

	err = json.NewEncoder(file).Encode(users)
	if err != nil {
		log.Printf("Ошибка при сохранении данных пользователей: %v", err)
	}
}

func handleMessage(update tgbotapi.Update) {

	if update.Message == nil {
		fmt.Println("Получен пустой update, сообщение отсутствует.")
		return
	}

	if update.Message.From == nil || update.Message.Chat == nil {
		fmt.Println("Сообщение не содержит данных о пользователе или чате.")
		return
	}

	username := update.Message.From.UserName
	chatID := update.Message.Chat.ID


	phone := ""
	if update.Message.Contact != nil {
		phone = update.Message.Contact.PhoneNumber
	}

	mu.Lock()
	if _, exists := users[username]; !exists {

		users[username] = User{ChatID: chatID, Phone: phone}
		fmt.Printf("Новый пользователь добавлен: %s (chat_id: %d, phone: %s)\n", username, chatID, phone)
		saveUsers()
	}
	mu.Unlock()


	msg := tgbotapi.NewMessage(chatID, "Ваш chat_id сохранен!")
	_, err := bot.Send(msg)
	if err != nil {
		fmt.Printf("Ошибка при отправке сообщения: %v\n", err)
	}
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}


	mu.Lock()
	user, exists := users[req.Username]
	mu.Unlock()

	if !exists {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	msg := tgbotapi.NewMessage(user.ChatID, req.Message)
	_, err = bot.Send(msg)
	if err != nil {
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Message sent to chat_id: %d", user.ChatID)
}

func handleSendMessageToAll(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	for username, user := range users {
		msg := tgbotapi.NewMessage(user.ChatID, req.Message)
		_, err := bot.Send(msg)
		if err != nil {
			fmt.Printf("Не удалось отправить сообщение пользователю %s (chat_id: %d): %v\n", username, user.ChatID, err)
		}
	}

	fmt.Fprintf(w, "Message sent to all users")
}

func main() {
	var err error
	bot, err = tgbotapi.NewBotAPI("telegram_bot_api") 
	if err != nil {
		log.Panic(err)
	}

	loadUsers()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Panic(err)
	}

	http.HandleFunc("/send-message", handleSendMessage)         
	http.HandleFunc("/send-message-all", handleSendMessageToAll) 

	go func() {
		fmt.Println("HTTP сервер запущен на :8080")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	for update := range updates {
		if update.Message != nil {
			handleMessage(update)
		}
	}
}
