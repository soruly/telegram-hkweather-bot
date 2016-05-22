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
  "database/sql"
  "fmt"
  _ "github.com/go-sql-driver/mysql"
)

type Config struct {
    BotToken string
    WebHookPath string
    WebHookURL string
    Listen string
    SQLConfig string
}

func fetchCurrent() {
  channel, _ := rss.Read("http://rss.weather.gov.hk/rss/CurrentWeather.xml")
  feedText := ""
  var pubDate rss.Date
  for _, item := range channel.Item {
    pubDate = item.PubDate
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
  
  stmtIns, err := db.Prepare(`INSERT INTO feed (topic, language, pubdate, content)
    VALUES( ?, ?, ?, ? ) ON DUPLICATE KEY UPDATE pubdate=VALUES(pubdate), content=VALUES(content)`)

  if err != nil {
    panic(err.Error())
  }
  defer stmtIns.Close()
  _, err = stmtIns.Exec("current", "eng", fmt.Sprintf("%v",pubDate), feedText)
  if err != nil {
    panic(err.Error())
  }
  log.Println("Updated current RSS feed")
}

func fetchWarning() {
  channel, _ := rss.Read("http://rss.weather.gov.hk/rss/WeatherWarningSummaryv2.xml")
  feedText := ""
  var pubDate rss.Date
  for _, item := range channel.Item {
    pubDate = item.PubDate
    feedText = item.Title
  }
  
  stmtIns, err := db.Prepare(`INSERT INTO feed (topic, language, pubdate, content)
    VALUES( ?, ?, ?, ? ) ON DUPLICATE KEY UPDATE pubdate=VALUES(pubdate), content=VALUES(content)`)

  if err != nil {
    panic(err.Error())
  }
  defer stmtIns.Close()
  _, err = stmtIns.Exec("warning", "eng", fmt.Sprintf("%v",pubDate), feedText)
  if err != nil {
    panic(err.Error())
  }
  log.Println("Updated warning RSS feed")
}

func getTopic(topic string) string {
  var content string
  err := db.QueryRow("SELECT content FROM feed WHERE topic=? AND language=?", topic, "eng").Scan(&content)
  if err != nil {
    log.Fatal(err)
  }
  log.Println(content)
  return content
}

func tellmeHandler(topic string) string {
  switch topic {
    case "current", "warning":
      return getTopic(topic)
    default:
      return "Supported topics: *current*, *warning*"
  }
}

var db *sql.DB

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

  db, err = sql.Open("mysql", config.SQLConfig)
  if err != nil {
    panic(err.Error())
  }
  defer db.Close()

  fetchCurrent()
  fetchWarning()

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