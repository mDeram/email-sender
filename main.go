package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
    amqp "github.com/rabbitmq/amqp091-go"
)

var QUEUE_NAME = "email"

type Configs map[string]ConfigItem
type publisherFunc func (email Email) error

type ConfigItem struct {
    Secret string
    From string
    To string
    Prefix string
    Secure bool
}

func parseConfig() Configs {
    var config Configs
    content, err := ioutil.ReadFile("./emailconfig.json")
    panicOnErr(err)

    panicOnErr(json.Unmarshal(content, &config))
    for _, item := range config {
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

func getQueuePublisher() publisherFunc {
    connection, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
    panicMsgOnErr(err, "Failed to connect to RabbitMQ")

    channel, err := connection.Channel()
    panicMsgOnErr(err, "Failed to open a channel")

    _, err = channel.QueueDeclare(
        QUEUE_NAME, // name
        true,    // durable
        false,   // delete when unused
        false,   // exclusive
        false,   // no-wait
        nil,     // arguments
    )
    panicMsgOnErr(err, "Failed to declare a queue")

    publisher := func (email Email) error {
        body, err := json.Marshal(email)
        if err != nil { return err }

        return channel.Publish(
            "",
            QUEUE_NAME,
            false,
            false,
            amqp.Publishing{
                ContentType: "text/plain",
                Body: []byte(body),
            },
        )
    }

    return publisher
}

func addEmailEndpoint(configs Configs, publisher publisherFunc) {
    http.HandleFunc("/send-email", func (w http.ResponseWriter, req *http.Request) {
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


        err = publisher(email)
        if err != nil {
            http.Error(w, "Could not put email in queue", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    })
}

func main() {
    configs := parseConfig()
    publisher := getQueuePublisher()

    addEmailEndpoint(configs, publisher)

    port := "8080"
    fmt.Printf("Starting mail server at port %s\n", port);
    panicOnErr(http.ListenAndServe(":" + port, nil))
}
