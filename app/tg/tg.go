package main

import (
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"hack-a-tone/internal/core/domain"
	"hack-a-tone/internal/core/port"
	"log"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

var (
	ViewData          = "Посмотреть данные о системе 📊"
	ChangePods        = "Изменить количество подов 🔢"
	RestartDeployment = "Перезапустить деплоймент 🔄"
	RestartPod        = "Перезапустить под 🔁"
	RollbackVersion   = "Откатить версию 🔙"
)

var actionButtons = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(ViewData),
		tgbotapi.NewKeyboardButton(RestartDeployment),
		tgbotapi.NewKeyboardButton(RollbackVersion),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(ChangePods),
		tgbotapi.NewKeyboardButton(RestartPod),
	),
)

type Bot struct {
	bot           *tgbotapi.BotAPI
	k8sController port.KubeController
	userLogged    map[string]bool
	usersData     map[string]string
	repo          port.AlertRepo
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func NewBot(token string, k8sController port.KubeController, db port.AlertRepo) *Bot {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		slog.Error("Не удалось создать бота", "error", err)
		return nil
	}

	return &Bot{
		bot:           bot,
		k8sController: k8sController,
		repo:          db,
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

func getPodsString(b *Bot, ns string) (string, []string, error) {
	pods, err := b.k8sController.GetAllPods(context.Background(), ns)
	if err != nil {
		slog.Error("Не удалось получить все ревизии", err)
		return "", []string{}, err
	} else {
		out := make([]string, len(pods.Items))
		podsNames := make([]string, len(pods.Items))
		for i, _ := range pods.Items {
			podsNames[i] = pods.Items[i].Name
			out[i] = fmt.Sprintf("%d) %s", i+1, pods.Items[i].Name)
		}
		str := strings.Join(out, "\n")
		return str, podsNames, nil
	}
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
	Key       string `json:"k"`
	Revision  string `json:"r"`
	Deploy    string `json:"d"`
	Namespace string `json:"n"`
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Panicf("json marshal failed: %v", err)
	}
	return string(b)
}

var ChatIDToNamespaces = map[int64][]string{}
var NamespacesToChatIDs = map[string][]int64{}

func WaitStrings(b *Bot, updates *tgbotapi.UpdatesChannel, chatID int64, startMsg string) []string {
	msg := tgbotapi.NewMessage(chatID, startMsg)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}
	askedMessage, _ := b.bot.Send(msg)

	for listen := range *updates {
		listenMessage := listen.Message
		if listenMessage != nil && listenMessage.ReplyToMessage != nil &&
			listenMessage.ReplyToMessage.MessageID == askedMessage.MessageID {
			return strings.Split(strings.TrimSpace(listenMessage.Text), " ")
		} else {
			return []string{}
		}
	}
	return []string{}
}

func (b *Bot) ValidateNamespaces(ns []string) (res []string) {
	for _, n := range ns {
		_, err := b.k8sController.GetDeployments(context.Background(), n)
		if err == nil {
			res = append(res, n)
		} else {
			slog.Info("ошибка при валидации namespace:", err)
		}
	}
	return
}

func (b *Bot) RegisterNamespaces(chatID int64, ch *tgbotapi.UpdatesChannel) {
	strs := WaitStrings(b, ch, chatID, "Введите через пробел названия неймспейсов для отслеживания")
	if len(strs) != 0 {
		var msgStr string
		vld := b.ValidateNamespaces(strs)
		if len(vld) != len(strs) {
			msgStr = "Что-то не сошлось, с ними все ок - " + strings.Join(vld, " ") + ", а пришло - " + strings.Join(strs, " ") + "\n"
		}

		// todo: проверить что существует
		ChatIDToNamespaces[chatID] = strs
		for _, str := range strs {
			a := NamespacesToChatIDs[str]
			a = append(a, chatID)
			NamespacesToChatIDs[str] = a
		}
		msg := tgbotapi.NewMessage(chatID, msgStr+fmt.Sprintf("Namespaces: %s успешно зарегистрированы !\nВведите /start для начала работы", strs))
		b.bot.Send(msg)
		slog.Info("New chat registered", "chatID", chatID, "namespaces", strs)
	}
}

