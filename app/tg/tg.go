package main

import (
	"fmt"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"hack-a-tone/internal/core/port"
	"strings"
)

var OurChatID int64

var startScreen = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Придумать название кнопки для действий надо бы"),
		tgbotapi.NewKeyboardButton("Посмотреть данные о системе"),
	),
)

var someActionButtons = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Добавить мощностей"),
		tgbotapi.NewKeyboardButton("Перезагрузить ... (pod/service)"),
		tgbotapi.NewKeyboardButton("Rollback"),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Посмотреть данные о системе"),
		tgbotapi.NewKeyboardButton("Вернуться"),
	),
)

type Bot struct {
	bot           *tgbotapi.BotAPI
	k8sController port.KubeController
	userLogged    map[string]bool
	usersData     map[string]string
}

func NewBot(token string, k8sController port.KubeController) *Bot {
	bot, err := tgbotapi.NewBotAPI(token)
	// todo: not panic
	if err != nil {
		panic(err)
	}

	return &Bot{
		bot:           bot,
		k8sController: k8sController,
	}
}

func (b *Bot) start() {
	// Set update timeout
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates from bot
	updates, _ := b.bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		OurChatID = update.Message.Chat.ID

		var msg tgbotapi.MessageConfig

		if update.Message.Text != "" {
			switch strings.ToLower(update.Message.Text) {
			case "/start":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Привет ! Я создан для того, чтобы ..."+
					"\nСконфигурируй систему, с которой хочешь работать")
				msg.ReplyMarkup = startScreen

			case "вернуться":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Выбери, что хочешь сделать:")
				msg.ReplyMarkup = startScreen

			case "придумать название кнопки для действий надо бы":
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Выбери действие, которое хочешь сделать")
				msg.ReplyMarkup = someActionButtons
				fmt.Println("im here")

			default:
			}

		} else {
			//msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Я не умею такое обрабатывать :(\nВоспользуйся подсказками !")
		}
		b.bot.Send(msg)
	}
}

func (b *Bot) SendMsg(a Alert) {
	var msg tgbotapi.MessageConfig

	msg = tgbotapi.NewMessage(OurChatID, a.String())

	b.bot.Send(msg)
}
