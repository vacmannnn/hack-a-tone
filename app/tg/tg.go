package main

import (
	"context"
	"fmt"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"hack-a-tone/internal/core/port"
	"log/slog"
	"strconv"
	"strings"
)

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
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		switch strings.ToLower(update.Message.Text) {
		case "/start":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Привет ! Я создан для того, чтобы ..."+
				"\nСконфигурируй систему, с которой хочешь работать")
			msg.ReplyMarkup = startScreen
			b.bot.Send(msg)

		case "вернуться":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Выбери, что хочешь сделать:")
			msg.ReplyMarkup = startScreen
			b.bot.Send(msg)

		case "придумать название кнопки для действий надо бы":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Выбери действие, которое хочешь сделать")
			msg.ReplyMarkup = someActionButtons
			b.bot.Send(msg)

		case "посмотреть данные о системе":
			pods, err := b.k8sController.GetAllPods(context.Background())
			if err != nil {
				slog.Error("Не удалось получить все поды", err)
			}
			out := make([]string, len(pods.Items))
			for i, v := range pods.Items {
				out[i] = fmt.Sprintf("%d: %s", i+1, v.Status.Phase)
			}
			str := strings.Join(out, "\n")
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, str)
			//msg.ReplyMarkup = someActionButtons
			b.bot.Send(msg)

		case "добавить мощностей":
			ask := tgbotapi.NewMessage(update.Message.Chat.ID, "Сколько подиков добавить?")
			ask.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true} // шаг 1 :contentReference[oaicite:0]{index=0}
			askedPodsId, _ := b.bot.Send(ask)
			for listen := range updates {
				listenMessage := listen.Message
				if listenMessage != nil && listenMessage.ReplyToMessage != nil &&
					listenMessage.ReplyToMessage.MessageID == askedPodsId.MessageID {
					u32, err := strconv.ParseUint(listenMessage.Text, 10, 32)
					if err != nil {
						newAsk := tgbotapi.NewMessage(update.Message.Chat.ID, "Введи целое беззнаковое число")
						newAsk.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}
						askedPodsId, _ = b.bot.Send(newAsk)
						continue
					}
					newAsk := tgbotapi.NewMessage(listen.Message.Chat.ID, fmt.Sprintf("Добавлено %d подиков", u32))
					b.bot.Send(newAsk)
				} else {
					break
				}
			}
		default:
		}
	}
}
