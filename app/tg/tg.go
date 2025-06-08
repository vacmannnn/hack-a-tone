package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"hack-a-tone/internal/core/domain"
	"hack-a-tone/internal/core/port"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	SeeLastIncidents  = "Посмотреть последние N инцидентов 👀"
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
		tgbotapi.NewKeyboardButton(SeeLastIncidents),
	),
)

type Bot struct {
	bot           *tgbotapi.BotAPI
	k8sController port.KubeController
	userLogged    map[string]bool
	usersData     map[string]string
	repo          port.AlertRepo
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
				b.MessageWithReplyMarkup(listen.Message.Chat.ID,
					fmt.Sprintf("Введи целое положительное число не больше %d", mx), actionButtons)
				continue
			}
			return i64
		} else {
			b.MessageWithReplyMarkup(listen.Message.Chat.ID, "Операция была отменена", actionButtons)
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
		for i := range pods.Items {
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
		for i := range revs {
			out[i] = fmt.Sprintf("%d) %s", i+1, revs[i])
		}
		str := strings.Join(out, "\n")
		return str, revs, nil
	}
}

func getNamespacesString(chatID int64) string {
	out := make([]string, len(ChatIDToNamespaces[chatID]))
	for i, ns := range ChatIDToNamespaces[chatID] {
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
		slog.Error("json marshal failed: %v", err)
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
		slog.Info("Trying to get deployments for", ns)
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
	slog.Info("Starting register namespaces to chat -", chatID)
	strs := WaitStrings(b, ch, chatID,
		"Привет! Я создан для того, чтобы помогать быстрее реагировать на аварийные события в Kubernetes. "+
			"Введи через пробел названия неймспейсов для отслеживания")
	if len(strs) != 0 {
		slog.Info("Got not empty namespaces list to register chat with ID", chatID)
		var msgStr string
		vld := b.ValidateNamespaces(strs)
		if len(vld) != len(strs) {
			slog.Info("Some namespaces didnt pass validation:", "passed", vld, "all", strs)
			msgStr = "Что-то не сошлось, с ними все ок - " + strings.Join(vld, " ") + ", а пришло - " + strings.Join(strs, " ") + "\n"
		}

		// todo: проверить что существует
		ChatIDToNamespaces[chatID] = strs
		for _, str := range strs {
			a := NamespacesToChatIDs[str]
			a = append(a, chatID)
			NamespacesToChatIDs[str] = a
		}
		msg := tgbotapi.NewMessage(chatID, msgStr+fmt.Sprintf("Namespaces: %s успешно зарегистрированы!", strs))
		msg.ReplyMarkup = actionButtons
		b.bot.Send(msg)
		slog.Info("New chat registered", "chatID", chatID, "namespaces", strs)
	} else {
		msg := tgbotapi.NewMessage(chatID, "Операция отменена")
		msg.ReplyMarkup = actionButtons
		b.bot.Send(msg)
	}
}

func (b *Bot) start() {
	// Set update timeout
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Get updates from bot
	updates, _ := b.bot.GetUpdatesChan(u)

	handlers := map[string]func(*tgbotapi.BotAPI, *tgbotapi.CallbackQuery){
		"roll_yes": func(api *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			var data ActionData
			json.Unmarshal([]byte(cq.Data), &data)

			err := b.k8sController.SetRevision(context.Background(), data.Deploy, data.Namespace, data.Revision)
			if err != nil {
				str := "Не получилось установить ревизию"
				MessageWithReplyMarkup(api, cq.Message.Chat.ID, str, actionButtons)
				slog.Error(str, err)
			}

			edit := tgbotapi.NewEditMessageText(cq.Message.Chat.ID, cq.Message.MessageID, "Ревизия была установлена ✅")
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			api.Send(edit)
			api.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
			MessageWithReplyMarkup(api, cq.Message.Chat.ID, "Выберите следующее действие", actionButtons)
		},
		"roll_no": func(api *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			edit := tgbotapi.NewEditMessageText(cq.Message.Chat.ID, cq.Message.MessageID, "Ревизия не была установлена ❌")
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			api.Send(edit)
			api.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
			MessageWithReplyMarkup(api, cq.Message.Chat.ID, "Выберите следующее действие", actionButtons)
		},
		"rs_yes": func(api *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			var data ActionData
			json.Unmarshal([]byte(cq.Data), &data)

			err := b.k8sController.RestartDeployment(context.Background(), data.Deploy, data.Namespace)
			if err != nil {
				str := "Не получилось перезапустить deployment"
				MessageWithReplyMarkup(api, cq.Message.Chat.ID, str, actionButtons)
				slog.Error(str, err)
			}

			edit := tgbotapi.NewEditMessageText(cq.Message.Chat.ID, cq.Message.MessageID, "Deployment был перезапущен ✅")
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			api.Send(edit)
			api.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
			MessageWithReplyMarkup(api, cq.Message.Chat.ID, "Выберите следующее действие", actionButtons)
		},
		"rs_no": func(api *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
			edit := tgbotapi.NewEditMessageText(cq.Message.Chat.ID, cq.Message.MessageID, "Deployment не был перезапущен ❌")
			edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}}
			api.Send(edit)
			api.AnswerCallbackQuery(tgbotapi.NewCallback(cq.ID, ""))
			MessageWithReplyMarkup(api, cq.Message.Chat.ID, "Выберите следующее действие", actionButtons)
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

		currentMessage := update.Message
		currentChatID := currentMessage.Chat.ID

		switch currentMessage.Text {
		case "/start":
			b.RegisterNamespaces(currentChatID, &updates)

		case SeeLastIncidents:
			ask1 := "Введите количество последних инцидентов, которые вы хотите посмотреть\n"
			incidentsNum := WaitNumber(b, &updates, update.Message.Chat.ID, ask1, 20)
			if incidentsNum < 1 {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Введите положительное число меньше 20")
				msg.ReplyMarkup = actionButtons
				b.bot.Send(msg)
				continue
			}
			alerts, err := b.repo.GetLastNAlerts(int(incidentsNum), ChatIDToNamespaces[update.Message.Chat.ID])
			if err != nil {
				slog.Error("getting last n alerts from repo:", "error", err)
			}

			var astr []string
			for _, a := range alerts {
				astr = append(astr, a.String())
			}
			a := strings.Join(astr, "\n")
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, a)
			b.bot.Send(msg)

		case RollbackVersion:
			ns, depl, status := b.AskNsAndDeploy(&updates, currentChatID)
			if status != Ok {
				continue
			}

			askRevs := "Укажите номер ревизии:\n"
			revsString, revs, err := getRevisionsString(b, ns, depl)
			if err != nil {
				str := "Не получилось получить номер ревизии"
				slog.Error(str, err)
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
				continue
			}
			revId := WaitNumber(b, &updates, currentChatID, askRevs+revsString, int64(len(revs)))
			revision := revs[revId-1]

			dataYes := ActionData{
				Key:       "roll_yes",
				Revision:  revision,
				Deploy:    depl,
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
			askStr := fmt.Sprintf("Восстановить ревизию %s у deployment %s?", revision, depl)
			b.MessageWithReplyMarkup(currentChatID, askStr, keyboard)

		case ViewData:
			photoMsg := GetPhotoMessageForGrafana(update.Message.Chat.ID)
			deployStatus, err := b.k8sController.StatusAll(context.Background())
			if err != nil {
				str := "Не удалось получить общий статус"
				slog.Error(str, err)
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
			} else {
				msg := tgbotapi.NewMessage(currentChatID, PrettyPrintStatus(deployStatus))
				msg.ReplyMarkup = actionButtons
				msg.ParseMode = tgbotapi.ModeMarkdown
				b.bot.Send(msg)
				b.bot.Send(photoMsg)
			}

		case ChangePods:
			ns, depl, status := b.AskNsAndDeploy(&updates, currentChatID)
			if status != Ok {
				continue
			}
			curCount, err := b.k8sController.GetPodsCount(context.Background(), ns, depl)
			if err != nil {
				str := "Не удалось получить количество подов"
				slog.Error(str, err)
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
				continue
			}
			askStr := fmt.Sprintf("Введите новое количество подов (сейчас %d)", curCount)
			number := WaitNumber(b, &updates, currentChatID, askStr, 30)
			if number == -1 {
				continue
			}
			err = b.k8sController.ScalePod(context.Background(), depl, ns, int32(number))
			if err != nil {
				str := "Не удалось изменить количество подов"
				slog.Error(str, err)
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
			} else {
				str := fmt.Sprintf("Новое количество подов: %d", number)
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
			}

		case RestartDeployment:
			ns, depl, status := b.AskNsAndDeploy(&updates, currentChatID)
			if status != Ok {
				continue
			}

			dataYes := ActionData{Key: "rs_yes", Revision: "1", Deploy: depl, Namespace: ns}
			dataNo := ActionData{Key: "rs_no", Revision: "1", Deploy: "1", Namespace: "1"}
			checkBtn := tgbotapi.NewInlineKeyboardButtonData("✅", mustJSON(dataYes))
			crossBtn := tgbotapi.NewInlineKeyboardButtonData("❌", mustJSON(dataNo))
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(checkBtn, crossBtn),
			)
			str := fmt.Sprintf("Перзапустить deployment %s?", depl)
			b.MessageWithReplyMarkup(currentChatID, str, keyboard)

		case RestartPod:
			ns, status := b.AskNamespace(&updates, currentChatID)
			if status != Ok {
				continue
			}
			pod, status := b.AskPod(&updates, currentChatID, ns)
			if status != Ok {
				continue
			}
			err := b.k8sController.RestartPod(context.Background(), ns, pod)
			if err != nil {
				slog.Error("Can not restart pod", err)
				str := fmt.Sprintf("Не получилось перезапустить под")
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
			} else {
				str := fmt.Sprintf("Под был перезапущен")
				b.MessageWithReplyMarkup(currentChatID, str, actionButtons)
			}
		default:
		}
	}
}

