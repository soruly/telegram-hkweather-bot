package main

import (
  "strings"
  "gopkg.in/telegram-bot-api.v4"
  "log"
  "time"
  "net/http"
  "regexp"
  "github.com/ungerik/go-rss"
  "gopkg.in/yaml.v2"
  "io/ioutil"
  "database/sql"
  "fmt"
  "strconv"
  _ "github.com/go-sql-driver/mysql"
)

type Config struct {
    BotToken string
    WebHookPath string
    WebHookURL string
    Listen string
    SQLConfig string
}

func fetchTopic(topic string, language string) (string, string) {
  url := ""
  switch topic {
    case "current": 
      switch language {
        case "eng":
          url = "http://rss.weather.gov.hk/rss/CurrentWeather.xml"
        case "cht":
          url = "http://rss.weather.gov.hk/rss/CurrentWeather_uc.xml"
        case "chs":
          url = "http://gbrss.weather.gov.hk/rss/CurrentWeather_uc.xml"
      }
    case "warning": 
      switch language {
        case "eng":
          url = "http://rss.weather.gov.hk/rss/WeatherWarningSummaryv2.xml"
        case "cht":
          url = "http://rss.weather.gov.hk/rss/WeatherWarningSummaryv2_uc.xml"
        case "chs":
          url = "http://gbrss.weather.gov.hk/rss/WeatherWarningSummaryv2_uc.xml"
      }
    
  }
  channel, _ := rss.Read(url)
  feedText := ""
  var rssDate rss.Date
  for _, item := range channel.Item {
    rssDate = item.PubDate
    feedText = item.Description
  }
  pubDate := fmt.Sprintf("%v",rssDate)

  if(topic == "current"){
    regexr := regexp.MustCompile(`(?s)<p>.*?</p>`)
    feedText = regexr.FindString(feedText)
    regexr = regexp.MustCompile(`[\t\r\n]`)
    feedText = regexr.ReplaceAllString(feedText,"")
    regexr = regexp.MustCompile(`<br/>`)
    feedText = regexr.ReplaceAllString(feedText,"\n")
    regexr = regexp.MustCompile(`<[^<]*>`)
    feedText = regexr.ReplaceAllString(feedText,"")
    regexr = regexp.MustCompile(`  `)
    feedText = regexr.ReplaceAllString(feedText,"")
    if(language != "eng"){
      regexr = regexp.MustCompile(` `)
      feedText = regexr.ReplaceAllString(feedText,"")
    }
    regexr = regexp.MustCompile("\n\n+")
    feedText = regexr.ReplaceAllString(feedText,"")
  }
  feedText = strings.TrimSpace(feedText)
  
  stmtIns, err := db.Prepare(`INSERT INTO feed (topic, language, pubdate, content)
    VALUES( ?, ?, ?, ? ) ON DUPLICATE KEY UPDATE pubdate=VALUES(pubdate), content=VALUES(content)`)

  if err != nil {
    panic(err.Error())
  }
  defer stmtIns.Close()
  _, err = stmtIns.Exec(topic, language, pubDate, feedText)
  if err != nil {
    panic(err.Error())
  }
  log.Println("Updated "+language+" "+ topic +" RSS feed")

  return pubDate, feedText
}


func getTopic(topic string, language string) string {
  var content string
  err := db.QueryRow("SELECT content FROM feed WHERE topic=? AND language=?", topic, language).Scan(&content)
  if err != nil {
    log.Fatal(err)
  }
  return content
}

func tellmeHandler(topic string, language string) string {
  switch topic {
    case "current", "warning":
      return getTopic(topic, language)
    default:
      return "Supported topics: *current*, *warning*"
  }
}

func subscribeHandler(userID int, topic string, language string) string {
  switch topic {
    case "current", "warning":
        stmtIns, err := db.Prepare(`INSERT INTO subscribe (id, topic)
          VALUES( ?, ? ) ON DUPLICATE KEY UPDATE topic=VALUES(topic)`)
        if err != nil {
          panic(err.Error())
        }
        defer stmtIns.Close()
        _, err = stmtIns.Exec(userID, topic)
        if err != nil {
          panic(err.Error())
        }
      return "You have subscribed "+ topic
    default:
      return "Supported topics: *current*, *warning*"
  }
}

func unsubscribeHandler(userID int, topic string, language string) string {
  switch topic {
    case "current", "warning":
        stmtIns, err := db.Prepare(`DELETE FROM subscribe WHERE id=? AND topic=?`)
        if err != nil {
          panic(err.Error())
        }
        defer stmtIns.Close()
        _, err = stmtIns.Exec(userID, topic)
        if err != nil {
          panic(err.Error())
        }
      return "You have unsubscribed "+ topic
    default:
      return "Supported topics: *current*, *warning*"
  }
}