func (b *Bot) start() {
	// Set update timeout
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates from bot
	updates, _ := b.bot.GetUpdatesChan(u)

	handlers := map[string]func(*tgbotapi.BotAPI, *tgbotapi.CallbackQuery){
		"roll_yes": func(b1 *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			var data ActionData
			json.Unmarshal([]byte(cq.Data), &data)

			err := b.k8sController.SetRevision(context.Background(), data.Deploy, data.Namespace, data.Revision)
			if err != nil {
				slog.Error("Сan not set revision number", err)
			}

			edit := tgbotapi.NewEditMessageText(
				cq.Message.Chat.ID,
				cq.Message.MessageID,
				"Ревизия была установлена ✅",
			)
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			b1.Send(edit)
			b1.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
		},
		"roll_no": func(b *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			edit := tgbotapi.NewEditMessageText(
				cq.Message.Chat.ID,
				cq.Message.MessageID,
				"Ревизия не была установлена ❌",
			)
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			b.Send(edit)
			b.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
		},
		"rs_yes": func(b1 *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			var data ActionData
			json.Unmarshal([]byte(cq.Data), &data)

			err := b.k8sController.RestartDeployment(context.Background(), data.Deploy, data.Namespace)
			if err != nil {
				slog.Error("Сan not restart deployment", err)
			}

			edit := tgbotapi.NewEditMessageText(
				cq.Message.Chat.ID,
				cq.Message.MessageID,
				"Deployment был перезапущен ✅",
			)
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			b1.Send(edit)
			b1.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
		},
		"rs_no": func(b *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			edit := tgbotapi.NewEditMessageText(
				cq.Message.Chat.ID,
				cq.Message.MessageID,
				"Deployment не был перезапущен ❌",
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
				if handler, found := handlers[data.Key]; found {
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

		switch update.Message.Text {
		case "/start":
			if len(ChatIDToNamespaces[update.Message.Chat.ID]) == 0 {
				b.RegisterNamespaces(update.Message.Chat.ID, &updates)
				continue
			}
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Привет ! Я создан для того, чтобы ..."+
				"\nСконфигурируй систему, с которой хочешь работать")
			msg.ReplyMarkup = actionButtons
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

			dataYes := ActionData{
				Key:       "roll_yes",
				Revision:  revision,
				Deploy:    deployment,
				Namespace: ns,
			}
			dataNo := ActionData{
				Key:       "roll_no",
				Revision:  "1",
				Deploy:    "1",
				Namespace: "1",
			}

			checkBtn := tgbotapi.NewInlineKeyboardButtonData("✅", mustJSON(dataYes))
			crossBtn := tgbotapi.NewInlineKeyboardButtonData("❌", mustJSON(dataNo))
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(checkBtn, crossBtn),
			)

			msg := tgbotapi.NewMessage(update.Message.Chat.ID,
				fmt.Sprintf("Восстановить ревизию %s у deployment %s?", revision, deployment),
			)
			msg.ReplyMarkup = keyboard

			if _, err := b.bot.Send(msg); err != nil {
				log.Println("Send message error:", err)
			}

		case ViewData:
			deployStatus, err := b.k8sController.StatusAll(context.Background())
			if err != nil {
				slog.Error("Не удалось получить общий статус", err)
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, PrettyPrintStatus(deployStatus))
				msg.ParseMode = tgbotapi.ModeMarkdown
				b.bot.Send(msg)
			}

		case ChangePods:
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

			curCount, err := b.k8sController.GetPodsCount(context.Background(), ns, deployment)
			if err != nil {
				slog.Error("Не удалось получить количество подиков", err)
				continue
			}

			number := WaitNumber(b, &updates, update.Message.Chat.ID,
				fmt.Sprintf("Введите новое количество подов (сейчас %d)", curCount), 30)
			if number != -1 {
				err = b.k8sController.ScalePod(context.Background(), deployment, ns, int32(number))
				if err != nil {
					slog.Error("Не удалось изменить количество подов", err)
					continue
				}
				newAsk := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Новое количество подов: %d", number))
				newAsk.ReplyMarkup = actionButtons
				b.bot.Send(newAsk)
			}

		case RestartDeployment:
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

			dataYes := ActionData{
				Key:       "rs_yes",
				Revision:  "1",
				Deploy:    deployment,
				Namespace: ns,
			}
			dataNo := ActionData{
				Key:       "rs_no",
				Revision:  "1",
				Deploy:    "1",
				Namespace: "1",
			}

			checkBtn := tgbotapi.NewInlineKeyboardButtonData("✅", mustJSON(dataYes))
			crossBtn := tgbotapi.NewInlineKeyboardButtonData("❌", mustJSON(dataNo))
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(checkBtn, crossBtn),
			)

			msg := tgbotapi.NewMessage(update.Message.Chat.ID,
				fmt.Sprintf("Перзапустить deployment %s?", deployment),
			)
			msg.ReplyMarkup = keyboard

			if _, err := b.bot.Send(msg); err != nil {
				log.Println("Send message error:", err)
			}

		case RestartPod:
			ask1 := "В каком namespace (введите число)?\n"
			ask2 := getNamespacesString()
			namespaceId := WaitNumber(b, &updates, update.Message.Chat.ID, ask1+ask2, int64(len(registeredNamespaces)))
			if namespaceId == -1 {
				continue
			}
			ns := registeredNamespaces[namespaceId-1]
			ask3 := "Какой под (введите число)?\n"
			ask4, pods, err := getPodsString(b, ns)
			if err != nil {
				continue
			}
			deplId := WaitNumber(b, &updates, update.Message.Chat.ID, ask3+ask4, int64(len(pods)))
			if deplId == -1 {
				continue
			}
			pod := pods[deplId-1]
			err = b.k8sController.RestartPod(context.Background(), ns, pod)
			if err != nil {
				log.Println("Can not restart pod", err)
				continue
			}
			newAsk := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Под был перезапущен"))
			newAsk.ReplyMarkup = actionButtons
			b.bot.Send(newAsk)

		default:
		}
	}
}

