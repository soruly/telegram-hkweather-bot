package main

import (
  "strings"
  "log"
  "time"
  "net/http"
  "regexp"
  "io/ioutil"
  "database/sql"
  "fmt"
  "strconv"
  "gopkg.in/yaml.v2"
  "gopkg.in/telegram-bot-api.v4"
  "github.com/ungerik/go-rss"
  _ "github.com/go-sql-driver/mysql"
)

type Config struct {
    BotToken string
    WebHookPath string
    WebHookURL string
    Listen string
    SQLConfig string
    UpdateInterval int
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
  if(topic == "warning"){
    regexr := regexp.MustCompile(`<br/>`)
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
      switch language {
        case "eng":
          return "You have subscribed "+ topic
        case "cht":
          return "你已訂閱了頻道 "+ topic
        case "chs":
          return "你已订阅了频道 "+ topic
      }
    default:
      switch language {
        case "eng":
          return "Please use `topic` to tell me what you want to subscribe"
        case "cht":
          return "請使用 `topic` 告訴我你想訂閱的頻道"
        case "chs":
          return "请使用 `topic` 告诉我你想订阅的频道"
      }
  }
  return ""
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
      switch language {
        case "eng":
          return "You have unsubscribed "+ topic
        case "cht":
          return "你已取消訂閱頻道 "+ topic
        case "chs":
          return "你已取消订阅频道 "+ topic
      }
    default:
      switch language {
        case "eng":
          return "Please use `topic` to tell me what you want to subscribe"
        case "cht":
          return "請使用 `topic` 告訴我你想取消訂閱的頻道"
        case "chs":
          return "请使用 `topic` 告诉我你想取消订阅的频道"
      }
  }
  return ""
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
          fmt.Println(columns[i], ": ", value)
          userID, _ := strconv.ParseInt(value, 10, 64)
          msg := tgbotapi.NewMessage(userID, content)
          msg.ParseMode = "Markdown"
          fmt.Printf("Notify userID %d that %s of language %s has updated", userID, topic, language)
          bot.Send(msg)
        }
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

func listenFeed(topic string, language string, updateInterval int) {
  //load the previous feed from database in case server restarted
  temp := getTopic(topic, language)
  
  for {
    _, content := fetchTopic(topic, language)
    if(content != temp){
      log.Printf("changed prev: %s now: %s", temp, content)
      notifyUsers(topic, language, content)
      temp = content
    }
    time.Sleep(time.Duration(updateInterval) * time.Second)
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

  go listenFeed("current", "eng", config.UpdateInterval)
  go listenFeed("current", "cht", config.UpdateInterval)
  go listenFeed("current", "chs", config.UpdateInterval)
  go listenFeed("warning", "eng", config.UpdateInterval)
  go listenFeed("warning", "cht", config.UpdateInterval)
  go listenFeed("warning", "chs", config.UpdateInterval)

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
        switch language {
          case "eng":
            responseText = "Supported topics: *current*, *warning*"
          case "cht":
            responseText = "支援的資訊頻道: *current*, *warning*"
          case "chs":
            responseText = "支援的资讯频道: *current*, *warning*"
        }
      case args[0] == "tellme" && len(args) <= 1:
        switch language {
          case "eng":
            responseText = "Please use `topic` to tell me what do you want me to tell"
          case "cht":
            responseText = "請使用 `topic` 告訴我你想知道的資訊"
          case "chs":
            responseText = "请使用 `topic` 告诉我你想知道的资讯"
        }
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
        responseText = "語言設定為繁體中文"
      case args[0] == "简体中文":
        language = "chs"
        setUILanguage(update.Message.From.ID, language)
        responseText = "语言设定为简体中文"
      default:
        switch language {
          case "eng":
            responseText = "I understand these commands: `topics`, `tellme`, `subscribe`, `unsubscribe`, `English`, `繁體中文`, `简体中文`"
          case "cht":
            responseText = "支援的指令: `topics`, `tellme`, `subscribe`, `unsubscribe`, `English`, `繁體中文`, `简体中文`"
          case "chs":
            responseText = "支援的指令: `topics`, `tellme`, `subscribe`, `unsubscribe`, `English`, `繁體中文`, `简体中文`"
        }
    }

    msg := tgbotapi.NewMessage(update.Message.Chat.ID, responseText)
    msg.ReplyToMessageID = update.Message.MessageID
    msg.ParseMode = "Markdown"

    bot.Send(msg)
    //log.Printf("%+v\n", update)
  }
}