func notifyUsers(topic string, language string, content string){
  rows, err := db.Query("SELECT `user`.`id` FROM `subscribe` LEFT JOIN user ON `user`.`id`=`subscribe`.`id` WHERE `subscribe`.`topic`='"+topic+"' AND `user`.`language`='"+language+"'")
  if err != nil {
    panic(err.Error())
  }

  columns, err := rows.Columns()
  if err != nil {
    panic(err.Error())
  }

  values := make([]sql.RawBytes, len(columns))

  scanArgs := make([]interface{}, len(values))
  for i := range values {
      scanArgs[i] = &values[i]
  }

  for rows.Next() {
    err = rows.Scan(scanArgs...)
    if err != nil {
        panic(err.Error())
    }

    var value string
    for i, col := range values {
        if col == nil {
            value = "NULL"
        } else {
            value = string(col)
        }
        //fmt.Println(columns[i], ": ", value)
        userID, _ := strconv.ParseInt(value, 10, 64)
        msg := tgbotapi.NewMessage(userID, content)
        msg.ParseMode = "Markdown"
        fmt.Printf("Notify userID %d that %s of language %s has updated", userID, topic, language)
        bot.Send(msg)
    }
  }
  if err = rows.Err(); err != nil {
    panic(err.Error())
  }  
}

func setUILanguage(userID int, language string) {
  stmtIns, err := db.Prepare(`INSERT INTO user (id, language)
    VALUES( ?, ? ) ON DUPLICATE KEY UPDATE language=VALUES(language)`)

  if err != nil {
    panic(err.Error())
  }
  defer stmtIns.Close()
  _, err = stmtIns.Exec(userID, language)
  if err != nil {
    panic(err.Error())
  }
}

func getUILanguage(userID int) string {
  var content string
  err := db.QueryRow("SELECT language FROM user WHERE id=?", userID).Scan(&content)
  if err != nil {
    return ""
  }
  return content
}

func listenFeed(topic string, language string) {
  //load the previous feed from database in case server restarted
  temp := getTopic(topic, language)
  
  for {
    _, content := fetchTopic(topic, language)
    if(content != temp){
      log.Printf("changed prev: %s now: %s", temp, content)
      notifyUsers(topic, language, content)
      temp = content
    }
    time.Sleep(300 * time.Second)
  }
}

var db *sql.DB
var bot *tgbotapi.BotAPI
var err error

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

  bot, err = tgbotapi.NewBotAPI(config.BotToken)
  if err != nil {
    log.Fatal(err)
  }

  bot.Debug = true

  log.Printf("Authorized on account %s", bot.Self.UserName)

  _, err = bot.SetWebhook(tgbotapi.NewWebhook(config.WebHookURL+config.WebHookPath+"/"+config.BotToken))
  if err != nil {
    log.Fatal(err)
  }

  go listenFeed("current", "eng")
  go listenFeed("current", "cht")
  go listenFeed("current", "chs")
  go listenFeed("warning", "eng")
  go listenFeed("warning", "cht")
  go listenFeed("warning", "chs")

  updates := bot.ListenForWebhook(config.WebHookPath+"/"+config.BotToken)
  go http.ListenAndServe(config.Listen, nil)

  for update := range updates {
    log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

    language := getUILanguage(update.Message.From.ID)
    if(language == ""){
      log.Println("Setting default language to eng")
      language = "eng"
      setUILanguage(update.Message.From.ID, language)
    }
    log.Println("Setting UI language to "+language)
    
    responseText := ""
    args := strings.Split(update.Message.Text, " ")


    switch {
      case args[0] == "topics":
        responseText = "Supported topics: *current*, *warning*"
      case args[0] == "tellme" && len(args) <= 1:
        responseText = "What do you want me to tell?\nSupported topics: *current*, *warning*"
      case args[0] == "tellme":
        responseText = tellmeHandler(args[1], language)
      case args[0] == "subscribe":
        responseText = subscribeHandler(update.Message.From.ID, args[1], language)
      case args[0] == "unsubscribe":
        responseText = unsubscribeHandler(update.Message.From.ID, args[1], language)
      case args[0] == "English":
        language = "eng"
        setUILanguage(update.Message.From.ID, language)
        responseText = "Setting UI language to English"
      case args[0] == "繁體中文":
        language = "cht"
        setUILanguage(update.Message.From.ID, language)
        responseText = "Setting UI language to 繁體中文"
      case args[0] == "简体中文":
        language = "chs"
        setUILanguage(update.Message.From.ID, language)
        responseText = "Setting UI language to 简体中文"
      default:
        responseText = "I understand these commands: `topics`, `tellme`, `subscribe`, `unsubscribe`, `English`, `繁體中文`, `简体中文`"
    }

    msg := tgbotapi.NewMessage(update.Message.Chat.ID, responseText)
    msg.ReplyToMessageID = update.Message.MessageID
    msg.ParseMode = "Markdown"

    bot.Send(msg)
    //log.Printf("%+v\n", update)
  }
}