var PodsThatWas = map[string]string{}

type Status int

// 2) определяем константы с помощью iota
const (
	Ok        Status = iota // 0
	Cancelled               // 1
	Error                   // 2
)

func (b *Bot) AskPod(updates *tgbotapi.UpdatesChannel, chatId int64, ns string) (string, Status) {
	askPods := "Какой под (введите число)?\n"
	podsString, pods, err := getPodsString(b, ns)
	if err != nil {
		b.MessageWithReplyMarkup(chatId, err.Error(), actionButtons)
		return "", Error
	}
	podID := WaitNumber(b, updates, chatId, askPods+podsString, int64(len(pods)))
	if podID == -1 {
		return "", Cancelled
	}
	pod := pods[podID-1]

	return pod, Ok
}

func (b *Bot) AskNsAndDeploy(updates *tgbotapi.UpdatesChannel, chatId int64) (string, string, Status) {
	ns, status := b.AskNamespace(updates, chatId)
	if status != Ok {
		return "", "", status
	}
	depl, status := b.AskDeploy(updates, chatId, ns)
	if status != Ok {
		return ns, "", status
	}
	return ns, depl, Ok
}

func (b *Bot) AskNamespace(updates *tgbotapi.UpdatesChannel, chatId int64) (string, Status) {
	askNs := "В каком namespace (введите число)?\n" + getNamespacesString(chatId)
	namespaceId := WaitNumber(b, updates, chatId, askNs, int64(len(ChatIDToNamespaces)))
	if namespaceId == -1 {
		return "", Cancelled
	}
	ns := ChatIDToNamespaces[chatId][namespaceId-1]
	return ns, Ok
}

