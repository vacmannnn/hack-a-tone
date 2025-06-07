package main

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/Syfaro/telegram-bot-api"
)

var numericKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Есть ли babyfox ?"),
		tgbotapi.NewKeyboardButton("Babyfox есть в андрейке"),
		tgbotapi.NewKeyboardButton("Babyfox нет в андрейке"),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Хочу потыкать кнопки"),
	),
)

var howMany = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("1-5"),
		tgbotapi.NewKeyboardButton("5-10"),
		tgbotapi.NewKeyboardButton("10-15"),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("15+"),
		tgbotapi.NewKeyboardButton("Не могу точно сказать"),
	),
)

var buttonsToClick = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("1"),
		tgbotapi.NewKeyboardButton("2"),
		tgbotapi.NewKeyboardButton("3"),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("4"),
		tgbotapi.NewKeyboardButton("5"),
		tgbotapi.NewKeyboardButton("6"),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("7"),
		tgbotapi.NewKeyboardButton("8"),
		tgbotapi.NewKeyboardButton("9"),
	),
)

func telegramBot() {
	// Create bot
	bot, err := tgbotapi.NewBotAPI("8000937203:AAHC8ZofmbGMGFw5gbOVPnfLqwdrgOarjYs")
	if err != nil {
		panic(err)
	}

	// Set update timeout
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates from bot
	updates, _ := bot.GetUpdatesChan(u)
	var barely string
	var mt sync.Mutex
	lastTimesSeen := make([]string, 3)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		var msg tgbotapi.MessageConfig

		// Check if message from user is text
		if reflect.TypeOf(update.Message.Text).Kind() == reflect.String && update.Message.Text != "" {
			log.Println(update.Message.Text)
			switch strings.ToLower(update.Message.Text) {
			case "/start":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Привет ! Я создан для того, чтобы подсказывать,"+
					" есть ли babyfox в андрейке.\nВыбери, что ты хочешь: рассказать, есть ли он, или узнать")
				msg.ReplyMarkup = numericKeyboard

			case "есть ли babyfox ?":
				var text string
				if lastTimesSeen[2] == "" {
					text = "Еще никто не рассказывал о его наличие, будь первым !"
				} else {
					text = lastTimesSeen[0] + "\n" + lastTimesSeen[1] + "\n" + lastTimesSeen[2] + "\n"
				}
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, text)

			case "хочу потыкать кнопки":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Будет сделано !")
				msg.ReplyMarkup = buttonsToClick

			case "babyfox есть в андрейке":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Отлично !")
				msg.ReplyMarkup = howMany
				mt.Lock()
				seen := "Был, " + time.Now().Add(time.Hour*3).Format("2006.01.02 15:04:05")
				lastTimesSeen = lastTimesSeen[1:]
				lastTimesSeen = append(lastTimesSeen, seen)
				mt.Unlock()

			case "1-5", "5-10", "10-15", "15+":
				barely = update.Message.Text
				lastTimesSeen[2] = lastTimesSeen[2] + ", примерно " + barely
				fmt.Println(msg.ChannelUsername)
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Спасибо !")
				msg.ReplyMarkup = numericKeyboard

			case "babyfox нет в андрейке":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Грустно :(")
				mt.Lock()
				seen := "Не было, " + time.Now().Add(time.Hour*3).Format("2006.01.02 15:04:05")
				lastTimesSeen = lastTimesSeen[1:]
				lastTimesSeen = append(lastTimesSeen, seen)
				mt.Unlock()

			default:
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Мне тяжело отвечать на что-то нестандартное, попробуй нажать на кнопку !")
				msg.ReplyMarkup = numericKeyboard

			}
		} else {
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Я не умею такое обрабатывать :(\nВоспользуйся подсказками !")
		}
		bot.Send(msg)
	}
}

func main() {
	telegramBot()
}
