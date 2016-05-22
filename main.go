package main

import (
  "strings"
  "gopkg.in/telegram-bot-api.v4"
  "log"
  "net/http"
  "regexp"
  "github.com/ungerik/go-rss"
  "gopkg.in/yaml.v2"
  "io/ioutil"
)

type Config struct {
    BotToken string
    WebHookPath string
    WebHookURL string
    Listen string
}

func getCurrent() string {
  channel, _ := rss.Read("http://rss.weather.gov.hk/rss/CurrentWeather.xml")
  feedText := ""
  for _, item := range channel.Item {
    feedText = item.Description
  }
  regexr := regexp.MustCompile(`(?s)<p>.*?</p>`)
  feedText = regexr.FindString(feedText)
  regexr = regexp.MustCompile(`\n`)
  feedText = regexr.ReplaceAllString(feedText,"")
  regexr = regexp.MustCompile(`<br/>`)
  feedText = regexr.ReplaceAllString(feedText,"\n")
  regexr = regexp.MustCompile(`<[^<]*>`)
  feedText = regexr.ReplaceAllString(feedText,"")
  regexr = regexp.MustCompile(`  `)
  feedText = regexr.ReplaceAllString(feedText,"")
  return feedText
}

func getWarning() string {
  channel, _ := rss.Read("http://rss.weather.gov.hk/rss/WeatherWarningSummaryv2.xml")
  feedText := ""
  for _, item := range channel.Item {
    feedText = item.Title
  }
  return feedText
}

func tellmeHandler(topic string) string {
  switch topic {
    case "current":
      return getCurrent()
    case "warning":
      return getWarning()
    default:
      return "Supported topics: *current*, *warning*"
  }
}

func main() {
  var config Config
  source, err := ioutil.ReadFile("config.yaml")
  if err != nil {
      panic(err)
  }
  err = yaml.Unmarshal(source, &config)
  if err != nil {
      panic(err)
  }

  bot, err := tgbotapi.NewBotAPI(config.BotToken)
  if err != nil {
    log.Fatal(err)
  }

  bot.Debug = true

  log.Printf("Authorized on account %s", bot.Self.UserName)

  _, err = bot.SetWebhook(tgbotapi.NewWebhook(config.WebHookURL+config.WebHookPath+"/"+config.BotToken))
  if err != nil {
    log.Fatal(err)
  }

  updates := bot.ListenForWebhook(config.WebHookPath+"/"+config.BotToken)
  go http.ListenAndServe(config.Listen, nil)

  for update := range updates {
    log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

    responseText := ""
    args := strings.Split(update.Message.Text, " ")
    switch {
      case args[0] == "topics":
        responseText = "Supported topics: *current*, *warning*"
      case args[0] == "tellme" && len(args) <= 1:
        responseText = "What do you want me to tell?\nSupported topics: *current*, *warning*"
      case args[0] == "tellme":
        responseText = tellmeHandler(args[1])
      default:
        responseText = "I understand these commands: `topics`, `tellme`"
    }

    msg := tgbotapi.NewMessage(update.Message.Chat.ID, responseText)
    msg.ReplyToMessageID = update.Message.MessageID
    msg.ParseMode = "Markdown"

    bot.Send(msg)
    //log.Printf("%+v\n", update)
  }
}