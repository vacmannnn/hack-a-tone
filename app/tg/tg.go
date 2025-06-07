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
		tgbotapi.NewKeyboardButton("Добавить подиков"),
		tgbotapi.NewKeyboardButton("Уменьшить подиков"),
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

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
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

var registeredNamespaces = []string{"default", "kube-system"}

func WaitNumber(b *Bot, updates *tgbotapi.UpdatesChannel, chatID int64, start string, mx int64) int64 {
	msg := tgbotapi.NewMessage(chatID, start)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}
	askedMessage, _ := b.bot.Send(msg)

	for listen := range *updates {
		listenMessage := listen.Message
		if listenMessage != nil && listenMessage.ReplyToMessage != nil &&
			listenMessage.ReplyToMessage.MessageID == askedMessage.MessageID {
			i64, err := strconv.ParseInt(listenMessage.Text, 10, 64)
			if err != nil || i64 <= 0 || i64 > mx {
				newAsk := tgbotapi.NewMessage(listen.Message.Chat.ID,
					fmt.Sprintf("Введи целое положительное число не больше %d", mx))
				newAsk.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}
				askedMessage, _ = b.bot.Send(newAsk)
				continue
			}
			return i64
		} else {
			return -1
		}
	}
	return -1
}

func printNamespaces() string {
	out := make([]string, len(registeredNamespaces))
	for i, ns := range registeredNamespaces {
		out[i] = fmt.Sprintf("%d) %s", i+1, ns)
	}
	str := strings.Join(out, "\n")
	return str
}

func printDeployments(b *Bot, ns string) (string, []string, error) {
	deployments, err := b.k8sController.GetDeployments(context.Background(), ns)
	if err != nil {
		slog.Error("Не удалось получить все деплои", err)
		return "", []string{}, err
	} else {
		deps := make([]string, len(deployments.Items))
		out := make([]string, len(deployments.Items))
		for i, v := range deployments.Items {
			deps[i] = v.Name
			out[i] = fmt.Sprintf("%d) %s", i+1, v.Name)
		}
		str := strings.Join(out, "\n")
		return str, deps, nil
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
			for _, namespace := range registeredNamespaces {
				deployments, err := b.k8sController.GetDeployments(context.Background(), namespace)
				if err != nil {
					slog.Error("Не удалось получить все деплои", err)
				} else {
					out := make([]string, len(deployments.Items))
					for i, v := range deployments.Items {
						out[i] = fmt.Sprintf("%s:%s", namespace, v.Name)
					}
					str := strings.Join(out, "\n")
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, str)
					b.bot.Send(msg)
				}
			}

		case "добавить подиков":
			ask1 := "В каком namespace (введите число)?\n"
			ask2 := printNamespaces()
			namespaceId := WaitNumber(b, &updates, update.Message.Chat.ID, ask1+ask2, int64(len(registeredNamespaces)))
			ns := registeredNamespaces[namespaceId-1]

			ask3 := "В каком deployment (введите число)?\n"
			ask4, depls, err := printDeployments(b, ns)
			if err != nil {
				continue
			}
			deplId := WaitNumber(b, &updates, update.Message.Chat.ID, ask3+ask4, int64(len(depls)))
			deployment := depls[deplId-1]

			number := WaitNumber(b, &updates, update.Message.Chat.ID, "На сколько увеличить?", 1000)
			if number != -1 {
				err = b.k8sController.ScalePod(context.Background(), deployment, ns, int32(number))
				if err != nil {
					slog.Error("Не удалось увеличить подики", err)
					continue
				}
				newAsk := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Добавлено %d подиков", number))
				newAsk.ReplyMarkup = startScreen
				b.bot.Send(newAsk)
			}
		case "уменьшить подиков":
			ask1 := "В каком namespace (введите число)?\n"
			ask2 := printNamespaces()
			namespaceId := WaitNumber(b, &updates, update.Message.Chat.ID, ask1+ask2, int64(len(registeredNamespaces)))
			ns := registeredNamespaces[namespaceId-1]

			ask3 := "В каком deployment (введите число)?\n"
			ask4, depls, err := printDeployments(b, ns)
			if err != nil {
				continue
			}
			deplId := WaitNumber(b, &updates, update.Message.Chat.ID, ask3+ask4, int64(len(depls)))
			deployment := depls[deplId-1]

			number := WaitNumber(b, &updates, update.Message.Chat.ID, "На сколько уменьшить?", 1000)
			if number != -1 {
				err = b.k8sController.ScalePod(context.Background(), deployment, ns, int32(number))
				if err != nil {
					slog.Error("Не удалось уменьшить подики", err)
					continue
				}
				newAsk := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Убавлено %d подиков", number))
				newAsk.ReplyMarkup = startScreen
				b.bot.Send(newAsk)
			}
		default:
		}
	}
}
