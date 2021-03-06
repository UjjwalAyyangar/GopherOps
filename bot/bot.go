package bot

import (
	"errors"
	"fmt"
	"github.com/nlopes/slack"
	"golang.org/x/net/context"
	"Golem/git"
	"net/http"
	//"regexp"
	"strings"
)

type (
	Channel struct {
		id          string
		description string
		welcome     bool
		special     bool
	}
	Logger  func(message string, args ...interface{})
	ChatBot struct {
		adminName      string
		id             string
		botCall        string
		name           string
		token          string
		users          map[string]string
		predefMessages string
		channels       map[string]Channel
		hclient        *http.Client
		client         *slack.Client
		logf           Logger
		ctx            context.Context
	}
)

var cmdLog Logger
const (
	NotUnderstoodMessage = "Sorry I was not able to understand"
)
func (bot *ChatBot) Init(rtm *slack.RTM) error {
	bot.logf("Determining the bot / user IDs\n")
	users, err := bot.client.GetUsers()
	if err != nil {
		return err
	}
	bot.users = map[string]string{}
	for _, user := range users {
		if user.IsBot {
			bot.id = user.ID

		} else if user.IsAdmin {
			bot.users[user.Name] = user.ID
			bot.adminName = user.Name
		}
	}
	if bot.id == "" {
		return errors.New("Unable to find bot in the list of users ")
	}

	//How the bot will be called?
	bot.botCall = strings.ToLower("<@" + bot.id + ">")
	users = nil
	bot.logf("Determining the channels ID\n")

	publicChannels, err := bot.client.GetChannels(true)
	//Set to true for excluding the archived channels

	for _, channel := range publicChannels {
		channelName := strings.ToLower(channel.Name)
		if chn, isPresent := bot.channels[channelName]; isPresent {
			chn.id = "#" + channel.ID
			bot.channels[channelName] = chn
		}
	}
	publicChannels = nil

	bot.logf("Determining groups ID \n")
	botGroups, err := bot.client.GetGroups(true)
	for _, group := range botGroups {
		groupName := strings.ToLower(group.Name)
		if chn, ok := bot.channels[groupName]; ok && bot.channels[groupName].id == "" {
			chn.id = group.ID
			bot.channels[groupName] = chn
		}
	}
	botGroups = nil

	bot.logf("Initialized %s with ID %s\n", bot.name, bot.id)

	msgParams := slack.PostMessageParameters{}
	_, _, err = bot.client.PostMessage(bot.users[bot.adminName], "Bot deployed", msgParams)
	if err != nil {
		bot.logf("Deployment failed", err)
	}
	return err
}
var welcomeMessage string

func SetWelcome(msg string){
	welcomeMessage = msg
}

func (b * ChatBot) TeamJoined(event *slack.TeamJoinEvent){
	var message string
	if len(welcomeMessage)==0{
		message= "Welcome to the team "+event.User.Name+"!"
	}else {
		message = welcomeMessage
	}

	msgParams := slack.PostMessageParameters{AsUser: true}
	_,_,err := b.client.PostMessage(event.User.ID,message,msgParams)
	if err!=nil{
		b.logf("%s\n",err)
		return
	}
}

func (b *ChatBot) isBotMessage(event *slack.MessageEvent, eventText string) bool {
	prefixes := []string{b.predefMessages, b.name}
	for _, p := range prefixes {
		if strings.HasPrefix(eventText, p) {
			return true
		}
	}
	return strings.HasPrefix(event.Channel, "D")
}

func (b *ChatBot) trimBot(msg string) string {
	msg = strings.Replace(msg, strings.ToLower(b.predefMessages), "", 1)
	//fmt.Println("The message is - ", msg)
	x := "<@" + strings.ToLower(b.id) + ">"
	//fmt.Println("Id is ", x)
	msg = strings.TrimPrefix(msg, x)
	msg = strings.Trim(msg, " :\n")
	//fmt.Println("final message", msg)
	return msg

}

var botResponse map[string][]string

func SetResponse(a map[string][]string, caller string) {
	if len(botResponse) == 0 {
		botResponse = a
	} else {
		botResponse[caller] = a[caller]
	}
	fmt.Println(botResponse)

}
func addReaction(caller, response string) {
	if _, isPresent := botResponse[caller]; isPresent {
		cmdLog("Could not add caller %s, since it is already present", caller)
	} else {
		botResponse[caller] = append(botResponse[caller], response)
	}
}

func isGitRequest( eventText string) bool {
	fields := strings.Fields(eventText)
	if len(fields) > 3 {

		if fields[3] == "git"{
			return true
		}
		return false
	}
	return false
}
//create a private git repository checkbit  with description "abcedf"i

func handleGitRequest(bot *ChatBot, event *slack.MessageEvent, eventText string){
	//bot.logf("Checking for git request")
	fields := strings.Fields(eventText)	
	if fields[4] == "repository" {
		repository := new (git.GitHubRepo)

		
		repository.Name = fields[5]
		repository.Scope = fields[2]
		if len(fields)>6 {
			
			repository.Description = fields[8]
		}
		git.CreateRepository(*repository)
		response := "Repository successfully created"
		bot.logf("Checkking")
		respond(bot,event,response+"\n")
	} else {
		respond(bot,event,NotUnderstoodMessage+"\n") 
	}
}

func (bot *ChatBot) HandleMessage(event *slack.MessageEvent) {
	if event.BotID != "" || event.User == "" || event.SubType == "bot_message" {
		return
	}
	eventText := strings.Trim(strings.ToLower(event.Text), " \n\r")

	if !bot.isBotMessage(event, eventText) {
		return
	}
	eventText = bot.trimBot(eventText)
	fmt.Println(eventText, len(eventText))
	
	if isGitRequest(eventText){
		handleGitRequest(bot,event,eventText)	
	} else {
		responsePresent:=0
		for _, response := range botResponse[eventText] {
			respond(bot, event, response+"\n")
			responsePresent++
		}
		if responsePresent == 0 {
			respond(bot,event,NotUnderstoodMessage+"\n")
		}
		return
	}

}

func respond(bot *ChatBot, event *slack.MessageEvent, response string) {
	msgParams := slack.PostMessageParameters{AsUser: true}
	_, _, err := bot.client.PostMessage(event.Channel, response, msgParams)
	if err != nil {
		bot.logf("%s\n", err)
	}
}

func NewBot(ctx context.Context, slackBotAPI *slack.Client, httpClient *http.Client, name, token string, log Logger) *ChatBot {
	return &ChatBot{
		ctx:     ctx,
		name:    name,
		token:   token,
		hclient: httpClient,
		client:  slackBotAPI,
		logf:    log,
		channels: map[string]Channel{
			"random":  {description: "For random stuff", welcome: true},
			"general": {description: "For general discussions", welcome: true},
		},
	}
}