func (b *Bot) SendMsg(a domain.Alert) {
	var msg tgbotapi.MessageConfig

	ns, _ := b.k8sController.GetNamespaceFromPod(context.Background(), a.Labels.Pod)
	//err := b.repo.WriteAlert(a, ns)
	//if err != nil {
	//	slog.Error("Не удалось записать алерт", err)
	//}

	for _, chatID := range NamespacesToChatIDs[ns] {
		msg = tgbotapi.NewMessage(chatID, a.String())
	}
	b.bot.Send(msg)
}

func PrettyPrintStatus(deploys []domain.DeployStatus) string {
	var sb strings.Builder

	for i, deploy := range deploys {
		sb.WriteString(fmt.Sprintf("Deployment `%s` (#%d)\n", deploy.Name, i+1))
		sb.WriteString(fmt.Sprintf("Status: %s\n", deploy.Status))
		if len(deploy.Pods) == 0 {
			sb.WriteString("\tNo pods found\n")
			continue
		}

		for podName, pod := range deploy.Pods {
			sb.WriteString(fmt.Sprintf("\tPod: `%s`\n", podName))
			sb.WriteString(fmt.Sprintf("\t\tTotal CPU: %.3f cores\n", pod.TotalCPU))
			sb.WriteString(fmt.Sprintf("\t\tTotal Memory: %.3f MB\n", pod.TotalMem))

			if len(pod.Containers) == 0 {
				sb.WriteString("\t\tNo containers found\n")
				continue
			}

			for containerName, container := range pod.Containers {
				sb.WriteString(fmt.Sprintf("\t\tContainer: `%s`\n", containerName))
				sb.WriteString(fmt.Sprintf("\t\t\tCPU: %.3f cores\n", container.CPU))
				sb.WriteString(fmt.Sprintf("\t\t\tMemory: %.3f MB\n", container.Memory))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