func (b *Bot) AskDeploy(updates *tgbotapi.UpdatesChannel, chatId int64, ns string) (string, Status) {
	askDepls := "В каком deployment (введите число)?\n"
	deplsString, depls, err := getDeploymentsString(b, ns)
	if err != nil {
		b.MessageWithReplyMarkup(chatId, err.Error(), actionButtons)
		return "", Error
	}
	deplId := WaitNumber(b, updates, chatId, askDepls+deplsString, int64(len(depls)))
	if deplId == -1 {
		return ns, Cancelled
	}
	deployment := depls[deplId-1]
	return deployment, Ok
}

func (b *Bot) MessageWithReplyMarkup(chatID int64, messageText string, replyMarkup interface{}) {
	MessageWithReplyMarkup(b.bot, chatID, messageText, replyMarkup)
}

func MessageWithReplyMarkup(api *tgbotapi.BotAPI, chatID int64, messageText string, replyMarkup interface{}) {
	newMessage := tgbotapi.NewMessage(chatID, messageText)
	newMessage.ReplyMarkup = replyMarkup
	_, err := api.Send(newMessage)
	if err != nil {
		slog.Error("Can not send reply message", err)
	}
}

func (b *Bot) SendAlert(a domain.Alert) {
	var msg tgbotapi.MessageConfig

	ns, err := b.k8sController.GetNamespaceFromPod(context.Background(), a.Labels.Pod)
	if err != nil {
		slog.Error("getting namespace from pod name", err)
		ns = PodsThatWas[a.Labels.Pod]
	} else {
		PodsThatWas[a.Labels.Pod] = ns
	}

	err = b.repo.WriteAlert(a, ns)
	if err != nil {
		slog.Error("Не удалось записать алерт", err)
	}

	for _, chatID := range NamespacesToChatIDs[ns] {
		msg = tgbotapi.NewMessage(chatID, a.String())
		b.bot.Send(msg)
	}
}

