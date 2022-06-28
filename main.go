package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
    "github.com/mailgun/mailgun-go/v3"
    "context"
    "time"
)

type Config struct {
    Domain string
    ApiKey string
    Configs Configs
}

type Configs map[string]ConfigItem

type ConfigItem struct {
    Secret string
    From string
    To string
    Prefix string
    Secure bool
}

type sendEmailFunc func (email Email) error

func parseConfig() Config {
    var config Config
    content, err := ioutil.ReadFile("./emailconfig.json")
    panicOnErr(err)

    panicOnErr(json.Unmarshal(content, &config))
    for _, item := range config.Configs {
        if item.To == "" && item.Secret == "" {
            panic("Endpoint needs a secret to send to anyone")
        }
    }

    return config
}

type EmailData struct {
    To string
    Subject string
    Content string
}

type Email struct {
    From string `json:"from"`
    To string `json:"to"`
    Subject string `json:"subject"`
    Text string `json:"text,omitempty"`
    Html string `json:"html,omitempty"`
}

func panicOnErr(err error) {
    if err != nil {
        panic(err)
    }
}

func panicMsgOnErr(err error, msg string) {
    if err != nil {
        fmt.Printf("%s: %s", msg, err)
        panic(err)
    }
}

func addEmailEndpoint(configs Configs, sendEmail sendEmailFunc) {
    http.HandleFunc("/send-email", func (w http.ResponseWriter, req *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Headers", "*")
        w.Header().Set("Access-Control-Allow-Methods", "OPTIONS,POST")

        if req.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        if req.Method != "POST" {
            w.WriteHeader(http.StatusForbidden)
            return
        }

        name := req.Header.Get("Email-Server-Name")
        secret := req.Header.Get("Email-Server-Secret")

        config, exists := configs[name]
        if !exists {
            http.Error(w, "Does not match any configuration", http.StatusBadRequest)
            return
        }
        if config.Secret != "" && config.Secret != secret {
            w.WriteHeader(http.StatusForbidden)
            return
        }

        decoder := json.NewDecoder(req.Body)
        decoder.DisallowUnknownFields()
        var emailData EmailData
        err := decoder.Decode(&emailData)
        if err != nil {
            http.Error(w, "Bad json or too much fields", http.StatusBadRequest)
            return
        }
        if (emailData.To == "" && config.To == "") || emailData.Subject == "" || emailData.Content == "" {
            http.Error(w, "Missing some fields", http.StatusBadRequest)
            return
        }
        if emailData.To != "" && config.To != "" {
            http.Error(w, "You can not use a 'To' field, edit the configuration", http.StatusBadRequest)
            return
        }

        to := emailData.To
        if config.To != "" {
            to = config.To
        }

        email := Email{
            From: config.From,
            To: to,
            Subject: config.Prefix + emailData.Subject,
        }

        if config.Secure {
            email.Html = emailData.Content
        } else {
            email.Text = emailData.Content
        }

        err = sendEmail(email)
        if err != nil {
            http.Error(w, "Could not send email", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    })
}

func getMessageSender(config Config) sendEmailFunc {
    mg := mailgun.NewMailgun(config.Domain, config.ApiKey)
    mg.SetAPIBase("https://api.eu.mailgun.net/v3")

    return func(email Email) error {
        message := mg.NewMessage(
            email.From,
            email.Subject,
            email.Text,
            email.To,
        )
        message.SetHtml(email.Html)
        ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
        defer cancel()

        _, _, err := mg.Send(ctx, message)
        return err
    }
}

func main() {
    config := parseConfig()
    sendEmail := getMessageSender(config)

    addEmailEndpoint(config.Configs, sendEmail)

    port := "8080"
    fmt.Printf("Starting mail server at port %s\n", port);
    panicOnErr(http.ListenAndServe(":" + port, nil))
}
