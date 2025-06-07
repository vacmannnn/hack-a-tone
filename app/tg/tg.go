package main

import (
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"hack-a-tone/internal/core/port"
	"log"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

var OurChatID int64

var (
	ViewData          = "Посмотреть данные о системе 📊"
	AddPods           = "Увеличить количество подов ➕"
	RemovePods        = "Уменьшить количество подов ➖"
	RestartDeployment = "Перезагрузить деплоймент 🔄"
	RestartPod        = "Перезагрузить под 🔁"
	RollbackVersion   = "Откатить версию 🔙"
	GoBack            = "Вернуться ◀️"
	LoremIpsum        = "Lorem ipsum 💬"
)

var startScreen = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(LoremIpsum),
		tgbotapi.NewKeyboardButton(ViewData),
	),
)

var someActionButtons = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(AddPods),
		tgbotapi.NewKeyboardButton(RemovePods),
		tgbotapi.NewKeyboardButton(RestartDeployment),
		tgbotapi.NewKeyboardButton(RollbackVersion),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(ViewData),
		tgbotapi.NewKeyboardButton(GoBack),
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
	if err != nil {
		slog.Error("Не удалось создать бота", "error", err)
		return nil
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

func getRevisionsString(b *Bot, ns string, depl string) (string, []string, error) {
	revs, err := b.k8sController.GetAvailableRevisions(context.Background(), depl, ns)
	if err != nil {
		slog.Error("Не удалось получить все ревизии", err)
		return "", []string{}, err
	} else {
		sort.Sort(sort.Reverse(sort.StringSlice(revs)))
		out := make([]string, len(revs))
		for i, _ := range revs {
			out[i] = fmt.Sprintf("%d) %s", i+1, revs[i])
		}
		str := strings.Join(out, "\n")
		return str, revs, nil
	}
}

func getNamespacesString() string {
	out := make([]string, len(registeredNamespaces))
	for i, ns := range registeredNamespaces {
		out[i] = fmt.Sprintf("%d) %s", i+1, ns)
	}
	str := strings.Join(out, "\n")
	return str
}

func getDeploymentsString(b *Bot, ns string) (string, []string, error) {
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

type ActionData struct {
	Answer        string `json:"a"`
	CurRevVersion string `json:"r"`
	CurDeploy     string `json:"d"`
	CurNamespace  string `json:"n"`
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Panicf("json marshal failed: %v", err)
	}
	return string(b)
}

func (b *Bot) start() {
	// Set update timeout
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates from bot
	updates, _ := b.bot.GetUpdatesChan(u)

	handlers := map[string]func(*tgbotapi.BotAPI, *tgbotapi.CallbackQuery){
		"yes": func(b1 *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			var data ActionData
			json.Unmarshal([]byte(cq.Data), &data)

			err := b.k8sController.SetRevision(context.Background(), data.CurDeploy, data.CurNamespace, data.CurRevVersion)
			if err != nil {
				slog.Error("Сan not set revision number", err)
			}

			edit := tgbotapi.NewEditMessageText(
				cq.Message.Chat.ID,
				cq.Message.MessageID,
				"Версия была установлена ✅",
			)
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			b1.Send(edit)
			b1.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
		},
		"no": func(b *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			edit := tgbotapi.NewEditMessageText(
				cq.Message.Chat.ID,
				cq.Message.MessageID,
				"Версия не была установлена ❌",
			)
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			b.Send(edit)
			b.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
		},
	}

	for update := range updates {
		if update.Message == nil {
			if cq := update.CallbackQuery; cq != nil {
				var data ActionData
				json.Unmarshal([]byte(cq.Data), &data)
				if handler, found := handlers[data.Answer]; found {
					handler(b.bot, cq) // вызываем нужный обработчик
				} else {
					// необработанный callbackData
					b.bot.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, "Неизвестная кнопка"))
				}
			}
			continue
		}
		if update.Message.Text == "" {
			continue
		}

		OurChatID = update.Message.Chat.ID

		switch update.Message.Text {
		case "/start":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Привет ! Я создан для того, чтобы ..."+
				"\nСконфигурируй систему, с которой хочешь работать")
			msg.ReplyMarkup = startScreen
			b.bot.Send(msg)

		case GoBack:
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Выбери, что хочешь сделать:")
			msg.ReplyMarkup = startScreen
			b.bot.Send(msg)

		case LoremIpsum:
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Выбери действие, которое хочешь сделать")
			msg.ReplyMarkup = someActionButtons
			b.bot.Send(msg)

		case RollbackVersion:
			ask1 := "В каком namespace (введите число)?\n"
			ask2 := getNamespacesString()
			namespaceId := WaitNumber(b, &updates, update.Message.Chat.ID, ask1+ask2, int64(len(registeredNamespaces)))
			if namespaceId == -1 {
				continue
			}
			ns := registeredNamespaces[namespaceId-1]

			ask3 := "В каком deployment (введите число)?\n"
			ask4, depls, err := getDeploymentsString(b, ns)
			if err != nil {
				continue
			}
			deplId := WaitNumber(b, &updates, update.Message.Chat.ID, ask3+ask4, int64(len(depls)))
			if deplId == -1 {
				continue
			}
			deployment := depls[deplId-1]
			ask5 := "Укажите номер ревизии:\n"
			ask6, revs, err := getRevisionsString(b, ns, deployment)
			if err != nil {
				continue
			}
			revId := WaitNumber(b, &updates, update.Message.Chat.ID, ask5+ask6, int64(len(revs)))
			if revId == -1 {
				continue
			}
			revision := revs[revId-1]
			print(deployment, " ", revision)

			dataYes := ActionData{
				Answer:        "yes",
				CurRevVersion: revision,
				CurDeploy:     deployment,
				CurNamespace:  ns,
			}
			dataNo := ActionData{
				Answer:        "no",
				CurRevVersion: "1",
				CurDeploy:     "1",
				CurNamespace:  "1",
			}

			checkBtn := tgbotapi.NewInlineKeyboardButtonData("✅", mustJSON(dataYes))
			crossBtn := tgbotapi.NewInlineKeyboardButtonData("❌", mustJSON(dataNo))
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(checkBtn, crossBtn),
			)

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Восстановить ревизию?"))
			msg.ReplyMarkup = keyboard

			if _, err := b.bot.Send(msg); err != nil {
				log.Println("Send message error:", err)
			}

		case ViewData:
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

		case AddPods:
			ask1 := "В каком namespace (введите число)?\n"
			ask2 := getNamespacesString()
			namespaceId := WaitNumber(b, &updates, update.Message.Chat.ID, ask1+ask2, int64(len(registeredNamespaces)))
			if namespaceId == -1 {
				continue
			}
			ns := registeredNamespaces[namespaceId-1]

			ask3 := "В каком deployment (введите число)?\n"
			ask4, depls, err := getDeploymentsString(b, ns)
			if err != nil {
				continue
			}
			deplId := WaitNumber(b, &updates, update.Message.Chat.ID, ask3+ask4, int64(len(depls)))
			if deplId == -1 {
				continue
			}
			deployment := depls[deplId-1]

			number := WaitNumber(b, &updates, update.Message.Chat.ID, "На сколько увеличить?", 30)
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
		case RemovePods:
			ask1 := "В каком namespace (введите число)?\n"
			ask2 := getNamespacesString()
			namespaceId := WaitNumber(b, &updates, update.Message.Chat.ID, ask1+ask2, int64(len(registeredNamespaces)))
			ns := registeredNamespaces[namespaceId-1]

			ask3 := "В каком deployment (введите число)?\n"
			ask4, depls, err := getDeploymentsString(b, ns)
			if err != nil {
				continue
			}
			deplId := WaitNumber(b, &updates, update.Message.Chat.ID, ask3+ask4, int64(len(depls)))
			deployment := depls[deplId-1]

			number := WaitNumber(b, &updates, update.Message.Chat.ID, "На сколько уменьшить?", 30)
			if number != -1 {
				err = b.k8sController.ScalePod(context.Background(), deployment, ns, int32(-number))
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

func (b *Bot) SendMsg(a Alert) {
	var msg tgbotapi.MessageConfig

	msg = tgbotapi.NewMessage(OurChatID, a.String())

	b.bot.Send(msg)
}