func GetPhotoMessageForGrafana(chatId int64) *tgbotapi.PhotoConfig {
	token := os.Getenv("GRAFANA_TOKEN")
	addr := strings.Split(os.Getenv("GRAFANA_ADDR"), ":")
	ip := addr[0]
	port := addr[1]

	url := fmt.Sprintf("http://%s:%s/render/d/efa86fd1d0c121a26444b636a3f509a9/cluster-overview?orgId=1&from=now-1h&to=now", ip, port)
	req, err := http.NewRequest("GET", url, nil)

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	if err != nil {
		slog.Error("Не удалось получить kafka png", err)
		return nil
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Не удалось получить kafka png", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Failed to download file: %s", resp.Status)
	}

	buf := new(bytes.Buffer)

	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		slog.Error("Failed to copy bytes png", err)
		return nil
	}

	img, err := png.Decode(buf)
	if err != nil {
		slog.Error("Failed to decode png", err)
		return nil
	}

	var buffNew []byte
	buffNew, err = cropTopPixelsPNG(img, 130)

	fileBytes := tgbotapi.FileBytes{
		Name:  "dashboard.png",
		Bytes: buffNew,
	}

	res := tgbotapi.NewPhotoUpload(chatId, fileBytes)
	return &res
}

func cropTopPixelsPNG(img image.Image, cropY int) ([]byte, error) {
	src, ok := img.(*image.RGBA)
	if !ok {
		bounds := img.Bounds()
		src = image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				src.Set(x, y, img.At(x, y))
			}
		}
	}
	newBounds := src.Bounds()
	newBounds.Min.Y += cropY
	if newBounds.Min.Y >= newBounds.Max.Y {
		return nil, io.ErrUnexpectedEOF
	}
	dst := image.NewRGBA(newBounds)
	for y := newBounds.Min.Y; y < newBounds.Max.Y; y++ {
		for x := newBounds.Min.X; x < newBounds.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, dst)
	if err != nil {
		slog.Error("Failed to encode PNG:", err)
		return nil, err
	}

	return buf.Bytes(), nil
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
